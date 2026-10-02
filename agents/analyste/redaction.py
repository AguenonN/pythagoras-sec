"""Mistral rédige, à partir d'un constat seulement.

Le modèle reçoit la **vue** du constat (``vue_modele``) : type, action,
sessions, numéros d'entrée, dates, trimestres, effets et directives. Il ne
voit ni le ``detail`` (écrit pour le juriste, il cite des arguments), ni les
``valeurs_coffre`` (SIREN, montants) : aucun argument du coffre n'entre dans
une requête.

Il rend, par un appel de fonction, un texte de directive en français, un
fondement proposé et l'explication du risque avec les sessions citées. Le
code vérifie ensuite : textes non vides, aucune session inventée, aucune
valeur du coffre recopiée (elle n'a pas pu la voir, mais la vérification ne
coûte rien) ; il marque le fondement « à valider par le juriste » et
complète l'explication par les sessions qu'elle ne cite pas.
"""

import json
import os
import re

from compilateur import mistral

from . import detecteurs as det

ICI = os.path.dirname(os.path.abspath(__file__))
RACINE = os.path.dirname(os.path.dirname(ICI))
ENREGISTREMENTS = os.path.join(ICI, "enregistrements")
ETAT = os.path.join(os.path.dirname(ICI), ".etat", "analyste")
ANNUAIRE = os.path.join(RACINE, "demo", "annuaire-source.json")
MAX_APPELS = 15  # budget du brief E
A_VALIDER = "à valider par le juriste"

NATURES = {
    det.SOURCE_INTERNE_PERIMEE: (
        "Une action a été transmise dans une session qui avait consulté une source interne de la banque dont la "
        "période (trimestre) est antérieure à celle d'une source officielle consultée, ou à défaut au trimestre en "
        "vigueur à la date de l'action. La décision a pu se fonder sur une valeur périmée."),
    det.ACTION_SANS_CONTROLE: (
        "Une action d'un montant élevé a été transmise sans aucune validation humaine et sans qu'aucune directive "
        "ne s'applique."),
    det.DONNEES_PERSONNELLES_EXTERNE: (
        "Des données personnelles ont été consultées dans une session, puis une action a été dirigée vers une "
        "destination étiquetée externe : les données ont pu sortir de l'organisation."),
    det.ENTREPRISE_NON_RETROUVEE: (
        "Une action (attribution) a été présentée pour une entreprise que la session n'a pas retrouvée au "
        "répertoire officiel des entreprises (SIRENE) : recherchée sans résultat, ou jamais recherchée. Rien "
        "n'établit que l'entreprise existe."),
}

SYSTEME = """Tu assistes le juriste d'une banque qui encadre des agents d'IA par des directives. Un \
contrôleur (le gate) applique ces directives à chaque appel d'outil d'un agent.

On te donne un CONSTAT établi par du code sur des exécutions passées : un risque que les directives en \
vigueur ne couvrent pas. Tu rédiges, pour le juriste, une PROPOSITION de directive. Il la rejettera, la \
modifiera ou la reprendra : tu ne décides de rien.

Ce que le gate sait observer au moment d'un appel, et rien d'autre :
- l'outil appelé (serveur/outil) et s'il s'agit d'une action ou d'une lecture ;
- les outils déjà appelés dans la même demande (session), avec ou sans données reçues ;
- les arguments de l'appel (par exemple un montant) ;
- les étiquettes des serveurs (par exemple donnees_personnelles, externe) ;
- le demandeur et ses groupes, la date et l'heure.
Il ne lit pas le contenu des réponses des serveurs.

Effets possibles : refuser l'appel ; le mettre en attente de la validation humaine d'un rôle ; notifier un \
rôle.

Règles de rédaction :
- texte_propose : UNE seule règle, en français courant, comme l'écrirait le juriste, deux phrases au plus. \
Elle nomme l'action visée, la condition observable qui la déclenche et l'effet. La condition est la plus \
simple qui couvre TOUTES les occurrences : reprends le fait observable commun donné dans le contexte, sans \
rien y ajouter (ni ordre des appels, ni autre outil). Nomme les outils par leur nom serveur/outil tel \
qu'il figure dans le constat. Préfère la validation humaine au refus quand une personne peut vérifier ce \
que l'agent n'a pas vérifié ; le refus quand l'action est interdite en soi. Pour une validation humaine, \
UN seul rôle : de préférence un rôle qui a déjà validé cette action (contexte), jamais un rôle sans \
rapport avec l'action. Ne cite aucun nom de session dans la règle.
- fondement_propose : reprends le fondement de la directive existante (contexte) dont le SUJET est le \
plus proche du risque (lis son texte : gels, usure, montant…) ; sinon un texte de droit que tu connais \
avec certitude. N'invente jamais de référence (procédure, numéro de politique, article incertain). Il \
sera validé par le juriste.
- explication : le risque, en trois phrases au plus, en citant les sessions du constat par leur nom exact. \
Sois fidèle aux occurrences : une action dont « transmise » vaut false n'a pas été exécutée ; dis quelle \
décision l'engine a prise.
- Si le contexte donne une limite d'observation : écris la règle voulue, et dis dans l'explication, en \
une phrase, que l'engine ne voit pas ce fait aujourd'hui.
- sessions_citees : les noms exacts des sessions que l'explication cite, pris dans le constat."""

OUTIL = {
    "type": "function",
    "function": {
        "name": "proposer_directive",
        "description": "Proposer au juriste un texte de directive fondé sur le constat.",
        "parameters": {
            "type": "object",
            "properties": {
                "texte_propose": {"type": "string", "description": "La règle, en français."},
                "fondement_propose": {"type": "string", "description": "Fondement juridique proposé."},
                "explication": {"type": "string", "description": "Le risque, avec les sessions citées."},
                "sessions_citees": {"type": "array", "items": {"type": "string"}},
            },
            "required": ["texte_propose", "fondement_propose", "explication", "sessions_citees"],
        },
    },
}


class RedactionInvalide(Exception):
    pass


def roles(chemin=ANNUAIRE):
    with open(chemin, encoding="utf-8") as f:
        a = json.load(f)
    return sorted({r["role"] for i in a.get("identites", []) for r in i.get("roles", [])})


def _sans_coffre(o):
    o = {k: v for k, v in o.items() if k != "valeurs_coffre"}
    # Le chemin de l'enregistrement relu nomme un fichier, pas une valeur ;
    # inutile au modèle.
    if "recherches" in o:
        o["recherches"] = [{k: v for k, v in r.items() if k != "enregistrement"} for r in o["recherches"]]
    return o


def vue_modele(constat):
    """Ce que le modèle voit d'un constat. Jamais ``detail`` ni
    ``valeurs_coffre``."""
    return {
        "type": constat["type"],
        "nature_du_risque": NATURES[constat["type"]],
        "action": constat["action"],
        "sessions": constat["sessions"],
        "occurrences": [_sans_coffre(o) for o in constat["occurrences"]],
        "contexte": constat.get("contexte") or {},
    }


def messages(constat, roles_):
    contenu = ("Rôles de validation existants : %s.\n\nCONSTAT :\n%s"
               % (", ".join(roles_), json.dumps(vue_modele(constat), ensure_ascii=False, indent=1, sort_keys=True)))
    return [{"role": "system", "content": SYSTEME}, {"role": "user", "content": contenu}]


def client(mode="rejeu", enregistrements=ENREGISTREMENTS):
    return mistral.Client(mode=mode, enregistrements=enregistrements, etat=ETAT, max_appels=MAX_APPELS,
                          modele=mistral.MODELE)


def _arguments(reponse):
    try:
        tcs = reponse["choices"][0]["message"].get("tool_calls") or []
    except (KeyError, IndexError, TypeError, AttributeError):
        raise RedactionInvalide("réponse du modèle illisible") from None
    for tc in tcs:
        fn = tc.get("function") or {}
        if fn.get("name") != "proposer_directive":
            continue
        args = fn.get("arguments", "{}")
        if isinstance(args, str):
            try:
                args = json.loads(args)
            except ValueError:
                raise RedactionInvalide("arguments de proposer_directive illisibles") from None
        return args
    raise RedactionInvalide("le modèle n'a pas appelé proposer_directive")


def _formes(v):
    """Les écritures d'une valeur du coffre qu'on refuse de retrouver."""
    if isinstance(v, bool):
        return set()
    if isinstance(v, float) and v.is_integer():
        v = int(v)
    if isinstance(v, int):
        s = str(v)
        groupes = "{:,}".format(v)
        return {s, groupes.replace(",", " "), groupes.replace(",", " "), groupes.replace(",", " "),
                groupes.replace(",", ".")}
    return {str(v)} if str(v) else set()


RE_SESSION = re.compile(r"\b\d{8}-\d{6}-[A-Za-z0-9_-]+")


def verifier(args, constat):
    """Contrôles du code ; rend la rédaction retenue."""
    out = {}
    for k in ("texte_propose", "fondement_propose", "explication"):
        v = args.get(k)
        if not isinstance(v, str) or not v.strip():
            raise RedactionInvalide("%s vide ou absent" % k)
        out[k] = v.strip()
    sessions = set(constat["sessions"])
    citees = set(args.get("sessions_citees") or []) | set(RE_SESSION.findall(out["explication"]))
    inventees = sorted(citees - sessions)
    if inventees:
        raise RedactionInvalide("session(s) absente(s) du constat : " + ", ".join(inventees))
    if RE_SESSION.search(out["texte_propose"]):
        raise RedactionInvalide("le texte proposé cite une session")
    for o in constat["occurrences"]:
        for v in (o.get("valeurs_coffre") or {}).values():
            for forme in _formes(v):
                for k in ("texte_propose", "fondement_propose", "explication"):
                    if len(forme) >= 4 and forme in out[k]:
                        raise RedactionInvalide("%s recopie une valeur du coffre" % k)
    if A_VALIDER not in out["fondement_propose"]:
        out["fondement_propose"] += " — " + A_VALIDER
    manquantes = [s for s in constat["sessions"] if s not in out["explication"]]
    if manquantes:
        out["explication"] += "\nSessions concernées : " + ", ".join(constat["sessions"]) + "."
    return out


def rediger(constat, client_, roles_=None):
    roles_ = roles_ if roles_ is not None else roles()
    reponse = client_.completer(messages(constat, roles_), [OUTIL], choix_outil="any")
    return verifier(_arguments(reponse), constat)
