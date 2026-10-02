"""Le compilateur appelé par la console (contrats de la démo, §4).

    cd agents
    python -m compilateur.service [--mode rejeu|reel] [--modele M] < requete.json > reponse.json

Lit **une** requête JSON sur l'entrée standard, écrit **une** réponse JSON
sur la sortie standard.

Requête ::

    {"conversation": "<id ou null>", "texte": "...", "fondement": "...", "auteur": "juriste@exemple.test",
     "reponses": [{"question": "<id>", "reponse": "..."}],
     "jeu_existant": "demo/directives/demo.json", "date_effet": "2026-10-04T11:05:00+02:00"}

Réponse ::

    {"conversation": "<id>", "etat": "questions"|"non_observable"|"jeu"|"refuse"|"erreurs",
     "questions": [{"id", "texte", "options": [...], "origine": "mistral"|"code"}],
     "non_observable": {"extrait", "raison"} | null,
     "relecture": "<texte des seules directives ajoutées>",
     "jeu_candidat": "demo/execution/compilation/<id>/demo.vN.json" | null,
     "erreurs": [...]}

L'état d'une conversation vit dans ``demo/execution/compilation/<id>/`` :
``conversation.json`` (la requête initiale, les questions posées et leurs
identifiants, les réponses reçues, les tours) et ``appels/`` (chaque réponse
de Mistral obtenue pour cette conversation). Rappeler le service avec des
réponses ne refait aucun appel déjà fait : une requête à Mistral déjà
posée dans la conversation est servie depuis ``appels/``, quel que soit le
mode.

``jeu_candidat`` est la version n+1 du jeu existant (contrats §1) : ses
directives inchangées, puis les ajoutées en brouillon, numérotées à la
suite ; il est accepté par ``directive verifier``. La relecture ne montre que
les directives ajoutées.

Les chemins de la requête et de la réponse sont relatifs à la racine du
dépôt (un chemin absolu est admis en entrée).

Mode : ``rejeu`` (défaut : uniquement les réponses enregistrées, jamais de
réseau) ou ``reel`` (les enregistrements d'abord, l'API sinon) ; aussi par
la variable ``COMPILATEUR_MODE``. Modèle : ``--modele`` ou
``COMPILATEUR_MODELE`` ; une conversation garde le modèle de son premier tour.

Codes de sortie : 0 pour toute réponse du contrat, y compris
``etat: erreurs`` quand la compilation n'aboutit pas (Mistral injoignable,
proposition inexploitable) ; 2 si la requête est invalide (la réponse JSON,
``etat: erreurs``, est écrite quand même).
"""

import argparse
import datetime
import hashlib
import json
import os
import secrets
import sys

from . import compilateur_v2 as c2
from . import directive_go, mistral, vocabulaire
from .catalogue import CatalogueInvalide

ICI = os.path.dirname(os.path.abspath(__file__))
RACINE = directive_go.RACINE
DOSSIER = os.path.join("demo", "execution", "compilation")
CATALOGUE = os.path.join(ICI, "exemples", "catalogue-demo.json")
VOCABULAIRE = os.path.join(ICI, "exemples", "vocabulaire-demo.json")

ETATS = (c2.QUESTIONS, c2.NON_OBSERVABLE, c2.JEU, c2.REFUSE, c2.ERREURS)
CHAMPS_REQUETE = ("conversation", "texte", "fondement", "auteur", "reponses", "jeu_existant", "date_effet")


class RequeteInvalide(Exception):
    pass


# -- chemins et fichiers -----------------------------------------------------

def chemin_absolu(chemin, racine=RACINE):
    return chemin if os.path.isabs(chemin) else os.path.join(racine, chemin)


def chemin_rendu(chemin, racine=RACINE):
    """Relatif à la racine s'il est dessous, absolu sinon."""
    rel = os.path.relpath(chemin, racine)
    return chemin if rel.startswith("..") else rel.replace(os.sep, "/")


def ecrire_atomique(chemin, donnees):
    tmp = chemin + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(donnees, f, ensure_ascii=False, indent=2)
        f.write("\n")
    os.replace(tmp, chemin)


def lire_json(chemin):
    with open(chemin, encoding="utf-8") as f:
        return json.load(f)


# -- client mémorisé par conversation ----------------------------------------

class ClientConversation:
    """Sert depuis ``appels/`` toute requête déjà posée dans la conversation ;
    sinon la confie au client Mistral et garde sa réponse."""

    def __init__(self, client, dossier, modele):
        self.client = client
        self.dossier = dossier
        self.modele = modele
        self.nouveaux = 0

    def completer(self, messages, outils, **options):
        cle = mistral.empreinte({"messages": messages, "outils": outils, "options": options,
                                 "modele": self.modele})
        chemin = os.path.join(self.dossier, cle + ".json")
        if os.path.exists(chemin):
            return lire_json(chemin)["reponse"]
        reponse = self.client.completer(messages, outils, **options)
        os.makedirs(self.dossier, exist_ok=True)
        ecrire_atomique(chemin, {"reponse": reponse})
        self.nouveaux += 1
        return reponse


# -- questions ---------------------------------------------------------------

def id_question(q):
    """Identifiant stable : le champ d'une question du code, une empreinte du
    texte d'une question de Mistral (même question, même identifiant)."""
    if q.get("origine") == c2.ORIGINE_CODE:
        return q["champ"]
    return "m-" + hashlib.sha256(q["question"].strip().encode("utf-8")).hexdigest()[:12]


def question_rendue(q):
    return {"id": id_question(q), "texte": q["question"], "options": list(q.get("options") or []),
            "origine": "code" if q.get("origine") == c2.ORIGINE_CODE else "mistral"}


def entree_reponse(q, reponse):
    """L'entrée que compiler_jeu attend pour une réponse à la question q."""
    e = {"question": q["question"], "reponse": reponse}
    if q.get("origine") == c2.ORIGINE_CODE:
        e.update(champ=q["champ"], options=list(q["options"]), origine=q["origine"])
    return e


# -- service -----------------------------------------------------------------

def _texte(v, nom, obligatoire=True):
    if v is None or (isinstance(v, str) and not v.strip()):
        if obligatoire:
            raise RequeteInvalide("%s manquant" % nom)
        return None
    if not isinstance(v, str):
        raise RequeteInvalide("%s : chaîne attendue" % nom)
    return v.strip()


def _date_effet(v):
    v = _texte(v, "date_effet")
    try:
        datetime.datetime.fromisoformat(v.replace("Z", "+00:00"))
    except ValueError:
        raise RequeteInvalide("date_effet : horodatage RFC 3339 attendu (%r)" % v) from None
    if "T" not in v:
        raise RequeteInvalide("date_effet : horodatage RFC 3339 complet attendu (%r)" % v)
    return v


def nouvel_id(maintenant=None):
    t = maintenant or datetime.datetime.now()
    return t.strftime("%Y%m%d-%H%M%S") + "-" + secrets.token_hex(3)


def _reponse(conversation, etat, questions=(), non_observable=None, relecture="", jeu_candidat=None, erreurs=()):
    return {"conversation": conversation, "etat": etat, "questions": list(questions),
            "non_observable": non_observable, "relecture": relecture, "jeu_candidat": jeu_candidat,
            "erreurs": list(erreurs)}


class Service:
    def __init__(self, mode="rejeu", modele=mistral.MODELE, racine=RACINE, dossier=None,
                 catalogue=CATALOGUE, vocabulaire_=VOCABULAIRE, client=None, fuseaux=None):
        """``client`` : fabrique (modele) → client Mistral, pour les tests ;
        par défaut, mistral.Client dans le mode donné."""
        self.mode = mode
        self.modele = modele
        self.racine = racine
        self.dossier = dossier or os.path.join(racine, DOSSIER)
        self.catalogue = catalogue
        self.vocabulaire = vocabulaire_
        self.fabrique = client or (lambda m: mistral.Client(mode=self.mode, modele=m))
        self.fuseaux = fuseaux

    def traiter(self, requete):
        """Rend (réponse, code de sortie)."""
        conv = requete.get("conversation") if isinstance(requete, dict) else None
        try:
            if not isinstance(requete, dict):
                raise RequeteInvalide("objet JSON attendu")
            inconnus = sorted(set(requete) - set(CHAMPS_REQUETE))
            if inconnus:
                raise RequeteInvalide("champs inconnus : " + ", ".join(inconnus))
            etat, dossier = self._etat(requete)
            conv = etat["id"]
            erreurs = self._recevoir(etat, requete.get("reponses") or [])
            if erreurs:
                self._sauver(dossier, etat)
                return _reponse(conv, c2.ERREURS, erreurs=erreurs), 2
        except RequeteInvalide as e:
            return _reponse(conv, c2.ERREURS, erreurs=[str(e)]), 2
        return self._compiler(etat, dossier), 0

    # -- état de la conversation --

    def _etat(self, requete):
        conv = requete.get("conversation")
        if conv is None:
            jeu = _texte(requete.get("jeu_existant"), "jeu_existant")
            etat = {
                "id": nouvel_id(),
                "texte": _texte(requete.get("texte"), "texte"),
                "fondement": _texte(requete.get("fondement"), "fondement", obligatoire=False),
                "auteur": _texte(requete.get("auteur"), "auteur"),
                "jeu_existant": jeu,
                "date_effet": _date_effet(requete.get("date_effet")),
                "modele": self.modele,
                "questions": {},
                "reponses": [],
                "tours": [],
            }
            dossier = os.path.join(self.dossier, etat["id"])
            os.makedirs(dossier)
            return etat, dossier
        if not isinstance(conv, str) or not conv or "/" in conv or "\\" in conv or conv.startswith("."):
            raise RequeteInvalide("conversation : identifiant invalide")
        dossier = os.path.join(self.dossier, conv)
        try:
            etat = lire_json(os.path.join(dossier, "conversation.json"))
        except FileNotFoundError:
            raise RequeteInvalide("conversation %s inconnue" % conv) from None
        # Une conversation porte un seul texte : en changer, c'en est une autre.
        for cle in ("texte", "fondement", "auteur", "jeu_existant", "date_effet"):
            v = requete.get(cle)
            if v is not None and (v.strip() if isinstance(v, str) else v) != etat[cle]:
                raise RequeteInvalide("%s différent de celui de la conversation %s : ouvrir une nouvelle conversation"
                                      % (cle, conv))
        return etat, dossier

    def _recevoir(self, etat, reponses):
        if not isinstance(reponses, list):
            return ["reponses : liste attendue"]
        erreurs = []
        for r in reponses:
            if not isinstance(r, dict) or not isinstance(r.get("question"), str) or not isinstance(r.get("reponse"), str):
                erreurs.append("reponses : {\"question\": <id>, \"reponse\": <texte>} attendu")
                continue
            q = etat["questions"].get(r["question"])
            if q is None:
                erreurs.append("réponse à une question inconnue de la conversation : %s" % r["question"])
                continue
            texte = r["reponse"].strip()
            if not texte:
                erreurs.append("réponse vide à la question %s" % r["question"])
                continue
            if q.get("origine") == c2.ORIGINE_CODE and \
                    c2.reponse_du_code([entree_reponse(q, texte)], q["champ"], q["options"]) is None:
                erreurs.append("question %s : réponse attendue parmi %s" % (r["question"], ", ".join(q["options"])))
                continue
            # Une nouvelle réponse à la même question remplace l'ancienne.
            etat["reponses"] = [e for e in etat["reponses"] if e["id"] != r["question"]]
            etat["reponses"].append(dict(entree_reponse(q, texte), id=r["question"]))
        return erreurs

    def _sauver(self, dossier, etat):
        ecrire_atomique(os.path.join(dossier, "conversation.json"), etat)

    # -- compilation --

    def _compiler(self, etat, dossier):
        conv = etat["id"]
        chemin_jeu = chemin_absolu(etat["jeu_existant"], self.racine)
        try:
            existant = lire_json(chemin_jeu)
            ok, err, _ = directive_go.verifier(existant, exiger_brouillon=False)
            if not ok:
                return self._fin(dossier, etat, _reponse(conv, c2.ERREURS, erreurs=[
                    "jeu existant %s refusé par le chargeur : %s" % (etat["jeu_existant"], "; ".join(err))]))
            vocab = vocabulaire.Vocabulaire.lire(self.catalogue, self.vocabulaire, fuseaux=self.fuseaux)
            client = ClientConversation(self.fabrique(etat["modele"]), os.path.join(dossier, "appels"), etat["modele"])
            reponses = [{k: v for k, v in e.items() if k != "id"} for e in etat["reponses"]]
            r = c2.compiler_jeu(etat["texte"], existant["jeu"]["id"], vocab, client, etat["auteur"],
                                etat["date_effet"], reponses=reponses, existant=existant,
                                fondement=etat["fondement"])
        except (OSError, ValueError, CatalogueInvalide, mistral.ErreurMistral, directive_go.ErreurOutil) as e:
            return self._fin(dossier, etat, _reponse(conv, c2.ERREURS, erreurs=["%s : %s" % (type(e).__name__, e)]))

        questions = [question_rendue(q) for q in r["questions"]]
        for q in r["questions"]:
            etat["questions"][id_question(q)] = q
        non_obs = None
        if r["non_observables"]:
            n = r["non_observables"][0]
            non_obs = {"extrait": n["extrait"], "raison": n["raison"]}
        relecture, candidat = "", None
        if r["etat"] == c2.JEU:
            jeu = r["jeu"]
            nom = "%s.v%d.json" % (jeu["jeu"]["id"], jeu["jeu"]["version"])
            chemin = os.path.join(dossier, nom)
            ecrire_atomique(chemin, jeu)
            relecture, candidat = r["relecture"], chemin_rendu(chemin, self.racine)
        etat["tours"].append({"etat": r["etat"], "questions": [q["id"] for q in questions],
                              "appels_nouveaux": client.nouveaux, "ajoutees": r.get("ajoutees", []),
                              "jeu_candidat": candidat, "erreurs": r["erreurs"]})
        return self._fin(dossier, etat, _reponse(conv, r["etat"], questions, non_obs, relecture, candidat, r["erreurs"]))

    def _fin(self, dossier, etat, reponse):
        self._sauver(dossier, etat)
        return reponse


# -- parcours scripté (tests, enregistrements) -------------------------------

def parcours(service, requete, reponses, tours_max=6):
    """Déroule une conversation : ``requete`` initiale, puis réponse à chaque
    question posée, trouvée dans ``reponses`` par le texte exact d'une
    question de Mistral ou par l'identifiant d'une question du code. Rend la
    liste des réponses du service ; s'arrête hors questions, ou sur une
    question sans réponse."""
    sorties = []
    rep, _ = service.traiter(dict(requete, conversation=None, reponses=[]))
    sorties.append(rep)
    for _ in range(tours_max):
        if rep["etat"] not in (c2.QUESTIONS, c2.NON_OBSERVABLE):
            break
        envoi = []
        for q in rep["questions"]:
            texte = reponses.get(q["texte"].strip()) if q["origine"] == "mistral" else reponses.get(q["id"])
            if texte is None:
                return sorties
            envoi.append({"question": q["id"], "reponse": texte})
        rep, _ = service.traiter({"conversation": rep["conversation"], "reponses": envoi})
        sorties.append(rep)
    return sorties


def lire_parcours(chemin):
    """Un parcours scripté (``exemples/service/<nom>.json``) ::

        {"requete": {<requête initiale du contrat §4, sans conversation ni reponses>},
         "reponses": {"<texte exact d'une question de Mistral>": "...", "<id d'une question du code>": "..."}}
    """
    p = lire_json(chemin)
    return p["requete"], p.get("reponses") or {}


def principal(argv=None, entree=sys.stdin, sortie=sys.stdout):
    ap = argparse.ArgumentParser(prog="compilateur.service", description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--parcours", help="déroule un parcours scripté (exemples/service/<nom>.json) au lieu de lire "
                                       "une requête : chaque réponse du service est écrite, une par ligne")
    ap.add_argument("--mode", choices=["rejeu", "reel"], default=os.environ.get("COMPILATEUR_MODE", "rejeu"))
    ap.add_argument("--modele", choices=list(mistral.MODELES),
                    default=os.environ.get("COMPILATEUR_MODELE", mistral.MODELE))
    ap.add_argument("--dossier", help="dossier des conversations (défaut : <racine>/%s)" % DOSSIER)
    ap.add_argument("--catalogue", default=CATALOGUE)
    ap.add_argument("--vocabulaire", default=VOCABULAIRE)
    a = ap.parse_args(argv)
    service = Service(mode=a.mode, modele=a.modele, dossier=a.dossier, catalogue=a.catalogue,
                      vocabulaire_=a.vocabulaire)
    if a.parcours:
        requete, reponses = lire_parcours(a.parcours)
        sorties = parcours(service, requete, reponses)
        for rep in sorties:
            sortie.write(json.dumps(rep, ensure_ascii=False) + "\n")
        return 0 if sorties[-1]["etat"] == c2.JEU else 1
    try:
        requete = json.loads(entree.read())
    except ValueError as e:
        rep, code = _reponse(None, c2.ERREURS, erreurs=["requête JSON illisible : %s" % e]), 2
    else:
        rep, code = service.traiter(requete)
    json.dump(rep, sortie, ensure_ascii=False, indent=2)
    sortie.write("\n")
    return code


if __name__ == "__main__":
    sys.exit(principal())
