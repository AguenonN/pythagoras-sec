"""Démo texte v2 : texte → questions → relecture → fichier de jeu brouillon,
rejouée sur les réponses réelles enregistrées du scénario week-end."""

import json
import os
import tempfile
import unittest
from unittest import mock

from compilateur import demo, scenarios_v2

from .aides import CATALOGUE_DEMO, CATALOGUE_V2_ENREGISTRE, VOCABULAIRE_V2_ENREGISTRE, ClientScripte, appel, reponse


def lancer(saisies, *args, schema=(), catalogue=CATALOGUE_DEMO):
    saisies = list(saisies)
    ecrit = []
    with tempfile.TemporaryDirectory() as tmp:
        sortie = os.path.join(tmp, "jeu.json")
        rc = demo.principal(list(schema) + ["--jeu", "weekend", "--catalogue", catalogue,
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
    def test_v2_seul(self):
        # La démo compile en v2 : c'est proposer_jeu qui est offert ; --v1 n'existe plus.
        s = reponse(appel("signaler_non_observable", {"extrait": "x", "raison": "Pas observable.", "question": "Critère ?"}))
        cl = ClientScripte(*[s] * 5)
        with mock.patch("compilateur.demo.mistral.Client", return_value=cl):
            rc, ecrit, _, _, _ = lancer(["non"] * 5, "--texte", "x", "--date-effet", "2026-10-05", schema=())
        self.assertEqual(rc, 1)
        self.assertIn("refuse de traduire un critère non observable", ecrit)
        self.assertIn("proposer_jeu", [o["function"]["name"] for o in cl.recus[0][1]])
        for option in ("--v1", "--v2"):
            with self.assertRaises(SystemExit):
                lancer([], "--texte", "x", "--date-effet", "2026-10-05", option)

    def test_parcours_week_end(self):
        sc = scenarios_v2.lire("weekend")
        rep = {r.get("question") or r["champ"]: r["reponse"] for r in sc["reponses"]}
        saisies = [sc["texte"], "2026-10-05",
                   rep["Le juriste entend-il bloquer aussi la consultation des workflows le week-end, ou seulement leur exécution (actions) ?"],
                   rep["Quel est le fondement juridique applicable pour cette règle ?"],
                   rep["Quel fuseau horaire appliquer pour le week-end ?"],
                   "peut-être", "refuser"]
        rc, ecrit, jeu, consigne, reste = lancer(saisies, "--prefixe-id", "W", "--vocabulaire", VOCABULAIRE_V2_ENREGISTRE,
                                                 "--modele", "mistral-small-latest",
                                                 catalogue=CATALOGUE_V2_ENREGISTRE)
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


class TestDemoJeuExistant(unittest.TestCase):
    """--jeu-existant : la démo texte rend la version n+1 du jeu de démo,
    v2 par défaut, la directive ajoutée numérotée à la suite."""

    def test_ajout_au_jeu_de_demo(self):
        from compilateur import directive_go
        demo_json = os.path.join(directive_go.RACINE, "demo", "directives", "demo.json")
        propo = reponse(appel("proposer_jeu", {"directives": [{
            "extrait": "Aucune action d'agent le week-end, heure de Paris",
            "fondements": [{"texte": "Règlement intérieur", "article": "12"}],
            "declencheur": {"serveur": "*", "outil": "*", "acces": "action"}, "racine": "n1",
            "noeuds": [{"id": "n1", "type": "temps", "jours": ["samedi", "dimanche"], "fuseau": "Europe/Paris"}],
            "effets": [{"effet": "refuser"}]}]}))
        cl = ClientScripte(propo, propo)
        with mock.patch("compilateur.demo.mistral.Client", return_value=cl):
            rc, ecrit, jeu, _, _ = lancer(["refuser"], "--texte", "Aucune action d'agent le week-end, heure de Paris "
                                          "(Règlement intérieur, art. 12).", "--date-effet", "2026-10-05",
                                          "--jeu-existant", demo_json, schema=())
        self.assertEqual(rc, 0, ecrit)
        with open(demo_json, encoding="utf-8") as f:
            demo_jeu = json.load(f)
        self.assertEqual((jeu["jeu"]["id"], jeu["jeu"]["version"]), ("demo", demo_jeu["jeu"]["version"] + 1))
        self.assertEqual(jeu["directives"][:-1], demo_jeu["directives"])
        self.assertEqual((jeu["directives"][-1]["id"], jeu["directives"][-1]["statut"]), ("D-009", "brouillon"))
        self.assertIn("Ajout au jeu demo", ecrit)
        self.assertNotIn("Directive D-001", ecrit)
