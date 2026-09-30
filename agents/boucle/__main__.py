"""Ligne de commande de la boucle.

    python -m boucle --sous-agent credit --demande "..." -- python3 serveur.py
    python -m boucle ... --reprendre suspendu.json --jeton JETON -- python3 serveur.py

Codes de sortie : 0 terminé, 2 bloqué, 3 en attente, 1 erreur.
"""

import argparse
import json
import os
import sys

from compilateur import mistral

from . import boucle, mcp

ICI = os.path.dirname(os.path.abspath(__file__))
ENREGISTREMENTS = os.path.join(ICI, "enregistrements")
ETAT = os.path.join(os.path.dirname(ICI), ".etat", "boucle")
# Budget d'appels réels de la boucle, compteur dans .etat/boucle : celui du
# brief d'intégration (nuit/brief_integration.md), étapes 4, 5 et 7.
MAX_APPELS = 25


def client_mistral(mode, enregistrements=ENREGISTREMENTS):
    return mistral.Client(mode=mode, enregistrements=enregistrements, etat=ETAT, max_appels=MAX_APPELS)


def principal(argv=None, sortie=sys.stdout):
    ap = argparse.ArgumentParser(prog="boucle", description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--sous-agent", required=True)
    ap.add_argument("--demande")
    ap.add_argument("--reprendre", help="fichier d'une boucle suspendue")
    ap.add_argument("--jeton", help="jeton de validation pour l'appel en attente")
    ap.add_argument("--suspendre-dans", help="où écrire l'état si la boucle se suspend")
    ap.add_argument("--mode", choices=["rejeu", "reel"], default="rejeu")
    ap.add_argument("--enregistrements", default=ENREGISTREMENTS,
                    help="réponses Mistral enregistrées (cache) ; défaut : celles que rejouent les tests")
    ap.add_argument("serveur", nargs="+", help="commande du serveur MCP stdio")
    a = ap.parse_args(argv)
    if bool(a.demande) == bool(a.reprendre):
        ap.error("--demande ou --reprendre, l'un des deux")
    if a.reprendre and not a.jeton:
        ap.error("--reprendre exige --jeton")

    with mcp.ClientMCP(a.serveur) as serveur:
        b = boucle.Boucle(serveur, client_mistral(a.mode, a.enregistrements), a.sous_agent)
        for s in b.signalements:
            sortie.write("schéma : %s\n" % s)
        for e in b.exclus:
            sortie.write("outil exclu : %s\n" % e)
        if a.reprendre:
            with open(a.reprendre, encoding="utf-8") as f:
                r = b.reprendre(json.load(f), a.jeton)
        else:
            r = b.executer(a.demande)
    sortie.write(r["reponse"] + "\n")
    if r["etat"] == boucle.EN_ATTENTE and a.suspendre_dans:
        with open(a.suspendre_dans, "w", encoding="utf-8") as f:
            json.dump(r, f, ensure_ascii=False, indent=1)
    return {boucle.TERMINE: 0, boucle.BLOQUE: 2, boucle.EN_ATTENTE: 3}.get(r["etat"], 1)


if __name__ == "__main__":
    sys.exit(principal())
