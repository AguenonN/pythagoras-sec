"""Détecteurs déterministes : des constats datés et sourcés, sans modèle.

Chaque détecteur parcourt les sessions (``journaux.Session``) et rend des
**occurrences** : un dict par fait relevé, qui cite la session, les numéros
d'entrée (``seq``) et les outils. Les occurrences d'un même risque sont
regroupées en un constat par ``propositions.regrouper``.

Une occurrence porte deux sortes de champs :

- ceux que le modèle peut voir (outils, numéros, dates, trimestres, effets,
  directives) ;
- ``valeurs_coffre`` : les arguments d'origine qui ont servi à la
  détection (SIREN, montant). Ils restent dans le constat pour le juriste,
  jamais dans la vue du modèle (``redaction.vue_modele``).

Une occurrence déjà couverte par une directive appliquée à l'appel n'est
pas rendue : c'est un risque que les directives en vigueur à ce moment
traitaient déjà.
"""

import json
import os
import urllib.parse

from . import couverture, journaux

ICI = os.path.dirname(os.path.abspath(__file__))
RACINE = os.path.dirname(os.path.dirname(ICI))
CATALOGUE = os.path.join(ICI, "sources.json")

SOURCE_INTERNE_PERIMEE = "source_interne_perimee"
ACTION_SANS_CONTROLE = "action_sans_controle_humain"
DONNEES_PERSONNELLES_EXTERNE = "donnees_personnelles_vers_externe"
ENTREPRISE_NON_RETROUVEE = "entreprise_non_retrouvee"

TYPES = (SOURCE_INTERNE_PERIMEE, ACTION_SANS_CONTROLE, DONNEES_PERSONNELLES_EXTERNE, ENTREPRISE_NON_RETROUVEE)


def lire_catalogue(chemin=CATALOGUE):
    with open(chemin, encoding="utf-8") as f:
        return json.load(f)


def _chemin(p, racine):
    return p if os.path.isabs(p) else os.path.join(racine, p)


# -- outils communs ----------------------------------------------------------

def trimestre(date):
    """``AAAA-Qn`` d'un datetime (UTC)."""
    return "%04d-Q%d" % (date.year, (date.month - 1) // 3 + 1)


def _cle_trimestre(t):
    try:
        annee, q = t.split("-Q")
        return int(annee), int(q)
    except (ValueError, AttributeError):
        return None


def pointeur(objet, ptr):
    """Pointeur JSON (RFC 6901) ; ``None`` si absent."""
    if ptr == "":
        return objet
    cur = objet
    for morceau in ptr.lstrip("/").split("/"):
        morceau = morceau.replace("~1", "/").replace("~0", "~")
        if isinstance(cur, dict) and morceau in cur:
            cur = cur[morceau]
        elif isinstance(cur, list) and morceau.isdigit() and int(morceau) < len(cur):
            cur = cur[int(morceau)]
        else:
            return None
    return cur


def _decision(appel):
    return {"effet": appel.effet, "directives_appliquees": appel.directives_appliquees(),
            "validation_humaine": "acceptee" if appel.validee_par_humain else "aucune"}


def _base(session, action):
    return {"session": session.nom, "seq_action": action.seq, "action": action.nom,
            "date_action": action.ts, "transmise": action.transmis, "decision": _decision(action)}


def _coffre(action, **valeurs):
    """Valeurs du coffre gardées pour le juriste : celles qui ont servi à la
    détection, et le dossier visé s'il est nommé."""
    out = dict(valeurs)
    dossier = action.arguments.get("dossier_id")
    if isinstance(dossier, str) and dossier:
        out["dossier"] = dossier
    return out


# -- 1. source interne périmée ------------------------------------------------

def periode_interne(catalogue, racine=RACINE):
    """Trimestre de la base interne, lu là où le serveur le lit, et le
    fichier qui le dit (pour sourcer le constat)."""
    p = catalogue["taux"]["interne"]["periode"]
    chemin = _chemin(p["fichier"], racine)
    with open(chemin, encoding="utf-8") as f:
        valeur = json.load(f).get(p["champ"])
    if _cle_trimestre(valeur) is None:
        raise ValueError("période illisible dans %s (%s)" % (p["fichier"], p["champ"]))
    return valeur, p["fichier"] + " (" + p["champ"] + ")"


def source_interne_perimee(sessions, catalogue, racine=RACINE, jeu_courant=None):
    """Une action transmise dans une session qui a consulté la base interne
    des taux, dont le trimestre est antérieur à celui d'une source officielle
    consultée dans la session (``source.date``) ou, à défaut, au trimestre
    en vigueur à la date de l'action."""
    interne = catalogue["taux"]["interne"]["outil"]
    officielle = catalogue["taux"]["officielle"]["serveur"]
    p_interne, origine_periode = periode_interne(catalogue, racine)
    out = []
    for s in sessions:
        for a in s.actions():
            if not a.transmis:
                continue
            avant = s.consultations_avant(a.seq)
            internes = [c for c in avant if c.nom == interne]
            if not internes or couverture.couvert_pendant(s, a, ["outil:" + interne], jeu_courant):
                continue
            officielles = []
            for c in avant:
                if c.serveur != officielle:
                    continue
                d = journaux.lire_date(c.source.get("date", ""))
                officielles.append({"seq": c.seq, "outil": c.nom, "date_source": c.source.get("date", ""),
                                    "trimestre": trimestre(d) if d else None})
            references = [o["trimestre"] for o in officielles if o["trimestre"]]
            date_action = a.date()
            en_vigueur = trimestre(date_action) if date_action else None
            if references:
                reference, origine_ref = max(references, key=_cle_trimestre), "source officielle consultée"
            elif en_vigueur:
                reference, origine_ref = en_vigueur, "trimestre en vigueur à la date de l'action"
            else:
                continue
            if _cle_trimestre(p_interne) >= _cle_trimestre(reference):
                continue
            o = _base(s, a)
            o.update({
                "consultation_interne": {"seq": internes[0].seq, "outil": interne, "trimestre": p_interne,
                                         "origine_trimestre": origine_periode},
                "source_officielle_consultee": bool(officielles),
                "consultations_officielles": officielles,
                "trimestre_reference": reference,
                "origine_reference": origine_ref,
                "valeurs_coffre": _coffre(a),
            })
            out.append(o)
    return out


# -- 2. action sans contrôle humain au-delà d'un montant ---------------------

def action_sans_controle(sessions, catalogue, seuil=None):
    """Une action transmise, d'un montant au moins égal au seuil, sans
    validation humaine et sans directive appliquée."""
    ptr = catalogue["montant"]["argument"]
    if seuil is None:
        seuil = catalogue["montant"]["seuil_defaut"]
    out = []
    for s in sessions:
        for a in s.actions():
            montant = pointeur(a.arguments, ptr)
            if isinstance(montant, bool) or not isinstance(montant, (int, float)):
                continue
            if not a.transmis or a.validee_par_humain or a.directives_appliquees() or a.effet != "autoriser":
                continue
            if montant < seuil:
                continue
            o = _base(s, a)
            o.update({"seuil": seuil, "champ_montant": ptr, "valeurs_coffre": _coffre(a, montant=montant)})
            out.append(o)
    return out


# -- 3. données personnelles puis action vers l'extérieur --------------------

def donnees_personnelles_externe(sessions, catalogue, jeu_courant=None):
    """Une consultation de données personnelles (données reçues) suivie,
    dans la même session, d'une action vers une destination étiquetée
    externe, qu'aucune directive appliquée n'a retenue."""
    perso = catalogue["etiquettes"]["donnees_personnelles"]
    externe = catalogue["etiquettes"]["externe"]
    out = []
    for s in sessions:
        for a in s.actions():
            if externe not in a.etiquettes_cible:
                continue
            sources = [c for c in s.appels if c.seq is not None and c.seq < a.seq and c.donnees_recues
                       and perso in _etiquettes_consultation(s, c)]
            if not sources:
                continue
            elements = ["etiquette:" + perso] + ["outil:" + c.nom for c in sources] + \
                ["serveur:" + c.serveur for c in sources]
            if couverture.couvert_pendant(s, a, elements, jeu_courant):
                continue
            o = _base(s, a)
            o.update({"consultations_personnelles": [{"seq": c.seq, "outil": c.nom} for c in sources],
                      "etiquettes_destination": sorted(a.etiquettes_cible), "valeurs_coffre": _coffre(a)})
            out.append(o)
    return out


def _etiquettes_consultation(session, appel):
    # L'entrée « appel » porte les étiquettes de la cible au moment de
    # l'appel (registre) : ce sont celles de la consultation.
    return appel.etiquettes_cible


# -- 4. entreprise non retrouvée au répertoire SIRENE ------------------------

class Repertoire:
    """Les réponses datées du répertoire des entreprises que le serveur a
    servies (enregistrements), par SIREN recherché."""

    def __init__(self, dossiers):
        self.reponses = {}
        for d in dossiers:
            try:
                noms = sorted(os.listdir(d))
            except FileNotFoundError:
                continue
            for n in noms:
                if not n.endswith(".json"):
                    continue
                try:
                    with open(os.path.join(d, n), encoding="utf-8") as f:
                        e = json.load(f)
                    corps = json.loads(e["corps"])
                    q = urllib.parse.parse_qs(urllib.parse.urlparse(e["url"]).query).get("q", [""])[0]
                except (ValueError, KeyError, TypeError):
                    continue
                if not q:
                    continue
                sirens = [r.get("siren") for r in corps.get("results") or [] if isinstance(r, dict)]
                self.reponses.setdefault(q, []).append({
                    "date": e.get("date", ""), "fichier": os.path.relpath(os.path.join(d, n), RACINE),
                    "trouvee": q in sirens, "nombre_resultats": corps.get("total_results", len(sirens))})

    def chercher(self, siren, date_source):
        """La réponse servie à cette consultation : même SIREN, même date
        que la ``source.date`` consignée par le gate ; ``None`` si aucune."""
        for r in self.reponses.get(siren, []):
            if journaux.lire_date(r["date"]) == journaux.lire_date(date_source):
                return r
        return None


def entreprise_non_retrouvee(sessions, catalogue, racine=RACINE, jeu_courant=None):
    """Une action qui désigne une entreprise (argument SIREN) que la session
    n'a pas retrouvée au répertoire : recherchée sans résultat, ou jamais
    recherchée. Une recherche dont la réponse n'est pas relisible n'est pas
    un constat (``non_verifiables``)."""
    rep = catalogue["repertoire_entreprises"]
    ptr = rep["argument"]
    repertoire = Repertoire([_chemin(d, racine) for d in rep["enregistrements"]])
    out, non_verifiables = [], []
    for s in sessions:
        for a in s.actions():
            siren = pointeur(a.arguments, ptr)
            if not isinstance(siren, str) or not siren:
                continue
            if couverture.couvert_pendant(s, a, elements_repertoire(catalogue), jeu_courant):
                continue
            recherches = [c for c in s.appels if c.seq is not None and c.seq < a.seq and c.nom == rep["outil"]
                          and pointeur(c.arguments, ptr) == siren and c.transmis]
            constats, verifiable = [], True
            for c in recherches:
                r = repertoire.chercher(siren, c.source.get("date", "")) if c.donnees_recues else None
                if r is None:
                    verifiable = False
                    continue
                constats.append((c, r))
            if any(r["trouvee"] for _, r in constats):
                continue
            if recherches and not constats:
                non_verifiables.append({"session": s.nom, "seq_action": a.seq})
                continue
            o = _base(s, a)
            o.update({
                "motif": "introuvable" if recherches else "non_recherchee",
                "recherches": [{"seq": c.seq, "outil": c.nom, "date_source": c.source.get("date", ""),
                                "nombre_resultats": r["nombre_resultats"], "enregistrement": r["fichier"]}
                               for c, r in constats],
                "recherches_complete": verifiable,
                "valeurs_coffre": _coffre(a, siren=siren),
            })
            out.append(o)
    return out, non_verifiables


def elements_repertoire(catalogue):
    outil = catalogue["repertoire_entreprises"]["outil"]
    return ["outil:" + outil, "serveur:" + outil.split("/")[0]]


def detecter(sessions, catalogue=None, racine=RACINE, seuil_montant=None, jeu_courant=None):
    """Toutes les occurrences, par type, et les cas non vérifiables.
    ``jeu_courant`` : le jeu lu quand une session n'a pas gardé le sien."""
    catalogue = catalogue or lire_catalogue()
    sirene, non_verifiables = entreprise_non_retrouvee(sessions, catalogue, racine, jeu_courant)
    occurrences = {
        SOURCE_INTERNE_PERIMEE: source_interne_perimee(sessions, catalogue, racine, jeu_courant),
        ACTION_SANS_CONTROLE: action_sans_controle(sessions, catalogue, seuil_montant),
        DONNEES_PERSONNELLES_EXTERNE: donnees_personnelles_externe(sessions, catalogue, jeu_courant),
        ENTREPRISE_NON_RETROUVEE: sirene,
    }
    return {t: dedoublonner(o) for t, o in occurrences.items()}, {ENTREPRISE_NON_RETROUVEE: non_verifiables}


def dedoublonner(occurrences):
    """Une action reprise (attente, puis jeton) est présentée plusieurs fois
    à l'engine : une seule occurrence par session, outil et valeurs, la
    dernière présentée (celle qui dit ce qui a finalement été décidé)."""
    garde = {}
    for o in occurrences:
        cle = (o["session"], o["action"], json.dumps(o["valeurs_coffre"], sort_keys=True))
        garde[cle] = o
    return sorted(garde.values(), key=lambda o: (o["session"], o["seq_action"]))
