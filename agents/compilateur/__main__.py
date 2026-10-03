"""Ligne de commande du compilateur : un tour de compilation, sans dialogue.

    python -m compilateur --texte "..." --jeu credit-pme \
        --catalogue compilateur/exemples/catalogue-demo.json \
        --auteur juriste@exemple.test --date-effet 2026-10-04 \
        [--vocabulaire V] [--fondement F] [--jeu-existant demo/directives/demo.json] \
        [--reponses reponses.json] [--mode reel] [--modele M] [--sortie jeu.json] [--json]

Le texte est compilé en un jeu de directives (schéma v2) en brouillon. Les
questions posées sont écrites ; on relance avec ``--reponses`` pour y
répondre. Le dialogue complet est dans ``compilateur.demo`` (texte) et
``compilateur.service`` (console).

Codes de sortie : 0 jeu valide, 3 questions à poser (ou refus motivé d'un
critère non observable), 1 erreurs.
"""

import argparse
import json
import os
import sys

from . import affichage, compilateur_v2 as c2, mistral, vocabulaire

VOCABULAIRE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "exemples", "vocabulaire-demo.json")


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
    ap.add_argument("--jeu", required=True, help="identifiant du jeu (ignoré avec --jeu-existant)")
    ap.add_argument("--catalogue", required=True)
    ap.add_argument("--vocabulaire", default=VOCABULAIRE, help="étiquettes, rôles et groupes")
    ap.add_argument("--auteur", required=True)
    ap.add_argument("--date-effet", required=True, type=date_effet)
    ap.add_argument("--fondement", help="fondement saisi à part du texte")
    ap.add_argument("--prefixe-id", default="D")
    ap.add_argument("--jeu-existant", help="jeu auquel ajouter : le résultat est sa version n+1")
    ap.add_argument("--reponses", help="JSON : [{\"question\": ..., \"reponse\": ...}] ; les réponses aux "
                    "questions du compilateur portent aussi \"champ\", \"options\" et \"origine\": \"compilateur\"")
    ap.add_argument("--mode", choices=["rejeu", "reel"], default="rejeu")
    ap.add_argument("--modele", choices=list(mistral.MODELES), default=mistral.MODELE_COMPILATEUR)
    ap.add_argument("--sortie", help="écrit le jeu en brouillon dans ce fichier")
    ap.add_argument("--json", action="store_true", help="sortie JSON complète")
    a = ap.parse_args(argv)

    texte = a.texte
    if a.texte_fichier:
        with open(a.texte_fichier, encoding="utf-8") as f:
            texte = f.read().strip()
    reponses, existant = [], None
    if a.reponses:
        with open(a.reponses, encoding="utf-8") as f:
            reponses = json.load(f)
    if a.jeu_existant:
        with open(a.jeu_existant, encoding="utf-8") as f:
            existant = json.load(f)

    r = c2.compiler_jeu(texte, a.jeu, vocabulaire.Vocabulaire.lire(a.catalogue, a.vocabulaire),
                        mistral.Client(mode=a.mode, modele=a.modele), a.auteur, a.date_effet,
                        reponses=reponses, prefixe_id=a.prefixe_id, existant=existant, fondement=a.fondement)

    if r["etat"] == c2.JEU and a.sortie:
        ecrire_sortie(a.sortie, r)

    if a.json:
        json.dump(r, sortie, ensure_ascii=False, indent=2)
        sortie.write("\n")
    elif r["etat"] in (c2.QUESTIONS, c2.NON_OBSERVABLE):
        for n in r["non_observables"]:
            sortie.write("Critère non observable : « %s » — %s\n" % (n["extrait"], n["raison"]))
        sortie.write("Questions pour le juriste :\n" + affichage.questions(r["questions"]))
    elif r["etat"] == c2.JEU:
        sortie.write(affichage.cote_a_cote(affichage.texte_et_reponses(texte, reponses), r["relecture"]))
    else:
        sortie.write("Jeu refusé par le vérificateur :\n")
        for e in r["erreurs"]:
            sortie.write("- " + e + "\n")
    return {c2.JEU: 0, c2.QUESTIONS: 3, c2.NON_OBSERVABLE: 3}.get(r["etat"], 1)


if __name__ == "__main__":
    sys.exit(principal())
