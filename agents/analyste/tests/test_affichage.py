"""Ce que la console affiche d'une proposition (type et détail du constat,
texte proposé, explication) est en français, dans le vocabulaire de la
console : « retiré », jamais « caviardage » ; trimestres « T3 2026 »."""

import glob
import json
import os
import re
import unittest

from analyste import propositions, redaction

ICI = os.path.dirname(os.path.abspath(__file__))
EXEMPLES = sorted(glob.glob(os.path.join(os.path.dirname(ICI), "exemples", "propositions", "*.json")))
# Mots anglais courants qu'aucun texte affiché ne devrait porter.
ANGLAIS = re.compile(r"\b(the|and|of|with|without|this|that|should|must|when|internal|rate|loan|"
                     r"approval|approve|refuse[ds]?|session[s]? (?:was|were)|found|not)\b", re.I)


def affiches(p):
    return {"type": p["constat"]["type"], "detail": p["constat"]["detail"], "texte_propose": p["texte_propose"],
            "explication": p["explication"], "fondement_propose": p["fondement_propose"]}


class TestAffichage(unittest.TestCase):
    def test_quatre_exemples(self):
        self.assertEqual(len(EXEMPLES), 4)

    def test_en_francais(self):
        for chemin in EXEMPLES:
            with open(chemin, encoding="utf-8") as f:
                p = json.load(f)
            for champ, texte in affiches(p).items():
                with self.subTest(proposition=p["id"], champ=champ):
                    self.assertNotIn("caviard", texte.lower())
                    self.assertIsNone(re.search(r"\b\d{4}-Q[1-4]\b", texte))
                    self.assertIsNone(re.search(r"\b[Ll]e gate\b", texte))
                    self.assertIsNone(ANGLAIS.search(texte), texte)
            self.assertNotIn("_", p["constat"]["type"])  # un libellé, pas un code
            self.assertIn(p["constat"]["code"], propositions.LIBELLES)

    def test_contournement_dit_retire(self):
        (chemin,) = [c for c in EXEMPLES if "contournement-retrait" in c]
        with open(chemin, encoding="utf-8") as f:
            p = json.load(f)
        self.assertIn("retiré", p["texte_propose"])
        self.assertIn("retiré", p["explication"])

    def test_normaliser(self):
        self.assertEqual(redaction.normaliser("le trimestre 2026-Q3 ; le gate ne voit pas"),
                         "le trimestre T3 2026 ; l'engine ne voit pas")

    def test_mot_interdit_refuse(self):
        c = {"sessions": ["20261002-000000-x"], "occurrences": []}
        with self.assertRaises(redaction.RedactionInvalide):
            redaction.verifier({"texte_propose": "Le caviardage est contourné.", "fondement_propose": "F",
                                "explication": "E 20261002-000000-x"}, c)
