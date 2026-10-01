"""Compilation v2 : un texte du juriste → un jeu de directives v2 en brouillon.

Mistral propose, le code vérifie, le juriste décide.

    texte (+ réponses) ──► Mistral ──► signaler_non_observable ──► refus motivé + question
                                  ├──► poser_question ──────────► questions
                                  └──► proposer_jeu (arbre à plat)
                                          │
              reconstruction de l'arbre (arbre.py), vérifications (rien d'inventé),
              directive verifier --exiger-brouillon (Go) ; refus : une correction
                                          │
              questions fermées du code (si_indetermine, rythme et échec des notifications)
                                          ▼
                              jeu en brouillon + directive relire (Go)

Ce que Mistral ne fixe jamais : identifiants, version, statut (toujours
brouillon), texte_fr (le texte du juriste, ou l'extrait qui le recopie mot
pour mot), date d'effet, si_indetermine, rythme et si_echec des
notifications. Les réponses aux questions du code sont consignées et ne sont
jamais envoyées à Mistral.
"""

import json
import re

from . import arbre, directive_go, verifications_v2
from . import outils_v2
from .compilateur import fondement
from .verifications import sans_accents

QUESTIONS = "questions"
NON_OBSERVABLE = "non_observable"
JEU = "jeu"
ERREURS = "erreurs"   # aucune proposition exploitable
REFUSE = "refuse"     # un jeu a été construit et refusé par les vérifications

ORIGINE_CODE = "compilateur"

# Valeurs provisoires, pour que le chargeur contrôle le reste du jeu avant que
# le juriste ait répondu aux questions du code ; jamais dans un jeu rendu.
_PROVISOIRE = {"rythme": "synthese", "si_echec": "refuser"}

_DECIMAL = re.compile(r"^-?\d+(?:\.\d+)?$")


def _du_code(r):
    return r.get("origine") == ORIGINE_CODE


def _choix(texte):
    return re.sub(r"\s+", "_", sans_accents(texte or "").strip().lower().rstrip("."))


def message_utilisateur(texte, vocab, reponses):
    parts = [
        "Catalogue des serveurs et outils disponibles :",
        vocab.pour_le_modele(),
        "",
        "Texte du juriste :",
        "<<<",
        texte,
        ">>>",
    ]
    vues, uniques = set(), []
    for r in reponses:
        cle = (r["question"].strip(), r["reponse"].strip())
        if not _du_code(r) and cle not in vues:
            vues.add(cle)
            uniques.append(r)
    if uniques:
        parts += ["", "Réponses du juriste à tes questions :"]
        for r in uniques:
            parts += ["- Question : " + r["question"], "  Réponse : " + r["reponse"]]
        parts += ["", "Ces réponses sont définitives : ne repose pas ces questions. Si chaque valeur est "
                      "maintenant déterminée, appelle proposer_jeu."]
    return "\n".join(parts)


# -- construction -------------------------------------------------------------

def _nombre(v):
    """Un nombre écrit en chaîne décimale devient un nombre (même valeur,
    autre écriture) ; un flottant entier devient un entier (2000000.0 se
    relit 2000000)."""
    if isinstance(v, str) and _DECIMAL.match(v.strip()):
        s = v.strip()
        return int(s) if "." not in s else float(s)
    if isinstance(v, float) and v.is_integer():
        return int(v)
    return v


def _normaliser_valeurs(arbre_go):
    def visiter(cle, f, _):
        if cle == "argument" and "valeur" in f:
            if f.get("op") in (">", ">=", "<", "<=", "==", "!="):
                f["valeur"] = _nombre(f["valeur"])
            elif isinstance(f["valeur"], list):
                f["valeur"] = [_nombre(x) if not isinstance(x, str) else x for x in f["valeur"]]
    arbre.parcourir(arbre_go, visiter)


def _date(iso, cle):
    import datetime
    try:
        datetime.date.fromisoformat(iso)
    except (TypeError, ValueError):
        return None, "%s : %r n'est pas une date AAAA-MM-JJ" % (cle, iso)
    return iso + "T00:00:00Z", None


def _pose(v):
    return v not in (None, "", [], {})


def construire_directive(p, texte, id_directive, date_effet, prefixe):
    """Rend (directive, erreurs) depuis une directive brute de proposer_jeu."""
    erreurs = []
    d = {"id": id_directive, "version": 1, "statut": "brouillon", "date_effet": date_effet}
    if _pose(p.get("revision_avant")):
        rev, err = _date(p["revision_avant"], prefixe + "revision_avant")
        if err:
            erreurs.append(err)
        else:
            d["revision_avant"] = rev
    extrait = (p.get("extrait") or "").strip()
    d["texte_fr"] = extrait if extrait and extrait in texte else texte
    d["fondements"] = [fondement(f) for f in (p.get("fondements") or []) if isinstance(f, dict)]
    dc = p.get("declencheur") or {}
    cible = {k: dc[k] for k in ("serveur", "outil", "etiquette", "acces") if _pose(dc.get(k))}
    d["declencheur"] = {"evenement": "appel_outil", "cible": cible}
    condition, err = arbre.reconstruire(p.get("noeuds"), p.get("racine"), prefixe + "condition : ")
    erreurs += err
    if condition is not None:
        _normaliser_valeurs(condition)
        d["condition"] = condition
    effets = []
    for e in p.get("effets") or []:
        if not isinstance(e, dict):
            continue
        out = {"effet": e.get("effet", "")}
        for k in ("par_role", "champs_autorises", "mode", "destinataire_role", "etiquette"):
            if _pose(e.get(k)):
                out[k] = e[k]
        if e.get("quatre_yeux") is True:
            out["quatre_yeux"] = True
        effets.append(out)
    d["effets"] = effets
    return d, erreurs


def _avec_provisoire(jeu):
    """Copie du jeu où les paramètres que fixera le juriste portent une
    valeur provisoire : le chargeur contrôle ainsi tout le reste."""
    j = json.loads(json.dumps(jeu))
    for d in j["directives"]:
        for e in d["effets"]:
            if e.get("effet") == "notifier":
                for k, v in _PROVISOIRE.items():
                    e.setdefault(k, v)
    return j


# -- questions du code -------------------------------------------------------

def _peut_etre_indeterminee(d):
    cible = d["declencheur"].get("cible") or {}
    return "condition" in d or bool(cible.get("etiquette") or cible.get("acces"))


def questions_du_code(jeu):
    """Les questions fermées que le code pose au juriste, dans un ordre fixe."""
    out = []
    ids = [d["id"] for d in jeu["directives"] if _peut_etre_indeterminee(d)]
    if ids:
        out.append({
            "question": ("Si une condition %s ne peut pas être évaluée pour un appel (information absente, "
                         "illisible ou inconnue du registre), faut-il refuser l'appel (choix par défaut) ou "
                         "ignorer la directive pour cet appel ? Répondez « refuser » ou « ignorer »."
                         % ((("de la directive " if len(ids) == 1 else "des directives ") + ", ".join(ids)))),
            "champ": "si_indetermine",
            "options": ["refuser", "ignorer"],
            "origine": ORIGINE_CODE,
        })
    for d in jeu["directives"]:
        for e in d["effets"]:
            if e.get("effet") != "notifier":
                continue
            role = e.get("destinataire_role", "")
            out.append({
                "question": ("Directive %s : la notification au rôle « %s » part-elle en temps réel ou dans une "
                             "synthèse périodique ? Répondez « temps_reel » ou « synthese »." % (d["id"], role)),
                "champ": "rythme:%s:%s" % (d["id"], role),
                "options": ["temps_reel", "synthese"],
                "origine": ORIGINE_CODE,
            })
            out.append({
                "question": ("Directive %s : si la notification au rôle « %s » ne peut pas partir, faut-il refuser "
                             "l'appel ou le laisser continuer ? Répondez « refuser » ou « continuer »." % (d["id"], role)),
                "champ": "si_echec:%s:%s" % (d["id"], role),
                "options": ["refuser", "continuer"],
                "origine": ORIGINE_CODE,
            })
    return out


def reponse_du_code(reponses, champ, options):
    """La dernière réponse consignée à la question du code ``champ``,
    normalisée, et son entrée ; None si absente ou hors options."""
    for r in reversed(reponses):
        if _du_code(r) and r.get("champ") == champ:
            choix = _choix(r.get("reponse"))
            return (choix, r) if choix in options else None
    return None


def appliquer_reponses(jeu, reponses, auteur):
    """Rend (jeu complété, questions restantes, consignation)."""
    restantes, consignation = [], []
    choix = {}
    for q in questions_du_code(jeu):
        r = reponse_du_code(reponses, q["champ"], q["options"])
        if r is None:
            restantes.append(q)
        else:
            choix[q["champ"]] = r[0]
            consignation.append(r[1])
    if restantes:
        return None, restantes, consignation
    j = json.loads(json.dumps(jeu))
    for d in j["directives"]:
        if _peut_etre_indeterminee(d):
            if choix["si_indetermine"] == "ignorer":
                d["si_indetermine"] = "ignorer"
                d["si_indetermine_valide_par"] = auteur
            else:
                d["si_indetermine"] = "refuser"
        for e in d["effets"]:
            if e.get("effet") == "notifier":
                role = e.get("destinataire_role", "")
                e["rythme"] = choix["rythme:%s:%s" % (d["id"], role)]
                e["si_echec"] = choix["si_echec:%s:%s" % (d["id"], role)]
    j["directives"] = [_ordonner(d) for d in j["directives"]]
    return j, [], consignation


_ORDRE = ["id", "version", "statut", "date_effet", "revision_avant", "texte_fr", "fondements",
          "si_indetermine", "si_indetermine_valide_par", "declencheur", "condition", "effets"]


def _ordonner(d):
    return {k: d[k] for k in _ORDRE if k in d}


# -- échange avec Mistral ----------------------------------------------------

def _appels(reponse):
    try:
        message = reponse["choices"][0]["message"]
    except (KeyError, IndexError, TypeError):
        return None, []
    appels = []
    for tc in message.get("tool_calls") or []:
        fn = tc.get("function", {})
        args = fn.get("arguments", "{}")
        if isinstance(args, str):
            try:
                args = json.loads(args)
            except ValueError:
                args = {"_illisible": args}
        appels.append((tc.get("id", ""), fn.get("name", ""), args))
    return message, appels


def compiler_jeu(texte, jeu_id, vocab, client, auteur, date_effet, reponses=(), prefixe_id="D"):
    """Rend un dict : etat (questions | non_observable | jeu | refuse | erreurs)
    et le détail.

    ``date_effet`` : horodatage RFC 3339 fixé par le juriste.
    ``reponses`` : [{"question", "reponse", "champ"?, "options"?, "origine"?}]."""
    reponses = list(reponses)
    source = "\n".join([texte] + [r["reponse"] for r in reponses if not _du_code(r)])
    resultat = {"texte": texte, "reponses": reponses, "questions": [], "non_observables": [],
                "erreurs": [], "jeu": None, "relecture": None, "tentatives": 0, "consignation": []}
    messages = [
        {"role": "system", "content": outils_v2.CONSIGNE},
        {"role": "user", "content": message_utilisateur(texte, vocab, reponses)},
    ]
    offerts = outils_v2.outils(vocab)

    for tentative in (1, 2):
        resultat["tentatives"] = tentative
        reponse = client.completer(messages, offerts)
        message, appels = _appels(reponse)
        non_obs = [a for _, n, a in appels if n == "signaler_non_observable"]
        questions = [{"question": a.get("question", ""), "sujet": a.get("sujet", "autre"),
                      "extrait": a.get("extrait", ""), "options": list(a.get("options") or [])}
                     for _, n, a in appels if n == "poser_question"]
        if non_obs:
            # Le refus motivé l'emporte : rien n'est proposé pour une règle
            # dont le critère ne se constate pas. Sa question rejoint les autres.
            resultat["non_observables"] = [{"extrait": a.get("extrait", ""), "raison": a.get("raison", "")}
                                           for a in non_obs]
            questions = [{"question": a.get("question", ""), "sujet": "non_observable",
                          "extrait": a.get("extrait", ""), "raison": a.get("raison", ""), "options": []}
                         for a in non_obs] + questions
            resultat.update(etat=NON_OBSERVABLE, questions=questions)
            return resultat
        repondues = {r["question"].strip() for r in reponses if not _du_code(r)}
        reposees = [q for q in questions if q["question"].strip() in repondues]
        questions = [q for q in questions if q["question"].strip() not in repondues]
        if questions:
            resultat.update(etat=QUESTIONS, questions=questions)
            return resultat
        if reposees and not any(n == "proposer_jeu" for _, n, _ in appels):
            resultat.update(etat=ERREURS, erreurs=["Mistral repose des questions auxquelles le juriste a déjà "
                                                   "répondu : " + " | ".join(q["question"] for q in reposees)])
            return resultat
        propositions = [a for _, n, a in appels if n == "proposer_jeu"]
        if not propositions:
            fin = (reponse.get("choices") or [{}])[0].get("finish_reason") if isinstance(reponse, dict) else None
            resultat.update(etat=ERREURS, erreurs=["Mistral n'a appelé aucun des trois outils" +
                                                   (" (fin de génération : %s)" % fin if fin else "")])
            return resultat

        erreurs = []
        if len(propositions) > 1:
            erreurs.append("un seul jeu par proposition (%d reçus) : réunis les directives dans un seul appel"
                           % len(propositions))
        brutes = propositions[0].get("directives") or []
        if not isinstance(brutes, list) or not brutes:
            erreurs.append("proposer_jeu : aucune directive")
            brutes = []
        directives = []
        for i, p in enumerate(brutes, 1):
            if not isinstance(p, dict):
                erreurs.append("directive %d : objet attendu" % i)
                continue
            id_directive = "%s-%03d" % (prefixe_id, i)
            prefixe = "directive %d (%s) : " % (i, id_directive)
            d, err = construire_directive(p, texte, id_directive, date_effet, prefixe)
            erreurs += err
            erreurs += verifications_v2.verifier_directive(p, d, vocab, source, prefixe)
            directives.append(d)
        jeu = {"jeu": {"id": jeu_id, "version": 1, "date_effet": date_effet, "auteur": auteur},
               "directives": directives}
        ok, err_go = True, []
        if directives:
            ok, err_go, _ = directive_go.verifier(_avec_provisoire(jeu))
        erreurs += err_go
        resultat["jeu"] = jeu
        if not erreurs and ok:
            return _conclure(resultat, jeu, reponses, auteur)
        resultat["erreurs"] = erreurs
        if tentative == 2:
            break
        # Une seule correction : les erreurs retournent à Mistral, telles quelles.
        messages.append({"role": "assistant", "content": message.get("content") or "",
                         "tool_calls": message["tool_calls"]})
        for i, nom, _ in appels:
            contenu = ("Refusé par le vérificateur :\n- " + "\n- ".join(erreurs) +
                       "\nCorrige le jeu, ou pose une question si une information manque."
                       if nom == "proposer_jeu" else "Ignoré.")
            messages.append({"role": "tool", "tool_call_id": i, "name": nom, "content": contenu})

    resultat.update(etat=REFUSE, jeu_refuse=resultat["jeu"], jeu=None, relecture=None)
    return resultat


def _conclure(resultat, jeu, reponses, auteur):
    complet, restantes, consignation = appliquer_reponses(jeu, reponses, auteur)
    resultat["consignation"] = consignation
    if restantes:
        resultat.update(etat=QUESTIONS, questions=restantes, jeu=None)
        return resultat
    ok, err_go, _ = directive_go.verifier(complet)
    if not ok:
        resultat.update(etat=REFUSE, erreurs=err_go, jeu_refuse=complet, jeu=None)
        return resultat
    resultat.update(etat=JEU, erreurs=[], jeu=complet, relecture=directive_go.relire(complet))
    return resultat
