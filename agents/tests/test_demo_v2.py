"""Démo texte v2 : texte → questions → relecture → fichier de jeu brouillon,
rejouée sur les réponses réelles enregistrées du scénario week-end."""

import json
import os
import tempfile
import unittest
from unittest import mock

from compilateur import demo, scenarios_v2

from .aides import CATALOGUE_DEMO, ClientScripte, appel, reponse


def lancer(saisies, *args):
    saisies = list(saisies)
    ecrit = []
    with tempfile.TemporaryDirectory() as tmp:
        sortie = os.path.join(tmp, "jeu.json")
        rc = demo.principal(["--v2", "--jeu", "weekend", "--catalogue", CATALOGUE_DEMO,
                             "--auteur", "juriste@cabinet.fr", "--sortie", sortie] + list(args),
                            lire=lambda _: saisies.pop(0), ecrire=ecrit.append)
        jeu = consigne = None
        if os.path.exists(sortie):
            with open(sortie, encoding="utf-8") as f:
                jeu = json.load(f)
            with open(sortie + ".reponses.json", encoding="utf-8") as f:
                consigne = json.load(f)
    return rc, "".join(ecrit), jeu, consigne, saisies


class TestDemoV2(unittest.TestCase):
    def test_parcours_week_end(self):
        sc = scenarios_v2.lire("weekend")
        rep = {r.get("question") or r["champ"]: r["reponse"] for r in sc["reponses"]}
        saisies = [sc["texte"], "2026-10-05",
                   rep["Le juriste entend-il bloquer aussi la consultation des workflows le week-end, ou seulement leur exécution (actions) ?"],
                   rep["Quel est le fondement juridique applicable pour cette règle ?"],
                   rep["Quel fuseau horaire appliquer pour le week-end ?"],
                   "peut-être", "refuser"]
        rc, ecrit, jeu, consigne, reste = lancer(saisies, "--prefixe-id", "W")
        self.assertEqual(rc, 0, ecrit)
        self.assertEqual(reste, [])
        self.assertIn("besoin de précisions", ecrit)
        self.assertIn("Réponse attendue : refuser ou ignorer.", ecrit, "une question du code est fermée")
        self.assertIn("Relecture : ce qui sera appliqué", ecrit)
        self.assertIn("Europe/Paris", ecrit)
        self.assertIn("Jeu écrit en brouillon", ecrit)
        d, = jeu["directives"]
        self.assertEqual(d["statut"], "brouillon")
        self.assertEqual(d["condition"], {"temps": {"jour_semaine": ["samedi", "dimanche"], "fuseau": "Europe/Paris"}})
        self.assertEqual(consigne["consignation"][0]["champ"], "si_indetermine")

    def test_refus_motive_affiche(self):
        s = reponse(appel("signaler_non_observable", {"extrait": "ne pas discriminer", "raison": "Pas observable.",
                                                      "question": "Quel critère ?"}))
        with mock.patch("compilateur.demo.mistral.Client", return_value=ClientScripte(*[s] * 5)):
            rc, ecrit, jeu, _, _ = lancer(["non"] * 5, "--texte", "Ne pas discriminer.", "--date-effet", "2026-10-05")
        self.assertEqual(rc, 1)
        self.assertIsNone(jeu)
        self.assertIn("refuse de traduire un critère non observable", ecrit)
        self.assertIn("Pas observable.", ecrit)
        self.assertIn("ne convergent pas", ecrit)

    def test_rien_n_est_ecrit_sur_refus(self):
        mauvais = {"directives": [{"extrait": "x", "fondements": [{"texte": "RGPD"}], "declencheur": {"serveur": "*"},
                                   "effets": [{"effet": "caviarder", "champs_autorises": ["/siren"], "mode": "caviarder"}]}]}
        cl = ClientScripte(reponse(appel("proposer_jeu", mauvais)), reponse(appel("proposer_jeu", mauvais)))
        with mock.patch("compilateur.demo.mistral.Client", return_value=cl):
            rc, ecrit, jeu, _, _ = lancer([], "--texte", "RGPD : x.", "--date-effet", "2026-10-05")
        self.assertEqual(rc, 1)
        self.assertIsNone(jeu)
        self.assertIn("Jeu refusé par le vérificateur", ecrit)
