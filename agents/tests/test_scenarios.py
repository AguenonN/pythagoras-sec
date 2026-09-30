"""Scénarios du brief, rejoués sur les réponses réelles de Mistral
enregistrées dans compilateur/enregistrements/. Aucun appel réseau : le
client est en mode rejeu et lève si une requête n'a pas été enregistrée."""

import json
import os
import unittest

from compilateur import compilateur as comp
from compilateur import mistral

from .aides import EXEMPLES, catalogue_exemple

ICI = os.path.dirname(os.path.abspath(__file__))

D001 = ("En application de l'art. 22 RGPD, en attendant des solutions concrètes, plus aucun workload ne doit "
        "utiliser de manière autonome les informations du serveur de taux officiel pour attribuer un prêt.")
SEUIL = "En application de l'art. 22 RGPD, au-delà d'un montant significatif, l'attribution d'un prêt est bloquée."
# gels-avoirs est un nom autorisé mais absent du catalogue fourni.
INCONNU = ("En application de l'art. 22 RGPD, aucun prêt ne doit être attribué sur la base des informations "
           "du serveur gels-avoirs.")
PARTENAIRE = "En application de l'art. 5 RGPD, le serveur partenaire ne reçoit que le SIREN."


def reponses(nom):
    with open(os.path.join(EXEMPLES, nom), encoding="utf-8") as f:
        return json.load(f)


def compiler(texte, reps=()):
    return comp.compiler(texte, "credit-pme", catalogue_exemple(), mistral.Client(mode="rejeu"),
                         auteur="juriste@cabinet.fr", date_effet="2026-10-04T00:00:00Z", reponses=reps)


class TestExempleD001(unittest.TestCase):
    def test_pose_les_bonnes_questions(self):
        r = compiler(D001)
        self.assertEqual(r["etat"], comp.QUESTIONS)
        self.assertIsNone(r["jeu"])
        champs = sorted(q["champ"] for q in r["questions"])
        self.assertEqual(champs, ["effet", "revision_avant"])
        effet = next(q for q in r["questions"] if q["champ"] == "effet")
        self.assertEqual(sorted(effet["options"]), ["attendre_validation", "refuser"],
                         "« de manière autonome » : validation humaine ou interdiction")

    def test_les_questions_enregistrees_sont_celles_des_reponses(self):
        posees = [q["question"] for q in compiler(D001)["questions"]]
        self.assertEqual(sorted(posees), sorted(r["question"] for r in reponses("reponses-d001.json")))

    def test_completee_directive_acceptee_et_relecture_stable(self):
        r = compiler(D001, reponses("reponses-d001.json"))
        self.assertEqual(r["etat"], comp.DIRECTIVE, r["erreurs"])
        d = r["jeu"]["directives"][0]
        self.assertEqual(d["statut"], "brouillon")
        self.assertEqual(d["texte_fr"], D001)
        self.assertEqual(d["verrou"], "provenance_session")
        self.assertEqual(d["cible"], {"serveur": "banque-synthetique", "outil": "attribuer_pret"})
        self.assertEqual(d["provenance"]["serveurs"], ["taux-officiel"])
        self.assertEqual(d["effet"], "attendre_validation")
        self.assertEqual(d["revision_avant"], "2026-12-31T00:00:00Z")
        self.assertEqual(d["fondement"], {"texte": "RGPD", "article": "22"})

        # Acceptée par le chargeur strict, indépendamment du compilateur.
        from compilateur import directive_go
        ok, erreurs, _ = directive_go.verifier(r["jeu"])
        self.assertTrue(ok, erreurs)

        with open(os.path.join(ICI, "attendus", "d001.txt"), encoding="utf-8") as f:
            attendu = f.read()
        self.assertEqual(r["relecture"], attendu)
        for _ in range(3):
            self.assertEqual(compiler(D001, reponses("reponses-d001.json"))["relecture"], attendu)


class TestSeuilNonChiffre(unittest.TestCase):
    def test_question_jamais_un_montant(self):
        r = compiler(SEUIL)
        self.assertEqual(r["etat"], comp.QUESTIONS)
        self.assertEqual([q["champ"] for q in r["questions"]], ["condition"])
        self.assertIsNone(r["jeu"])

    def test_montant_donne_par_le_juriste(self):
        r = compiler(SEUIL, reponses("reponses-seuil.json"))
        self.assertEqual(r["etat"], comp.DIRECTIVE, r["erreurs"])
        d = r["jeu"]["directives"][0]
        self.assertEqual(d["verrou"], "seuil_argument")
        self.assertEqual(d["condition"], [{"champ": "/montant", "op": ">", "valeur": 500000}])
        self.assertEqual(d["si_indetermine"], "refuser")
        self.assertEqual(d["effet"], "refuser")


class TestServeurInexistant(unittest.TestCase):
    def test_question_jamais_un_serveur_invente(self):
        r = compiler(INCONNU)
        self.assertIn(r["etat"], (comp.QUESTIONS, comp.ERREURS))
        self.assertNotEqual(r["etat"], comp.DIRECTIVE)
        self.assertIn("gels-avoirs", " ".join(q["question"] for q in r["questions"]))
        self.assertIsNone(r["relecture"])


class TestChampsPartenaire(unittest.TestCase):
    def test_champs_autorises_siren(self):
        premier = compiler(PARTENAIRE)
        self.assertEqual(premier["etat"], comp.QUESTIONS, "le « serveur partenaire » est confirmé, pas supposé")
        r = compiler(PARTENAIRE, reponses("reponses-partenaire.json"))
        self.assertEqual(r["etat"], comp.DIRECTIVE, r["erreurs"])
        d = r["jeu"]["directives"][0]
        self.assertEqual(d["verrou"], "champs_autorises_par_destination")
        self.assertEqual(d["cible"]["serveur"], "recherche-entreprises")
        self.assertEqual(d["champs"], {"autorises": ["/siren"], "mode": "caviarder"})
        self.assertEqual(d["effet"], "caviarder")


if __name__ == "__main__":
    unittest.main()


class TestTourDeCorrection(unittest.TestCase):
    """Le format du renvoi des erreurs (message assistant avec tool_calls,
    puis message tool) a été accepté par l'API réelle ; la réponse est
    enregistrée. Le premier refus est simulé."""

    def test_erreurs_renvoyees_une_fois_puis_acceptee(self):
        from unittest import mock
        from compilateur import directive_go
        vrai, n = directive_go.verifier, []

        def verifier(jeu):
            n.append(1)
            if len(n) == 1:
                return False, ["directive D-001: provenance.critere doit être explicite (tour de correction)"], ""
            return vrai(jeu)

        with mock.patch.object(directive_go, "verifier", verifier):
            r = compiler(D001, reponses("reponses-d001.json"))
        self.assertEqual((r["etat"], r["tentatives"]), (comp.DIRECTIVE, 2), r["erreurs"])
        self.assertEqual(r["jeu"]["directives"][0]["provenance"],
                         {"serveurs": ["taux-officiel"], "critere": "donnees_recues"})
