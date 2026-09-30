"""Aides communes aux tests : client Mistral scripté, catalogues."""

import json
import os

from compilateur import catalogue

ICI = os.path.dirname(os.path.abspath(__file__))
EXEMPLES = os.path.join(os.path.dirname(ICI), "compilateur", "exemples")

# Catalogue de démonstration, généré depuis le registre de démo
# (go run ./cmd/registre-demo -catalogue ...).
CATALOGUE_DEMO = os.path.join(EXEMPLES, "catalogue-demo.json")

# Catalogue écrit à la main avant le registre de démo, gelé : les réponses
# Mistral enregistrées des scénarios du compilateur (test_scenarios) ont été
# obtenues avec lui, et l'empreinte de chaque requête dépend de son contenu.
# Le modifier invaliderait ces enregistrements.
CATALOGUE_ENREGISTRE = os.path.join(ICI, "donnees", "catalogue-enregistrements.json")


def catalogue_exemple():
    return catalogue.Catalogue.lire(CATALOGUE_ENREGISTRE)


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


D001 = {
    "verrou": "provenance_session",
    "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"},
    "effet": "attendre_validation",
    "fondement": {"texte": "RGPD", "article": "22"},
    "revision_avant": "2026-12-31",
    "provenance": {"serveurs": ["taux-officiel"]},
}
