import json
import os
import tempfile
import unittest
from unittest import mock

from compilateur import mistral


class TestClient(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.enr = os.path.join(self.tmp.name, "enr")
        self.etat = os.path.join(self.tmp.name, "etat")
        self.attentes = []
        self.env = mock.patch.dict(os.environ, {"MISTRAL_API_KEY": "cle-de-test-xyz"})
        self.env.start()

    def tearDown(self):
        self.env.stop()
        self.tmp.cleanup()

    def client(self, mode="reel", transport=None, max_appels=40):
        return mistral.Client(mode=mode, enregistrements=self.enr, etat=self.etat, max_appels=max_appels,
                              transport=transport, dormir=self.attentes.append)

    def test_rejeu_sans_enregistrement(self):
        with self.assertRaises(mistral.EnregistrementAbsent):
            self.client(mode="rejeu").completer([{"role": "user", "content": "x"}], [])

    def test_reel_enregistre_puis_rejoue(self):
        vus = []

        def transport(corps, cle):
            vus.append(corps)
            return {"ok": len(vus)}

        c = self.client(transport=transport)
        self.assertEqual(c.completer([{"role": "user", "content": "x"}], []), {"ok": 1})
        self.assertEqual(c.completer([{"role": "user", "content": "x"}], []), {"ok": 1})
        self.assertEqual(len(vus), 1, "une réponse obtenue n'est jamais redemandée")
        self.assertEqual(vus[0]["model"], "mistral-small-latest")
        self.assertEqual(vus[0]["temperature"], 0)
        rejeu = self.client(mode="rejeu")
        self.assertEqual(rejeu.completer([{"role": "user", "content": "x"}], []), {"ok": 1})
        for nom in os.listdir(self.enr):
            with open(os.path.join(self.enr, nom), encoding="utf-8") as f:
                self.assertNotIn("cle-de-test-xyz", f.read(), "la clé ne doit jamais être écrite")

    def test_budget(self):
        c = self.client(transport=lambda corps, cle: {}, max_appels=2)
        c.completer([{"role": "user", "content": "1"}], [])
        c.completer([{"role": "user", "content": "2"}], [])
        with self.assertRaises(mistral.BudgetEpuise):
            c.completer([{"role": "user", "content": "3"}], [])
        with open(os.path.join(self.etat, "budget.json")) as f:
            self.assertEqual(json.load(f)["appels"], 2)

    def test_trois_tentatives_au_plus(self):
        n = []

        def transport(corps, cle):
            n.append(1)
            raise mistral.ErreurTransitoire("HTTP 503")

        with self.assertRaises(mistral.ErreurMistral) as ctx:
            self.client(transport=transport).completer([{"role": "user", "content": "x"}], [])
        self.assertEqual(len(n), 3)
        self.assertNotIn("cle-de-test-xyz", str(ctx.exception))
        self.assertIn(2, self.attentes)
        self.assertIn(4, self.attentes)

    def test_une_requete_par_seconde(self):
        c = self.client(transport=lambda corps, cle: {})
        c.completer([{"role": "user", "content": "1"}], [])
        c.completer([{"role": "user", "content": "2"}], [])
        self.assertTrue(any(a > 0.5 for a in self.attentes), "le second appel doit attendre")

    def test_sans_cle(self):
        with mock.patch.dict(os.environ, {"MISTRAL_API_KEY": ""}):
            with self.assertRaises(mistral.ErreurMistral):
                self.client(transport=lambda corps, cle: {}).completer([{"role": "user", "content": "x"}], [])


if __name__ == "__main__":
    unittest.main()
