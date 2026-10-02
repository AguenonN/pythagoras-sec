"""Sessions passées similaires et fondements suggérés, par embeddings.

Pour **proposer** seulement : ces champs accompagnent une proposition au
juriste (``sessions_similaires``, ``fondements_suggeres``, facultatifs au
contrat §6) ; rien n'entre dans l'engine ni dans une décision du gate, et
aucun constat n'est créé ou écarté par un score.

Textes plongés (jamais une valeur du coffre) :

- une **session terminée** : la demande (``etat.json``, déjà transmise au
  modèle de l'orchestrateur) et la suite des appels du journal (outil,
  accès, décision, directives, validation, résultat) ;
- un **constat** : la nature du risque, l'action, le fait observable commun
  et les appels en cause (outils et décisions) ;
- un **article** du corpus : code, numéro et texte ;
- pour les fondements, la **proposition** : son texte et la nature du risque.
"""

from . import corpus, plongements

# Proximité d'une session passée : le plus grand cosinus (mistral-embed)
# entre son texte et celui d'une session qui fonde le constat (même genre de
# texte : demande et appels). L'embedding du constat lui-même est calculé et
# rendu (``score_constat``), mais il sépare mal les parcours (0,79 à 0,88 sur
# les passages de la démo, sessions fondatrices comprises) : il n'ordonne pas.
# Seuil fixé le 02/10 sur les scores observés (docs/rapports/analyste.md,
# « Embeddings ») : décisions de crédit 0,97 à 0,99 ; remise d'un rapport sur
# un dossier de crédit 0,92 à 0,94 ; parcours RH 0,85 à 0,88. Au-dessous de
# 0,95, rien n'est cité.
SEUIL_SESSIONS = 0.95
NB_SESSIONS = 3
NB_FONDEMENTS = 2
MENTION = "suggestion — à valider par le juriste"
TERMINEES = ("termine", "bloque", "erreur")

EFFETS = {"autoriser": "autorisé", "attendre_validation": "mis en attente de validation", "refuser": "refusé"}


def terminee(session):
    return (session.etat or {}).get("statut") in TERMINEES


def _appel(a):
    s = "%s (%s) : %s" % (a.nom, "action" if a.acces == "action" else "lecture", EFFETS.get(a.effet, a.effet))
    if a.directives_appliquees():
        s += " par " + ", ".join(a.directives_appliquees())
    if a.validee_par_humain:
        s += ", validé par un humain"
    s += ", " + ("données reçues" if a.donnees_recues else "non transmis" if not a.transmis else a.statut)
    return s


def texte_session(session):
    lignes = [(session.etat or {}).get("demande", "").strip()]
    lignes += [_appel(a) for a in session.appels]
    return "\n".join(lignes)


def _en_cause(o):
    """Les consultations que l'occurrence met en cause, par leur outil."""
    out = []
    if "consultation_interne" in o:
        out.append(o["consultation_interne"]["outil"] + " (lecture) consulté")
        out += [c["outil"] + " (lecture) consulté" for c in o.get("consultations_officielles", [])]
    out += [r["outil"] + " (lecture) : aucun résultat" for r in o.get("recherches", [])]
    out += [c["outil"] + " (lecture) consulté" for c in o.get("consultations_personnelles", [])]
    if "retrait" in o:
        out.append(o["retrait"]["outil"] + " : champs retirés par " + ", ".join(o["retrait"]["directives"]))
    return out


def texte_constat(constat, natures):
    lignes = [natures[constat["type"]]]
    fait = (constat.get("contexte") or {}).get("fait_observable_commun")
    if fait:
        lignes.append(fait)
    for o in constat["occurrences"]:
        d = o["decision"]
        lignes += _en_cause(o)
        lignes.append("%s (action) : %s%s, %s" % (o["action"], EFFETS.get(d["effet"], d["effet"]),
                                                 " par " + ", ".join(d["directives_appliquees"])
                                                 if d["directives_appliquees"] else "",
                                                 "transmise" if o["transmise"] else "non transmise"))
    return "\n".join(lignes)


def texte_article(a):
    return "%s, article %s\n%s" % (a["code"], a["article"], a["texte"])


def texte_proposition(redaction, constat, natures):
    return "%s\nRisque : %s" % (redaction["texte_propose"], natures[constat["type"]])


def sessions_similaires(constat, sessions, plongeur, natures, seuil=SEUIL_SESSIONS, n=NB_SESSIONS):
    """Les ``n`` sessions terminées les plus proches des sessions qui
    fondent le constat, hors celles-ci, au-dessus du seuil ; scores arrondis
    au millième."""
    par_nom = {s.nom: s for s in sessions}
    fondatrices = [par_nom[n_] for n_ in constat["sessions"] if n_ in par_nom]
    candidates = [s for s in sessions if terminee(s) and s.nom not in constat["sessions"]]
    if not candidates or not fondatrices:
        return []
    textes = [texte_constat(constat, natures)] + [texte_session(s) for s in fondatrices + candidates]
    vecs = plongeur.vecteurs(textes)
    v_constat, v_fond, v_cand = vecs[0], vecs[1:1 + len(fondatrices)], vecs[1 + len(fondatrices):]
    out = []
    for s, v in zip(candidates, v_cand):
        proches = [(plongements.cosinus(v, f), fs.nom) for f, fs in zip(v_fond, fondatrices)]
        score, voisine = max(proches, key=lambda x: (x[0], x[1]))
        out.append({"session": s.nom, "score": round(score, 3), "proche_de": voisine,
                    "score_constat": round(plongements.cosinus(v, v_constat), 3)})
    out.sort(key=lambda x: (-x["score"], x["session"]))
    return [x for x in out if x["score"] >= seuil][:n]


def fondements_suggeres(constat, redaction, plongeur, natures, articles=None, n=NB_FONDEMENTS):
    """Les ``n`` articles du corpus les plus proches de la proposition."""
    if articles is None:
        articles, _ = corpus.charger()
    if not articles:
        return []
    vecs = plongeur.vecteurs([texte_proposition(redaction, constat, natures)] + [texte_article(a) for a in articles])
    scores = [(round(plongements.cosinus(vecs[0], v), 3), a) for v, a in zip(vecs[1:], articles)]
    scores.sort(key=lambda x: (-x[0], x[1]["id"]))
    return [{"id": a["id"], "code": a["code"], "article": a["article"], "score": sc, "lien": a["lien"],
             "mention": MENTION} for sc, a in scores[:n]]
