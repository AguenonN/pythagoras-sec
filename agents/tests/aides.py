"""Aides communes aux tests : client Mistral scripté, catalogue d'exemple."""

import json
import os

from compilateur import catalogue

ICI = os.path.dirname(os.path.abspath(__file__))
EXEMPLES = os.path.join(os.path.dirname(ICI), "compilateur", "exemples")


def catalogue_exemple():
    return catalogue.Catalogue.lire(os.path.join(EXEMPLES, "catalogue-credit-pme.json"))


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
