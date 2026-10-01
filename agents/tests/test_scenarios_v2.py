"""Scénarios v2 de la démo, rejoués sur les réponses réelles de Mistral
enregistrées (compilateur/enregistrements) : questions attendues, puis jeu
accepté par le chargeur strict et relecture stable. Aucun appel réseau :
le client est en mode rejeu et lève si une requête n'a pas été enregistrée.

Pour réécrire les relectures attendues : RELECTURE_MAJ=1."""

import json
import os
import unittest

from compilateur import compilateur_v2 as c2
from compilateur import directive_go, mistral, scenarios_v2

ICI = os.path.dirname(os.path.abspath(__file__))
ATTENDUS = os.path.join(ICI, "attendus", "v2")


def derouler(nom):
    return scenarios_v2.derouler(scenarios_v2.lire(nom), mistral.Client(mode="rejeu"))


class Scenario:
    nom = None
    sujets = None  # sujets des questions posées, tour par tour

    @classmethod
    def setUpClass(cls):
        cls.r, cls.tours = derouler(cls.nom)

    def test_questions_attendues(self):
        self.assertEqual([[q.get("champ") if q.get("origine") else q["sujet"] for q in t["questions"]]
                          for t in self.tours[:-1]], self.sujets)

    def test_jeu_accepte_et_relecture_stable(self):
        self.assertEqual(self.r["etat"], c2.JEU, self.r["erreurs"])
        jeu = self.r["jeu"]
        self.assertTrue(all(d["statut"] == "brouillon" for d in jeu["directives"]))
        ok, erreurs, _ = directive_go.verifier(jeu)
        self.assertTrue(ok, erreurs)
        chemin = os.path.join(ATTENDUS, self.nom + ".txt")
        if os.environ.get("RELECTURE_MAJ"):
            os.makedirs(ATTENDUS, exist_ok=True)
            with open(chemin, "w", encoding="utf-8") as f:
                f.write(self.r["relecture"])
        with open(chemin, encoding="utf-8") as f:
            attendu = f.read()
        self.assertEqual(self.r["relecture"], attendu)
        for _ in range(2):
            self.assertEqual(derouler(self.nom)[0]["relecture"], attendu)

    def directives(self):
        return self.r["jeu"]["directives"]


class TestCredit(Scenario, unittest.TestCase):
    nom = "credit"
    sujets = [["seuil", "etiquette", "cible"], ["si_indetermine"]]

    def test_un_texte_quatre_directives(self):
        ds = self.directives()
        self.assertEqual([d["id"] for d in ds], ["C-001", "C-002", "C-003", "C-004"])
        pret = {"evenement": "appel_outil", "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"}}
        self.assertEqual([d["declencheur"] for d in ds[:3]], [pret] * 3)
        self.assertEqual(ds[0]["condition"], {"non": {"session.a_consulte": {"outil": "gels-avoirs/verifier_gel",
                                                                             "critere": "donnees_recues"}}})
        self.assertEqual(ds[1]["condition"], {"argument": {"champ": "/montant", "op": ">", "valeur": 500000}})
        self.assertEqual(ds[2]["condition"], {"session.a_consulte": {"serveur": "taux-officiel", "critere": "donnees_recues"}})
        self.assertEqual([d["effets"] for d in ds[:3]], [[{"effet": "refuser"}], [{"effet": "refuser"}],
                                                         [{"effet": "attendre_validation"}]])
        self.assertEqual(ds[3]["declencheur"]["cible"], {"serveur": "recherche-entreprises", "outil": "*"})
        self.assertEqual(ds[3]["effets"], [{"effet": "caviarder", "champs_autorises": ["/siren"], "mode": "caviarder"}])
        self.assertEqual([d["fondements"] for d in ds], [
            [{"texte": "Code monétaire et financier", "article": "L562-4"}],
            [{"texte": "Politique de crédit", "article": "3.2"}],
            [{"texte": "RGPD", "article": "22"}], [{"texte": "RGPD", "article": "5"}]])
        texte = scenarios_v2.lire("credit")["texte"]
        for d in ds:
            self.assertIn(d["texte_fr"], texte)


class TestWeekend(Scenario, unittest.TestCase):
    nom = "weekend"
    sujets = [["consultation_ou_production"], ["fondement", "fuseau"], ["si_indetermine"]]

    def test_identique_au_jeu_de_reference(self):
        chemin = os.path.join(ICI, "..", "..", "internal", "directive", "testdata", "v2", "weekend.json")
        with open(chemin, encoding="utf-8") as f:
            ref = json.load(f)["directives"][0]
        d, = self.directives()
        for cle in ("declencheur", "condition", "effets", "fondements"):
            self.assertEqual(d[cle], ref[cle], cle)
        self.assertEqual(d["texte_fr"], scenarios_v2.lire("weekend")["texte"])
