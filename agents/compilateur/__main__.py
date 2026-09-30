"""Ligne de commande du compilateur.

    python -m compilateur --texte "..." --jeu credit-pme \\
        --catalogue compilateur/exemples/catalogue-credit-pme.json \\
        --auteur juriste@cabinet.fr --date-effet 2026-10-04 \\
        [--reponses reponses.json] [--mode reel] [--sortie jeu.json] [--json]

Codes de sortie : 0 directive valide, 3 questions à poser, 1 erreurs.
"""

import argparse
import json
import sys

from . import affichage, catalogue as cat, compilateur, mistral


def date_effet(s):
    """AAAA-MM-JJ (minuit UTC) ou horodatage RFC 3339 complet."""
    if "T" in s:
        return s
    import datetime
    datetime.date.fromisoformat(s)
    return s + "T00:00:00Z"


def ecrire_sortie(chemin, r):
    """Écrit le jeu en brouillon et, à côté, la consignation des questions
    et réponses qui l'ont produit (``<chemin>.reponses.json``)."""
    with open(chemin, "w", encoding="utf-8") as f:
        json.dump(r["jeu"], f, ensure_ascii=False, indent=2)
        f.write("\n")
    with open(chemin + ".reponses.json", "w", encoding="utf-8") as f:
        json.dump({"texte": r["texte"], "reponses": r["reponses"], "consignation": r["consignation"]},
                  f, ensure_ascii=False, indent=2)
        f.write("\n")


def principal(argv=None, sortie=sys.stdout):
    ap = argparse.ArgumentParser(prog="compilateur", description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    source = ap.add_mutually_exclusive_group(required=True)
    source.add_argument("--texte")
    source.add_argument("--texte-fichier")
    ap.add_argument("--jeu", required=True, help="identifiant du jeu")
    ap.add_argument("--catalogue", required=True)
    ap.add_argument("--auteur", required=True)
    ap.add_argument("--date-effet", required=True, type=date_effet)
    ap.add_argument("--id-directive", default="D-001")
    ap.add_argument("--reponses", help="JSON : [{\"question\": ..., \"reponse\": ...}] ; les réponses aux "
                    "questions du compilateur portent aussi \"champ\" et \"origine\": \"compilateur\"")
    ap.add_argument("--mode", choices=["rejeu", "reel"], default="rejeu")
    ap.add_argument("--sortie", help="écrit le jeu en brouillon dans ce fichier")
    ap.add_argument("--json", action="store_true", help="sortie JSON complète")
    a = ap.parse_args(argv)

    texte = a.texte
    if a.texte_fichier:
        with open(a.texte_fichier, encoding="utf-8") as f:
            texte = f.read().strip()
    reponses = []
    if a.reponses:
        with open(a.reponses, encoding="utf-8") as f:
            reponses = json.load(f)

    r = compilateur.compiler(
        texte, a.jeu, cat.Catalogue.lire(a.catalogue), mistral.Client(mode=a.mode),
        auteur=a.auteur, date_effet=a.date_effet, reponses=reponses,
        id_directive=a.id_directive)

    if r["etat"] == compilateur.DIRECTIVE and a.sortie:
        ecrire_sortie(a.sortie, r)

    if a.json:
        json.dump(r, sortie, ensure_ascii=False, indent=2)
        sortie.write("\n")
    elif r["etat"] == compilateur.QUESTIONS:
        sortie.write("Questions pour le juriste :\n" + affichage.questions(r["questions"]))
    elif r["etat"] == compilateur.DIRECTIVE:
        sortie.write(affichage.cote_a_cote(affichage.texte_et_reponses(texte, reponses), r["relecture"]))
    else:
        sortie.write("Directive refusée par le vérificateur :\n")
        for e in r["erreurs"]:
            sortie.write("- " + e + "\n")
    return {compilateur.DIRECTIVE: 0, compilateur.QUESTIONS: 3}.get(r["etat"], 1)


if __name__ == "__main__":
    sys.exit(principal())
