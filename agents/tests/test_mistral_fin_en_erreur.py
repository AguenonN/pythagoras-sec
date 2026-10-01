"""Une génération interrompue côté Mistral (finish_reason: error) n'est pas
une réponse : elle n'est ni rendue ni enregistrée, et compte comme une
erreur transitoire (nouvelle tentative)."""

import os
import tempfile
import unittest
from unittest import mock

from compilateur import mistral

ERREUR = {"choices": [{"finish_reason": "error", "message": {"content": "", "tool_calls": None}}]}
BONNE = {"choices": [{"finish_reason": "tool_calls", "message": {"content": "", "tool_calls": []}}]}


class TestFinEnErreur(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        env = mock.patch.dict(os.environ, {"MISTRAL_API_KEY": "cle-de-test"})
        env.start()
        self.addCleanup(env.stop)

    def client(self, transport):
        return mistral.Client(mode="reel", enregistrements=os.path.join(self.tmp.name, "enr"),
                              etat=os.path.join(self.tmp.name, "etat"), max_appels=10,
                              transport=transport, dormir=lambda s: None)

    def test_nouvelle_tentative_puis_enregistrement_de_la_bonne(self):
        reponses = [ERREUR, BONNE]
        c = self.client(lambda corps, cle: reponses.pop(0))
        self.assertEqual(c.completer([{"role": "user", "content": "x"}], []), BONNE)
        self.assertEqual(len(os.listdir(os.path.join(self.tmp.name, "enr"))), 1)

    def test_jamais_enregistree(self):
        c = self.client(lambda corps, cle: ERREUR)
        with self.assertRaises(mistral.ErreurMistral):
            c.completer([{"role": "user", "content": "x"}], [])
        self.assertFalse(os.path.exists(os.path.join(self.tmp.name, "enr")))
