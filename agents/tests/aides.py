"""Aides communes aux tests : client Mistral scripté, catalogues."""

import json
import os

from compilateur import catalogue

ICI = os.path.dirname(os.path.abspath(__file__))
EXEMPLES = os.path.join(os.path.dirname(ICI), "compilateur", "exemples")

# Catalogue de démonstration, généré depuis le registre de démo
# (go run ./cmd/registre-demo -catalogue ...).
CATALOGUE_DEMO = os.path.join(EXEMPLES, "catalogue-demo.json")

# Catalogue généré tel qu'il était quand les scénarios v2 du compilateur
# (test_scenarios_v2, parcours week-end de test_demo_v2) ont été enregistrés
# auprès de Mistral, gelé pour la même raison : le catalogue généré a changé
# depuis (accès read, remettre_rapport), et chaque requête enregistrée
# l'embarque. Copie de feat/compilateur-v2:agents/compilateur/exemples/catalogue-demo.json.
CATALOGUE_V2_ENREGISTRE = os.path.join(ICI, "donnees", "catalogue-v2-enregistrements.json")

# Vocabulaire (étiquettes, rôles, groupes) écrit à la main, tel qu'il était
# quand ces mêmes scénarios ont été enregistrés ; gelé pour la même raison :
# exemples/vocabulaire-demo.json est désormais généré depuis le registre et
# l'annuaire (go run ./cmd/registre-demo -vocabulaire ...), et chaque requête
# enregistrée embarque le vocabulaire.
VOCABULAIRE_V2_ENREGISTRE = os.path.join(ICI, "donnees", "vocabulaire-v2-enregistrements.json")

# Catalogue et vocabulaire générés tels qu'ils étaient quand les textes de la
# scène (week-end, base interne : test_service_scene) et les scénarios
# « medium » de l'étape 7 (test_scenarios_v2) ont été enregistrés auprès de
# Mistral ; gelés pour la même raison quand la session « secteurs » a ajouté
# les outils banque et RH au catalogue (commit précédant ces copies :
# 5b83cd5). Un passage réel avec le catalogue courant fait de nouveaux
# enregistrements.
CATALOGUE_SCENE = os.path.join(ICI, "donnees", "catalogue-scene-enregistrements.json")
VOCABULAIRE_SCENE = os.path.join(ICI, "donnees", "vocabulaire-scene-enregistrements.json")


def catalogue_demo():
    return catalogue.Catalogue.lire(CATALOGUE_DEMO)


def appel(nom, args, id_="c1"):
    return {"id": id_, "type": "function", "function": {"name": nom, "arguments": json.dumps(args, ensure_ascii=False)}}


def reponse(*appels):
    return {"choices": [{"message": {"role": "assistant", "content": "", "tool_calls": list(appels)}}]}


def texte_final(contenu):
    return {"choices": [{"message": {"role": "assistant", "content": contenu, "tool_calls": None}}]}


FAUX_SERVEUR = os.path.join(ICI, "faux_serveur_mcp.py")


class ClientScripte:
    """Rend les réponses dans l'ordre et garde les messages reçus."""

    def __init__(self, *reponses):
        self.reponses = list(reponses)
        self.recus = []
        self.options = []

    def completer(self, messages, outils, **options):
        self.recus.append((json.loads(json.dumps(messages)), outils))
        self.options.append(options)
        return self.reponses.pop(0)
