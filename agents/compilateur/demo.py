"""Démonstration en mode texte : texte du juriste → questions → réponses
saisies → relecture côte à côte → fichier de jeu en brouillon.

    cd agents
    .venv/bin/python -m compilateur.demo --jeu credit-pme \\
        --catalogue compilateur/exemples/catalogue-demo.json \\
        --auteur juriste@exemple.test --date-effet 2026-10-04 --sortie brouillon.json

Le texte est compilé en un jeu de directives (schéma v2 : plusieurs
directives possibles, arbre de condition, liste d'effets) : refus motivés
et questions de Mistral, questions fermées du code, relecture, jeu en
brouillon. Le vocabulaire (étiquettes, rôles, groupes) est lu dans
``--vocabulaire`` ; avec ``--jeu-existant``, le jeu écrit est la version n+1
de ce jeu. Le format v1 est retiré.

``derouler_v2`` ne dépend que de deux fonctions, lire et écrire ; la
console passe par ``compilateur.service``.
"""

import argparse
import json
import os
import sys

from . import affichage, compilateur_v2, mistral, vocabulaire
from .__main__ import date_effet as lire_date, ecrire_sortie

TOURS_MAX = 3  # au-delà, les questions ne convergent pas : on rend la main
VOCABULAIRE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "exemples", "vocabulaire-demo.json")


def _poser(lire, ecrire, q):
    """Pose une question et rend la réponse saisie, non vide ; une question
    du code est fermée : seule une de ses options est admise."""
    while True:
        rep = lire("   Votre réponse : ").strip()
        if not rep:
            continue
        if q.get("origine") != compilateur_v2.ORIGINE_CODE:
            return rep
        if compilateur_v2.reponse_du_code([dict(q, reponse=rep)], q["champ"], q["options"]) is not None:
            return rep
        ecrire("   Réponse attendue : %s.\n" % " ou ".join(q["options"]))


def derouler_v2(lire, ecrire, texte, jeu_id, vocab, client, auteur, date_effet, prefixe_id="D", existant=None):
    """Comme ``derouler``, pour un jeu v2 : rend le résultat final de
    compilateur_v2.compiler_jeu. ``existant`` : le jeu auquel ajouter."""
    reponses = []
    for _ in range(TOURS_MAX + 1):
        r = compilateur_v2.compiler_jeu(texte, jeu_id, vocab, client, auteur=auteur,
                                        date_effet=date_effet, reponses=reponses, prefixe_id=prefixe_id,
                                        existant=existant)
        if r["etat"] not in (compilateur_v2.QUESTIONS, compilateur_v2.NON_OBSERVABLE):
            break
        if r["etat"] == compilateur_v2.NON_OBSERVABLE:
            ecrire("\nLe compilateur refuse de traduire un critère non observable :\n")
            for n in r["non_observables"]:
                ecrire("\n  « %s »\n  %s\n" % (n["extrait"], n["raison"]))
            ecrire("\nDonnez un critère observable, ou reformulez la règle.\n")
        else:
            ecrire("\nLe compilateur a besoin de précisions :\n")
        for n, q in enumerate(r["questions"], 1):
            ecrire("\n%d. %s\n" % (n, q["question"]))
            for o in q.get("options") or []:
                ecrire("   - %s\n" % o)
            entree = {"question": q["question"], "reponse": _poser(lire, ecrire, q)}
            if q.get("origine"):
                entree.update(champ=q["champ"], options=q["options"], origine=q["origine"])
            reponses.append(entree)
    else:
        ecrire("\nLes questions ne convergent pas après %d tours ; reformulez la règle.\n" % TOURS_MAX)
        return r

    if r["etat"] == compilateur_v2.JEU:
        ecrire("\n" + affichage.cote_a_cote(affichage.texte_et_reponses(texte, reponses), r["relecture"]))
    elif r["etat"] in (compilateur_v2.REFUSE, compilateur_v2.ERREURS):
        ecrire("\nJeu refusé par le vérificateur :\n")
        for e in r["erreurs"]:
            ecrire("- %s\n" % e)
    return r


def principal(argv=None, lire=input, ecrire=sys.stdout.write):
    ap = argparse.ArgumentParser(prog="compilateur.demo", description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--jeu", required=True)
    ap.add_argument("--catalogue", required=True)
    ap.add_argument("--auteur", required=True)
    ap.add_argument("--date-effet", type=lire_date, help="AAAA-MM-JJ ; demandée si absente")
    ap.add_argument("--texte", help="texte de la directive ; demandé si absent")
    ap.add_argument("--sortie", required=True, help="fichier du jeu en brouillon")
    ap.add_argument("--mode", choices=["rejeu", "reel"], default="rejeu")
    ap.add_argument("--vocabulaire", default=VOCABULAIRE, help="étiquettes, rôles et groupes")
    ap.add_argument("--prefixe-id", default="D", help="préfixe des identifiants de directive : D-001, D-002…")
    ap.add_argument("--modele", choices=list(mistral.MODELES), default=mistral.MODELE_COMPILATEUR)
    ap.add_argument("--jeu-existant", help="jeu auquel ajouter : la sortie est sa version n+1, les directives "
                                           "existantes inchangées, les nouvelles en brouillon, numérotées à la suite")
    a = ap.parse_args(argv)
    existant = None
    if a.jeu_existant:
        with open(a.jeu_existant, encoding="utf-8") as f:
            existant = json.load(f)

    texte = a.texte
    while not texte or not texte.strip():
        texte = lire("Texte de la directive, avec son fondement :\n> ")
    texte = texte.strip()
    date = a.date_effet
    while not date:
        try:
            date = lire_date(lire("Date d'effet (AAAA-MM-JJ) : ").strip())
        except ValueError:
            ecrire("Date illisible.\n")

    r = derouler_v2(lire, ecrire, texte, a.jeu, vocabulaire.Vocabulaire.lire(a.catalogue, a.vocabulaire),
                    mistral.Client(mode=a.mode, modele=a.modele), a.auteur, date, prefixe_id=a.prefixe_id,
                    existant=existant)
    if r["etat"] != compilateur_v2.JEU:
        return 1
    ecrire_sortie(a.sortie, r)
    ecrire("\nJeu écrit en brouillon dans %s. Il ne sera appliqué qu'une fois passé en "
           "observation ou active par le juriste.\n" % a.sortie)
    return 0


if __name__ == "__main__":
    try:
        sys.exit(principal())
    except (EOFError, KeyboardInterrupt):
        sys.stdout.write("\nInterrompu, rien n'a été écrit.\n")
        sys.exit(130)
