"""Démonstration en mode texte : texte du juriste → questions → réponses
saisies → relecture côte à côte → fichier de jeu en brouillon.

    cd agents
    .venv/bin/python -m compilateur.demo --jeu credit-pme \\
        --catalogue compilateur/exemples/catalogue-credit-pme.json \\
        --auteur juriste@cabinet.fr --date-effet 2026-10-04 --sortie brouillon.json

``derouler`` est ce que l'interface juriste appellera : elle ne dépend que
de deux fonctions, lire et écrire.
"""

import argparse
import sys

from . import affichage, catalogue as cat, compilateur, mistral
from .__main__ import date_effet as lire_date, ecrire_sortie

TOURS_MAX = 3  # au-delà, les questions ne convergent pas : on rend la main


def _admise(q, rep):
    """Une question du code est fermée : seule une de ses options est admise."""
    if q.get("origine") != compilateur.ORIGINE_CODE:
        return True
    return compilateur.reponse_indetermine([dict(q, reponse=rep)]) is not None


def derouler(lire, ecrire, texte, jeu_id, catalogue, client, auteur, date_effet):
    """Rend le résultat final du compilateur (dict), après au plus TOURS_MAX
    séries de questions."""
    reponses = []
    for _ in range(TOURS_MAX + 1):
        r = compilateur.compiler(texte, jeu_id, catalogue, client, auteur=auteur,
                                 date_effet=date_effet, reponses=reponses)
        if r["etat"] != compilateur.QUESTIONS:
            break
        ecrire("\nLe compilateur a besoin de précisions :\n")
        for n, q in enumerate(r["questions"], 1):
            ecrire("\n%d. %s\n" % (n, q["question"]))
            for o in q["options"]:
                ecrire("   - %s\n" % o)
            rep = ""
            while not rep.strip() or not _admise(q, rep):
                rep = lire("   Votre réponse : ")
            entree = {"question": q["question"], "reponse": rep.strip()}
            if q.get("origine"):
                entree.update(champ=q["champ"], options=q["options"], origine=q["origine"])
            reponses.append(entree)
    else:
        ecrire("\nLes questions ne convergent pas après %d tours ; reformulez la directive.\n" % TOURS_MAX)
        return r

    if r["etat"] == compilateur.DIRECTIVE:
        ecrire("\n" + affichage.cote_a_cote(affichage.texte_et_reponses(texte, reponses), r["relecture"]))
    elif r["etat"] == compilateur.QUESTIONS:
        pass
    else:
        ecrire("\nDirective refusée par le vérificateur :\n")
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
    a = ap.parse_args(argv)

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

    r = derouler(lire, ecrire, texte, a.jeu, cat.Catalogue.lire(a.catalogue),
                 mistral.Client(mode=a.mode), a.auteur, date)
    if r["etat"] != compilateur.DIRECTIVE:
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
