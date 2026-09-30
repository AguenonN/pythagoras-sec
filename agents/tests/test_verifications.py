import unittest

from compilateur import verifications as v

from .aides import D001, catalogue_exemple


class TestExtraction(unittest.TestCase):
    def test_montants(self):
        cas = {
            "au-delà de 500 000 euros": 500000,
            "au-delà de 500\u202f000 €": 500000,
            "plus de 2.000.000 €": 2000000,
            "1,5 million d'euros": 1500000,
            "1,5 millions de euros": 1500000,
            "10k€": 10000,
            "10 k€": 10000,
            "3 milliards d’euros": 3e9,
            "un euro, soit 1 euro": 1,
            "un taux de 5,87 %": 5.87,
            "5%": 5,
        }
        for texte, n in cas.items():
            self.assertEqual(v.montants(texte), {float(n)}, texte)

    def test_sans_unite_rien(self):
        for texte in ["un montant significatif", "au-delà de 500 000", "24 mois", "le 31/12/2026",
                      "1,5 million", "SIREN 123456789"]:
            self.assertEqual(v.montants(texte), set(), texte)

    def test_references_rien(self):
        for texte in ["art. 22 %", "art.22 €", "Art 22 euros", "article 1240 €", "l'article 5 %",
                      "L561-10 €", "L. 511-1 euros", "R123-4 %", "R. 123 €", "art. 22-1 %"]:
            self.assertEqual(v.montants(texte), set(), texte)
        self.assertEqual(v.montants("En application de l'art. 22 RGPD, au-delà de 500 000 euros"), {500000.0})
        self.assertEqual(v.montants("art. L561-10 : au-delà de 10 000 €"), {10000.0})

    def test_durees(self):
        cas = {"au-delà de 24 mois": 24, "pendant 30 jours": 30, "1 jour": 1, "3 ans": 3, "1 an": 1,
               "2 années": 2, "2 annees": 2, "1,5 an": 1.5}
        for texte, n in cas.items():
            self.assertEqual(v.durees(texte), {float(n)}, texte)
        for texte in ["24", "24 moisson", "art. 5 ans", "L. 12 mois", "article 3 jours", "10 semaines"]:
            self.assertEqual(v.durees(texte), set(), texte)

    def test_dates_explicites(self):
        source = "Applicable jusqu'au 31/12/2026, puis à compter du 1er janvier 2027."
        self.assertTrue(v.valeur_ecrite("2026-12-31", source))
        self.assertTrue(v.valeur_ecrite("2027-01-01T00:00:00Z", source))
        self.assertFalse(v.valeur_ecrite("2027-06-30", source), "date non écrite")
        self.assertFalse(v.valeur_ecrite("2026-02-30", "le 30/02/2026"), "date impossible")
        self.assertFalse(v.valeur_ecrite(2026, source), "une année seule n'est pas une quantité")

    def test_valeur_ecrite(self):
        source = "En application de l'art. 22 RGPD, au-delà de 500 000 euros ; 24 mois ; 36 prêts."
        self.assertTrue(v.valeur_ecrite(500000, source))
        self.assertTrue(v.valeur_ecrite(24, source), "une durée avec unité justifie sa valeur")
        self.assertFalse(v.valeur_ecrite(22, source), "un numéro d'article ne justifie rien")
        self.assertFalse(v.valeur_ecrite(36, source), "un nombre sans unité ne justifie rien")
        self.assertFalse(v.valeur_ecrite(500, source))
        self.assertTrue(v.valeur_ecrite([500000], source))
        self.assertFalse(v.valeur_ecrite([500000, 22], source))

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
