import copy
import unittest

from compilateur import directive_go

from .aides import D001
from compilateur.compilateur import construire_jeu


class TestExigerBrouillon(unittest.TestCase):
    def setUp(self):
        self.jeu, _ = construire_jeu(dict(D001), "texte", "credit-pme", "juriste@cabinet.fr",
                                     "2026-10-04T00:00:00Z", "D-001")

    def test_brouillon_accepte(self):
        ok, erreurs, empreinte = directive_go.verifier(self.jeu)
        self.assertTrue(ok, erreurs)
        self.assertTrue(empreinte.startswith("sha256:"))

    def test_hors_brouillon_refuse_par_le_compilateur(self):
        for statut in ("active", "observation", "retiree"):
            jeu = copy.deepcopy(self.jeu)
            jeu["directives"][0]["statut"] = statut
            ok, erreurs, _ = directive_go.verifier(jeu)
            self.assertFalse(ok, statut)
            self.assertEqual(erreurs, ['directive: directive D-001: statut "%s", brouillon exigé' % statut])
            ok, _, _ = directive_go.verifier(jeu, exiger_brouillon=False)
            self.assertTrue(ok, "sans l'option, le chargeur seul ne contrôle pas le statut")


if __name__ == "__main__":
    unittest.main()
