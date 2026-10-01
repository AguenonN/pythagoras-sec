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


class _Arret(Exception):
    pass


class _RejeuSansCorrection:
    """Rejoue les tours enregistrés ; à la première demande de correction
    (message « tool »), s'arrête en gardant les messages envoyés."""

    def __init__(self):
        self.rejeu = mistral.Client(mode="rejeu")
        self.correction = None

    def completer(self, messages, outils, **options):
        if messages[-1]["role"] == "tool":
            self.correction = messages
            raise _Arret()
        return self.rejeu.completer(messages, outils, **options)


class TestTopManagement(unittest.TestCase):
    """Le critère « impliquant une décision avec le top management » n'est
    pas observable : Mistral refuse en expliquant et demande un critère. Le
    juriste le donne (2 M€, directeur-risques, quatre yeux, notification au
    juridique). La proposition réelle de Mistral ajoute alors une condition
    que le juriste n'a pas écrite : le code la refuse et la renvoie. (La
    correction de Mistral n'a pas pu être obtenue : génération interrompue
    côté Mistral, voir le rapport.)"""

    def test_refus_motive_puis_condition_inventee_refusee(self):
        client = _RejeuSansCorrection()
        with self.assertRaises(_Arret):
            scenarios_v2.derouler(scenarios_v2.lire("top-management"), client)
        r, tours = scenarios_v2.derouler(dict(scenarios_v2.lire("top-management"), reponses=[]),
                                         mistral.Client(mode="rejeu"))
        self.assertEqual(r["etat"], c2.NON_OBSERVABLE)
        self.assertIsNone(r["jeu"])
        self.assertIn("top management", r["non_observables"][0]["raison"])
        self.assertEqual(r["questions"][0]["sujet"], "non_observable")
        renvoi = client.correction[-1]["content"]
        self.assertIn("Refusé par le vérificateur", renvoi)
        self.assertIn("la source banque-synthetique n'est pas désignée par le juriste", renvoi)
        propose = json.loads(client.correction[-2]["tool_calls"][0]["function"]["arguments"])
        d, = propose["directives"]
        self.assertEqual(d["effets"][0], {"effet": "attendre_validation", "par_role": "directeur-risques", "quatre_yeux": True})
