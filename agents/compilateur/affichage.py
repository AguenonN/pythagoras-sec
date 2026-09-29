"""Affichage en texte : le texte du juriste et la relecture, côte à côte."""

import textwrap

LARGEUR = 58


def _colonne(texte, largeur):
    lignes = []
    for paragraphe in texte.split("\n"):
        if not paragraphe.strip():
            lignes.append("")
            continue
        lignes += textwrap.wrap(paragraphe, largeur, break_long_words=True, break_on_hyphens=False)
    return lignes


def cote_a_cote(gauche, droite, titre_gauche="Texte du juriste",
                titre_droite="Relecture : ce qui sera appliqué", largeur=LARGEUR):
    g = [titre_gauche, "-" * min(len(titre_gauche), largeur), ""] + _colonne(gauche, largeur)
    d = [titre_droite, "-" * min(len(titre_droite), largeur), ""] + _colonne(droite, largeur)
    n = max(len(g), len(d))
    g += [""] * (n - len(g))
    d += [""] * (n - len(d))
    return "\n".join((a.ljust(largeur) + " │ " + b).rstrip() for a, b in zip(g, d)) + "\n"


def texte_et_reponses(texte, reponses):
    out = texte
    for r in reponses:
        out += "\n\nQ. " + r["question"] + "\nR. " + r["reponse"]
    return out


def questions(qs):
    out = []
    for n, q in enumerate(qs, 1):
        out.append("%d. %s" % (n, q["question"]))
        for o in q.get("options", []):
            out.append("   - " + o)
    return "\n".join(out) + "\n"
