import unittest

from compilateur import affichage, compilateur as comp

from .aides import D001, ClientScripte, appel, catalogue_exemple, reponse

TEXTE = ("En application de l'art. 22 RGPD, en attendant des solutions concrètes, plus aucun workload "
         "ne doit utiliser de manière autonome les informations du serveur de taux officiel pour attribuer un prêt.")
REPONSES = [{"question": "Validation ou refus ?", "reponse": "Seulement après validation humaine."},
            {"question": "Date de relecture ?", "reponse": "À relire avant le 31/12/2026."}]
DATE = "2026-10-04T00:00:00Z"


def compiler(client, **kw):
    kw.setdefault("reponses", REPONSES)
    return comp.compiler(TEXTE, "credit-pme", catalogue_exemple(), client,
                         auteur="juriste@cabinet.fr", date_effet=DATE, **kw)


class TestCompiler(unittest.TestCase):
    def test_questions(self):
        c = ClientScripte(reponse(
            appel("poser_question", {"question": "Validation ou refus ?", "champ": "effet",
                                     "options": ["attendre_validation", "refuser"]}, "a"),
            appel("poser_question", {"question": "Date de relecture ?", "champ": "revision_avant"}, "b")))
        r = compiler(c, reponses=[])
        self.assertEqual(r["etat"], comp.QUESTIONS)
        self.assertEqual([q["champ"] for q in r["questions"]], ["effet", "revision_avant"])
        self.assertIsNone(r["jeu"])

    def test_question_l_emporte_sur_une_proposition(self):
        c = ClientScripte(reponse(appel("proposer_directive", D001, "a"),
                                  appel("poser_question", {"question": "Sûr ?", "champ": "autre"}, "b")))
        r = compiler(c)
        self.assertEqual(r["etat"], comp.QUESTIONS)
        self.assertIsNone(r["jeu"])

    def test_directive_valide(self):
        p = dict(D001, statut="active", texte_fr="autre chose", id="X-9")
        r = compiler(ClientScripte(reponse(appel("proposer_directive", p))))
        self.assertEqual(r["etat"], comp.DIRECTIVE, r["erreurs"])
        d = r["jeu"]["directives"][0]
        self.assertEqual(d["statut"], "brouillon", "le statut n'est jamais celui de Mistral")
        self.assertEqual(d["texte_fr"], TEXTE, "texte_fr est le texte du juriste tel quel")
        self.assertEqual(d["id"], "D-001")
        self.assertEqual(d["date_effet"], DATE)
        self.assertEqual(d["revision_avant"], "2026-12-31T00:00:00Z")
        self.assertIn("Pour tous les appelants : si la session a reçu des données du serveur taux-officiel, "
                      "tout appel à attribuer_pret sur banque-synthetique exige une validation humaine.",
                      r["relecture"])
        self.assertEqual(r["tentatives"], 1)

    def test_correction_unique_reussie(self):
        mauvais = dict(D001, effet="caviarder")
        c = ClientScripte(reponse(appel("proposer_directive", mauvais, "a")),
                          reponse(appel("proposer_directive", D001, "b")))
        r = compiler(c)
        self.assertEqual(r["etat"], comp.DIRECTIVE, r["erreurs"])
        self.assertEqual(r["tentatives"], 2)
        messages = c.recus[1][0]
        self.assertEqual(messages[-1]["role"], "tool")
        self.assertEqual(messages[-1]["tool_call_id"], "a")
        self.assertIn('effet "caviarder" non admis', messages[-1]["content"])

    def test_second_echec_rendu_au_juriste(self):
        mauvais = dict(D001, effet="caviarder")
        c = ClientScripte(reponse(appel("proposer_directive", mauvais, "a")),
                          reponse(appel("proposer_directive", mauvais, "b")))
        r = compiler(c)
        self.assertEqual(r["etat"], comp.ERREURS)
        self.assertEqual(len(c.recus), 2, "une seule correction")
        self.assertTrue(any("caviarder" in e for e in r["erreurs"]))
        self.assertIsNone(r["relecture"])

    def test_montant_invente_refuse(self):
        p = {"verrou": "seuil_argument", "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"},
             "effet": "refuser", "fondement": {"texte": "RGPD", "article": "22"},
             "condition": [{"champ": "/montant", "op": ">", "valeur": 100000}]}
        c = ClientScripte(reponse(appel("proposer_directive", p)), reponse(appel("proposer_directive", p)))
        r = compiler(c)
        self.assertEqual(r["etat"], comp.ERREURS)
        self.assertTrue(any("100000" in e for e in r["erreurs"]), r["erreurs"])

    def test_si_indetermine(self):
        p = {"verrou": "seuil_argument", "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"},
             "effet": "refuser", "fondement": {"texte": "RGPD", "article": "22"},
             "condition": [{"champ": "/montant", "op": ">", "valeur": 500000}]}
        rep = REPONSES + [{"question": "Seuil ?", "reponse": "500 000 euros"}]
        r = compiler(ClientScripte(reponse(appel("proposer_directive", p))), reponses=rep)
        self.assertEqual(r["etat"], comp.DIRECTIVE, r["erreurs"])
        self.assertEqual(r["jeu"]["directives"][0]["si_indetermine"], "refuser")
        # Mistral ne peut pas obtenir ignorer…
        p2 = dict(p, si_indetermine="ignorer")
        r = compiler(ClientScripte(reponse(appel("proposer_directive", p2)), reponse(appel("proposer_directive", p2))),
                     reponses=rep)
        self.assertEqual(r["etat"], comp.ERREURS)
        # …seul le juriste, nommément.
        r = compiler(ClientScripte(reponse(appel("proposer_directive", p))), reponses=rep,
                     ignorer_valide_par="Me Dupont")
        d = r["jeu"]["directives"][0]
        self.assertEqual((d["si_indetermine"], d["si_indetermine_valide_par"]), ("ignorer", "Me Dupont"))

    def test_sans_outil(self):
        r = compiler(ClientScripte({"choices": [{"message": {"content": "Bonjour"}}]}))
        self.assertEqual(r["etat"], comp.ERREURS)

    def test_deux_propositions(self):
        c = ClientScripte(reponse(appel("proposer_directive", D001, "a"), appel("proposer_directive", D001, "b")),
                          reponse(appel("proposer_directive", D001, "c")))
        r = compiler(c)
        self.assertEqual(r["etat"], comp.DIRECTIVE)
        self.assertEqual(r["tentatives"], 2)

    def test_outils_bornes_par_le_catalogue(self):
        c = ClientScripte(reponse(appel("proposer_directive", D001)))
        compiler(c)
        outils = {o["function"]["name"]: o["function"]["parameters"] for o in c.recus[0][1]}
        self.assertEqual(set(outils), {"proposer_directive", "poser_question"})
        p = outils["proposer_directive"]["properties"]
        self.assertEqual(len(p["verrou"]["enum"]), 6)
        self.assertEqual(p["cible"]["properties"]["serveur"]["enum"],
                         ["banque-synthetique", "recherche-entreprises", "taux-officiel", "*"])
        self.assertNotIn("statut", p)
        self.assertNotIn("texte_fr", p)
        self.assertNotIn("si_indetermine", p)
        self.assertNotIn("autoriser", p["effet"]["enum"])


class TestFondement(unittest.TestCase):
    def test_decoupage(self):
        cas = [
            ({"texte": "Art. 22 RGPD"}, {"texte": "RGPD", "article": "22"}),
            ({"texte": "RGPD", "article": "art. 22"}, {"texte": "RGPD", "article": "22"}),
            ({"texte": "article 1240 du Code civil"}, {"texte": "Code civil", "article": "1240"}),
            ({"texte": "l'art. 5.1.c RGPD"}, {"texte": "RGPD", "article": "5.1.c"}),
            ({"texte": "Art. L561-10 Code monétaire et financier"}, {"texte": "Code monétaire et financier", "article": "L561-10"}),
            ({"texte": "RGPD"}, {"texte": "RGPD"}),
        ]
        for entree, veut in cas:
            self.assertEqual(comp.fondement(entree), veut, entree)


class TestAffichage(unittest.TestCase):
    def test_cote_a_cote(self):
        s = affichage.cote_a_cote("gauche " * 30, "droite\n\nsuite")
        lignes = s.splitlines()
        self.assertTrue(all("│" in l for l in lignes))
        self.assertTrue(lignes[0].startswith("Texte du juriste"))
        self.assertEqual(s, affichage.cote_a_cote("gauche " * 30, "droite\n\nsuite"))


if __name__ == "__main__":
    unittest.main()
