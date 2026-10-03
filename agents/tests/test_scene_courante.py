"""Secours hors ligne de la scène de rédaction, sur le catalogue, le
vocabulaire et le demo.json actuels (session gel, 03/10) : les parcours
« base interne » puis « week-end » se rejouent jusqu'à D-009 puis D-010 ;
le texte brut de la rémunération, jusqu'à ses questions. Les réponses de
Mistral ont été enregistrées en réel (mistral-medium-latest)."""

import json
import os
import shutil
import tempfile
import unittest

from compilateur import service

ICI = os.path.dirname(os.path.abspath(__file__))
RACINE = os.path.dirname(os.path.dirname(ICI))
SERVICE = os.path.join(RACINE, "agents", "compilateur", "exemples", "service")


def lire(chemin):
    with open(chemin, encoding="utf-8") as f:
        return json.load(f)


class TestSceneCourante(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.tmp)

    def derouler(self, chemin, jeu):
        requete, reponses = service.lire_parcours(chemin)
        s = service.Service(mode="rejeu", dossier=self.tmp)
        return service.parcours(s, dict(requete, jeu_existant=jeu), reponses)

    def test_base_interne_puis_week_end(self):
        fin = self.derouler(os.path.join(SERVICE, "base-interne.json"), "demo/directives/demo.json")[-1]
        self.assertEqual(fin["etat"], "jeu", fin["erreurs"])
        cand = lire(service.chemin_absolu(fin["jeu_candidat"]))
        d9 = cand["directives"][-1]
        self.assertEqual((d9["id"], d9["statut"]), ("D-009", "brouillon"))
        self.assertEqual(d9["condition"], {"session.a_consulte": {"outil": "banque-synthetique/get_taux_interne",
                                                                  "critere": "donnees_recues"}})
        self.assertEqual(d9["effets"], [{"effet": "attendre_validation", "par_role": "analyste-risques"}])
        d9["statut"] = "active"
        avec = os.path.join(self.tmp, "avec-d009.json")
        with open(avec, "w", encoding="utf-8") as f:
            json.dump(cand, f, ensure_ascii=False)
        fin = self.derouler(os.path.join(SERVICE, "weekend.json"), avec)[-1]
        self.assertEqual(fin["etat"], "jeu", fin["erreurs"])
        d10 = lire(service.chemin_absolu(fin["jeu_candidat"]))["directives"][-1]
        self.assertEqual(d10["id"], "D-010")
        self.assertEqual(d10["condition"], {"temps": {"jour_semaine": ["samedi", "dimanche"], "fuseau": "Europe/Paris"}})
        self.assertEqual(d10["effets"], [{"effet": "refuser"}])

    def test_texte_brut_remuneration(self):
        sorties = self.derouler(os.path.join(RACINE, "demo", "scenarios", "compilation", "A5-brut.json"),
                                "demo/directives/demo.json")
        self.assertEqual([s["etat"] for s in sorties], ["questions"])
        self.assertTrue(any("messagerie" in q["texte"] for q in sorties[0]["questions"]))


if __name__ == "__main__":
    unittest.main()
