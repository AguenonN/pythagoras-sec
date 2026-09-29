import json
import os
import tempfile
import unittest

from compilateur import demo

from unittest import mock

from .aides import D001 as D001_PROPOSITION
from .aides import EXEMPLES, ClientScripte, appel, reponse
from .test_scenarios import D001, reponses

ICI = os.path.dirname(os.path.abspath(__file__))


class TestDemo(unittest.TestCase):
    def lancer(self, saisies, *args):
        saisies = list(saisies)
        vues, ecrit = [], []

        def lire(invite):
            vues.append(invite)
            return saisies.pop(0)

        with tempfile.TemporaryDirectory() as tmp:
            sortie = os.path.join(tmp, "jeu.json")
            rc = demo.principal(["--jeu", "credit-pme", "--catalogue", os.path.join(EXEMPLES, "catalogue-credit-pme.json"),
                                 "--auteur", "juriste@cabinet.fr", "--sortie", sortie] + list(args),
                                lire=lire, ecrire=ecrit.append)
            jeu = None
            if os.path.exists(sortie):
                with open(sortie, encoding="utf-8") as f:
                    jeu = json.load(f)
        return rc, "".join(ecrit), jeu, saisies

    def test_parcours_complet(self):
        saisies = [D001, "2026-10-04"] + [r["reponse"] for r in reponses("reponses-d001.json")]
        rc, ecrit, jeu, reste = self.lancer(saisies)
        self.assertEqual(rc, 0, ecrit)
        self.assertEqual(reste, [])
        self.assertIn("besoin de précisions", ecrit)
        self.assertIn("Relecture : ce qui sera appliqué", ecrit)
        d = jeu["directives"][0]
        self.assertEqual(d["statut"], "brouillon")
        self.assertEqual(d["verrou"], "provenance_session")
        with open(os.path.join(ICI, "attendus", "d001.txt"), encoding="utf-8") as f:
            self.assertIn("Verrou provenance_session.", f.read())

    def test_reponse_vide_redemandee(self):
        saisies = ["", "   ", reponses("reponses-d001.json")[0]["reponse"], reponses("reponses-d001.json")[1]["reponse"]]
        rc, _, jeu, _ = self.lancer(saisies, "--texte", D001, "--date-effet", "2026-10-04")
        self.assertEqual(rc, 0)
        self.assertIsNotNone(jeu)

    def test_rien_n_est_ecrit_sans_directive(self):
        mauvais = dict(D001_PROPOSITION, effet="caviarder")
        client = ClientScripte(reponse(appel("proposer_directive", mauvais)), reponse(appel("proposer_directive", mauvais)))
        with mock.patch("compilateur.demo.mistral.Client", return_value=client):
            rc, ecrit, jeu, _ = self.lancer([], "--texte", D001, "--date-effet", "2026-10-04")
        self.assertEqual(rc, 1)
        self.assertIsNone(jeu)
        self.assertIn("refusée par le vérificateur", ecrit)

    def test_questions_sans_fin(self):
        q = reponse(appel("poser_question", {"question": "Encore ?", "champ": "autre"}))
        client = ClientScripte(*[q] * (demo.TOURS_MAX + 1))
        with mock.patch("compilateur.demo.mistral.Client", return_value=client):
            rc, ecrit, jeu, _ = self.lancer(["oui"] * (demo.TOURS_MAX + 1), "--texte", D001, "--date-effet", "2026-10-04")
        self.assertEqual(rc, 1)
        self.assertIsNone(jeu)
        self.assertIn("ne convergent pas", ecrit)

if __name__ == "__main__":
    unittest.main()
