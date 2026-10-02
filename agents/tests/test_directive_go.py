import copy
import unittest

from compilateur import directive_go

JEU = {"jeu": {"id": "credit-pme", "version": 1, "date_effet": "2026-10-04T00:00:00Z", "auteur": "juriste@cabinet.fr"},
       "directives": [{
           "id": "D-001", "version": 1, "statut": "brouillon", "date_effet": "2026-10-04T00:00:00Z",
           "texte_fr": "Un prêt attribué à partir du taux officiel attend une validation humaine (RGPD, art. 22).",
           "fondements": [{"texte": "RGPD", "article": "22"}],
           "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"}},
           "condition": {"session.a_consulte": {"serveur": "taux-officiel", "critere": "donnees_recues"}},
           "effets": [{"effet": "attendre_validation"}]}]}

V1 = {"id": "D-001", "version": 1, "statut": "brouillon", "date_effet": "2026-10-04T00:00:00Z", "texte_fr": "t",
      "fondement": {"texte": "RGPD"}, "verrou": "interdiction", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}


class TestExigerBrouillon(unittest.TestCase):
    def test_brouillon_accepte(self):
        ok, erreurs, empreinte = directive_go.verifier(JEU)
        self.assertTrue(ok, erreurs)
        self.assertTrue(empreinte.startswith("sha256:"))

    def test_hors_brouillon_refuse_par_le_compilateur(self):
        for statut in ("active", "observation", "retiree"):
            jeu = copy.deepcopy(JEU)
            jeu["directives"][0]["statut"] = statut
            ok, erreurs, _ = directive_go.verifier(jeu)
            self.assertFalse(ok, statut)
            self.assertEqual(erreurs, ['directive: directive D-001: statut "%s", brouillon exigé' % statut])
            ok, _, _ = directive_go.verifier(jeu, exiger_brouillon=False)
            self.assertTrue(ok, "sans l'option, le chargeur seul ne contrôle pas le statut")

    def test_v1_refuse(self):
        jeu = dict(JEU, directives=[V1])
        ok, erreurs, _ = directive_go.verifier(jeu, exiger_brouillon=False)
        self.assertFalse(ok)
        self.assertIn("directive: directive 1: format de directive v1 non pris en charge : réécrire en v2", erreurs)
        with self.assertRaises(directive_go.ErreurOutil):
            directive_go.relire(jeu)


if __name__ == "__main__":
    unittest.main()
