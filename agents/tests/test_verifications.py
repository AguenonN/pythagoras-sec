import unittest

from compilateur import verifications as v

from .aides import D001, catalogue_exemple


class TestExtraction(unittest.TestCase):
    def test_nombres(self):
        cas = {
            "au-delà de 500 000 euros": 500000,
            "au-delà de 500 000 €": 500000,
            "plus de 2.000.000": 2000000,
            "1,5 million": 1500000,
            "10k€": 10000,
            "3 milliards": 3e9,
            "0,25": 0.25,
        }
        for texte, n in cas.items():
            self.assertIn(float(n), v.nombres(texte), texte)
        self.assertEqual(v.nombres("un montant significatif"), set())

    def test_dates(self):
        self.assertEqual(v.dates("avant le 31/12/2026"), {"2026-12-31"})
        self.assertEqual(v.dates("le 1er janvier 2027"), {"2027-01-01"})
        self.assertEqual(v.dates("le 15 août 2027"), {"2027-08-15"})
        self.assertEqual(v.dates("au 2027-03-05"), {"2027-03-05"})
        self.assertEqual(v.dates("en attendant des solutions concrètes"), set())


class TestProposition(unittest.TestCase):
    def setUp(self):
        self.cat = catalogue_exemple()
        self.source = "En application de l'art. 22 RGPD ... À relire avant le 31/12/2026."

    def erreurs(self, **changements):
        p = dict(D001)
        p.update(changements)
        return v.verifier_proposition(p, self.source, self.cat)

    def test_proposition_justifiee(self):
        self.assertEqual(self.erreurs(), [])

    def test_serveur_invente(self):
        e = self.erreurs(cible={"serveur": "gels-avoirs", "outil": "attribuer_pret"})
        self.assertTrue(any("absente du catalogue" in x for x in e), e)
        e = self.erreurs(provenance={"serveurs": ["gels-avoirs"]})
        self.assertTrue(any("gels-avoirs" in x for x in e), e)

    def test_outil_d_un_autre_serveur(self):
        e = self.erreurs(cible={"serveur": "taux-officiel", "outil": "attribuer_pret"})
        self.assertTrue(e)

    def test_montant_invente(self):
        e = self.erreurs(verrou="seuil_argument", provenance=None,
                         condition=[{"champ": "/montant", "op": ">", "valeur": 100000}])
        self.assertTrue(any("n'est pas écrite par le juriste" in x for x in e), e)

    def test_montant_ecrit(self):
        self.source += " Au-delà de 100 000 euros."
        e = self.erreurs(verrou="seuil_argument", provenance=None,
                         condition=[{"champ": "/montant", "op": ">", "valeur": 100000}])
        self.assertEqual(e, [])

    def test_champ_invente(self):
        e = self.erreurs(verrou="seuil_argument", provenance=None,
                         condition=[{"champ": "/revenu", "op": "present"}])
        self.assertTrue(any("/revenu" in x for x in e), e)

    def test_date_inventee(self):
        e = self.erreurs(revision_avant="2027-06-30")
        self.assertTrue(any("revision_avant" in x for x in e), e)

    def test_fondement_invente(self):
        e = self.erreurs(fondement={"texte": "Code civil", "article": "1240"})
        self.assertEqual(len(e), 2, e)

    def test_ignorer_jamais_propose(self):
        e = self.erreurs(si_indetermine="ignorer")
        self.assertTrue(any("ignorer" in x for x in e), e)

    def test_perimetre_hors_catalogue(self):
        e = self.erreurs(perimetre=["spiffe://lab/inconnu"])
        self.assertTrue(any("appelant" in x for x in e), e)


if __name__ == "__main__":
    unittest.main()
