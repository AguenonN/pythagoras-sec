"""Ligne de commande du compilateur (un tour, sans dialogue), sur un client
scripté : codes de sortie, sortie JSON, jeu écrit seulement s'il est
accepté."""

import io
import json
import os
import tempfile
import unittest
from unittest import mock

from compilateur import __main__ as cli
from compilateur import compilateur_v2 as c2

from .aides import CATALOGUE_DEMO, ClientScripte, appel, reponse
from .test_compilateur_v2 import CREDIT, d_seuil, jeu

SI = {"question": "q", "reponse": "refuser", "champ": "si_indetermine", "options": ["refuser", "ignorer"],
      "origine": "compilateur"}


class TestCLI(unittest.TestCase):
    def lancer(self, client, *args, reponses=None):
        sortie = io.StringIO()
        with tempfile.TemporaryDirectory() as tmp:
            extra = []
            if reponses is not None:
                chemin = os.path.join(tmp, "reponses.json")
                with open(chemin, "w", encoding="utf-8") as f:
                    json.dump(reponses, f)
                extra = ["--reponses", chemin]
            sortie_jeu = os.path.join(tmp, "jeu.json")
            with mock.patch("compilateur.__main__.mistral.Client", return_value=client):
                rc = cli.principal(["--texte", CREDIT, "--jeu", "credit-pme", "--catalogue", CATALOGUE_DEMO,
                                    "--auteur", "juriste@cabinet.fr", "--date-effet", "2026-10-04",
                                    "--sortie", sortie_jeu] + extra + list(args), sortie=sortie)
            ecrit = os.path.exists(sortie_jeu)
        return rc, sortie.getvalue(), ecrit

    def test_questions_code_3(self):
        rc, out, ecrit = self.lancer(ClientScripte(jeu(d_seuil())))
        self.assertEqual((rc, ecrit), (3, False))
        self.assertIn("Questions pour le juriste", out)

    def test_jeu_valide(self):
        rc, out, ecrit = self.lancer(ClientScripte(jeu(d_seuil())), "--json", reponses=[SI])
        r = json.loads(out)
        self.assertEqual((rc, r["etat"], ecrit), (0, c2.JEU, True))
        self.assertEqual(r["jeu"]["directives"][0]["statut"], "brouillon")

    def test_jeu_refuse_jamais_ecrit(self):
        mauvais = d_seuil(400000)
        rc, out, ecrit = self.lancer(ClientScripte(jeu(mauvais), jeu(mauvais)), "--json", reponses=[SI])
        r = json.loads(out)
        self.assertEqual((rc, r["etat"], ecrit), (1, c2.REFUSE, False))
        self.assertIsNone(r["jeu"])
        self.assertTrue(r["erreurs"])

    def test_non_observable_3(self):
        no = reponse(appel("signaler_non_observable", {"extrait": "x", "raison": "Intention.", "question": "Critère ?"}))
        rc, out, _ = self.lancer(ClientScripte(no))
        self.assertEqual(rc, 3)
        self.assertIn("Critère non observable", out)

    def test_v1_retire(self):
        with self.assertRaises(SystemExit):
            self.lancer(ClientScripte(), "--id-directive", "D-001")


if __name__ == "__main__":
    unittest.main()
