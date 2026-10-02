"""Scénarios v2 de la démo, rejoués sur les réponses réelles de Mistral
enregistrées (compilateur/enregistrements) : questions attendues, puis jeu
accepté par le chargeur strict et relecture stable. Aucun appel réseau :
le client est en mode rejeu et lève si une requête n'a pas été enregistrée.

Pour réécrire les relectures attendues : RELECTURE_MAJ=1."""

import json
import os
import unittest

from compilateur import compilateur_v2 as c2
from compilateur import directive_go, mistral, scenarios_v2, vocabulaire

from .aides import CATALOGUE_V2_ENREGISTRE, VOCABULAIRE_V2_ENREGISTRE

ICI = os.path.dirname(os.path.abspath(__file__))
ATTENDUS = os.path.join(ICI, "attendus", "v2")


def derouler_gele(scenario, client):
    """Déroule sur le catalogue et le vocabulaire gelés des enregistrements (voir aides)."""
    vocab = vocabulaire.Vocabulaire.lire(CATALOGUE_V2_ENREGISTRE, VOCABULAIRE_V2_ENREGISTRE)
    return scenarios_v2.derouler(scenario, client, vocab)


def derouler(nom):
    return derouler_gele(scenarios_v2.lire(nom), mistral.Client(mode="rejeu"))


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
            derouler_gele(scenarios_v2.lire("top-management"), client)
        r, tours = derouler_gele(dict(scenarios_v2.lire("top-management"), reponses=[]),
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


class TestDemandeur(Scenario, unittest.TestCase):
    """« Si l'utilisateur est membre du groupe X, toute rédaction de rapport
    basée sur des données client est bloquée pour lui. » Mistral demande le
    groupe ; sa proposition suivante (étiquettes et fondement non écrits par
    le juriste) est refusée par le code, et la correction pose les questions
    attendues : les étiquettes de « rapport » et de « données client », le
    fondement."""
    nom = "demandeur"
    sujets = [["groupe"], ["etiquette", "etiquette", "fondement"], ["designation_personne", "si_indetermine"]]

    def test_question_du_code_demandeur_ou_personne_concernee(self):
        q = self.tours[2]["questions"][0]
        self.assertEqual((q["champ"], q["origine"]), ("designation_personne", "compilateur"))
        self.assertIn("« utilisateur »", q["question"])
        self.assertEqual([(c["champ"], c["reponse"]) for c in self.r["consignation"]],
                         [("designation_personne", "demandeur"), ("si_indetermine", "refuser")])

    def test_reponse_contraire_refusee(self):
        sc = scenarios_v2.lire("demandeur")
        sc["reponses"] = [dict(r, reponse="personne_concernee") if r.get("champ") == "designation_personne" else r
                          for r in sc["reponses"]]
        r, _ = derouler_gele(sc, mistral.Client(mode="rejeu"))
        self.assertEqual(r["etat"], c2.REFUSE)
        self.assertIn("aucune condition du jeu ne porte sur une feuille sujet", r["erreurs"][0])
        self.assertIn("directive U-001 (feuille demandeur)", r["erreurs"][0])

    def test_demandeur_et_etiquettes(self):
        d, = self.directives()
        self.assertEqual(d["declencheur"], {"evenement": "appel_outil",
                                            "cible": {"serveur": "*", "outil": "*", "etiquette": "production_rapport"}})
        self.assertEqual(d["condition"], {"et": [
            {"demandeur": {"groupe": "prestataires-externes"}},
            {"session.a_consulte": {"etiquette": "donnees_client", "critere": "donnees_recues"}}]})
        self.assertEqual(d["effets"], [{"effet": "refuser"}])
        self.assertEqual(d["fondements"], [{"texte": "RGPD", "article": "32"}])


class TestRH(unittest.TestCase):
    """Règle RH des groupes X et Y. Mistral demande à quoi correspondent X et
    Y ; le juriste répond (contentieux-en-cours, salaries-proteges, et les
    deux outils d'action). La proposition réelle est juste sur le fond (une
    directive par outil : existe sur /salarie_ids, sujet sur /salarie_id),
    mais le code la refuse pour deux défauts : un accès « action » qui ne
    restreint rien sur un outil nommé, et un article de fondement omis. La
    correction n'a pas été obtenue : budget de 30 appels atteint."""

    def test_questions_puis_proposition_refusee_par_le_code(self):
        r, tours = derouler_gele(dict(scenarios_v2.lire("rh"), reponses=[]), mistral.Client(mode="rejeu"))
        self.assertEqual(r["etat"], c2.QUESTIONS)
        self.assertEqual([q["sujet"] for q in r["questions"]], ["groupe", "groupe"])
        self.assertIn("groupe X", r["questions"][0]["question"])
        self.assertIn("groupe Y", r["questions"][1]["question"])

        client = _RejeuSansCorrection()
        with self.assertRaises(_Arret):
            derouler_gele(scenarios_v2.lire("rh"), client)
        renvoi = client.correction[-1]["content"]
        self.assertIn("acces action ne restreint rien", renvoi)
        self.assertIn("le juriste cite l'article L1121-1 de Code du travail", renvoi)
        propose = json.loads(client.correction[-2]["tool_calls"][0]["function"]["arguments"])["directives"]
        self.assertEqual([(d["declencheur"]["outil"], d["noeuds"][0]["type"]) for d in propose],
                         [("decider_prime", "existe"), ("proposer_sanction", "sujet"),
                          ("decider_prime", "existe"), ("proposer_sanction", "sujet")])
