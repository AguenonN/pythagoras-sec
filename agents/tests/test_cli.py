import io
import json
import os
import tempfile
import unittest
from unittest import mock

from compilateur import __main__ as cli
from compilateur import compilateur as comp

from .aides import D001, EXEMPLES, ClientScripte, appel, reponse

TEXTE = "En application de l'art. 22 RGPD, … À relire avant le 31/12/2026."


class TestJSON(unittest.TestCase):
    def lancer(self, client, *args):
        sortie = io.StringIO()
        with mock.patch("compilateur.__main__.mistral.Client", return_value=client):
            rc = cli.principal(["--texte", TEXTE, "--jeu", "credit-pme", "--catalogue",
                                os.path.join(EXEMPLES, "catalogue-credit-pme.json"), "--auteur", "juriste@cabinet.fr",
                                "--date-effet", "2026-10-04", "--json"] + list(args), sortie=sortie)
        return rc, json.loads(sortie.getvalue())

    def test_jeu_refuse_marque(self):
        mauvais = dict(D001, effet="caviarder")
        with tempfile.TemporaryDirectory() as tmp:
            chemin = os.path.join(tmp, "jeu.json")
            rc, r = self.lancer(ClientScripte(reponse(appel("proposer_directive", mauvais)),
                                              reponse(appel("proposer_directive", mauvais))), "--sortie", chemin)
            self.assertFalse(os.path.exists(chemin), "un jeu refusé n'est jamais écrit")
        self.assertEqual(rc, 1)
        self.assertEqual(r["etat"], comp.REFUSE)
        self.assertIsNone(r["jeu"])
        self.assertEqual(r["jeu_refuse"]["directives"][0]["statut"], "brouillon")
        self.assertTrue(r["erreurs"])

    def test_jeu_valide_non_marque(self):
        rc, r = self.lancer(ClientScripte(reponse(appel("proposer_directive", D001))))
        self.assertEqual((rc, r["etat"]), (0, comp.DIRECTIVE))
        self.assertIsNotNone(r["jeu"])
        self.assertNotIn("jeu_refuse", r)


if __name__ == "__main__":
    unittest.main()
