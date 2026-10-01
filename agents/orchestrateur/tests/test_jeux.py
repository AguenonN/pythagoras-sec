"""Le jeu de la démo (contrats-demo.md §1) et le jeu de travail du scénario
« base interne » passent le chargeur strict (directive verifier) et se
relisent sans gabarit de repli (directive relire). Le démarrage du gate
devant le registre de démo est contrôlé par
internal/registredemo.TestTousLesJeuxDemo, qui charge tous les jeux de
demo/directives."""

import json
import os
import re
import unittest

from compilateur import directive_go

RACINE = directive_go.RACINE
DEMO = os.path.join(RACINE, "demo", "directives", "demo.json")
AVEC_D009 = os.path.join(RACINE, "demo", "scenarios", "jeux", "avec-d009.json")
# Gabarits de repli de internal/relecture : ce qu'elle écrit quand elle ne
# reconnaît pas une partie de la directive.
REPLI = re.compile(r"(évènement|accès|opérateur|champ|rythme|effet) inconnu ")


def lire(chemin):
    with open(chemin, encoding="utf-8") as f:
        return json.load(f)


class TestJeux(unittest.TestCase):
    def verifier(self, chemin):
        jeu = lire(chemin)
        ok, erreurs, _ = directive_go.verifier(jeu, exiger_brouillon=False)
        self.assertTrue(ok, erreurs)
        relecture = directive_go.relire(jeu)
        self.assertIsNone(REPLI.search(relecture), relecture)
        return jeu, relecture

    def test_demo(self):
        jeu, relecture = self.verifier(DEMO)
        self.assertEqual((jeu["jeu"]["id"], jeu["jeu"]["version"]), ("demo", 1))
        self.assertEqual([d["id"] for d in jeu["directives"]], ["D-00%d" % i for i in range(1, 9)])
        for d in jeu["directives"]:
            self.assertEqual(d["statut"], "active")
            self.assertIn("à valider par le juriste", d["fondements"][0]["texte"])
            self.assertIn("Directive %s, version 1, active" % d["id"], relecture)
        effets = {d["id"]: [e["effet"] for e in d["effets"]] for d in jeu["directives"]}
        self.assertEqual(effets, {"D-001": ["refuser"], "D-002": ["attendre_validation"], "D-003": ["refuser"],
                                  "D-004": ["caviarder"], "D-005": ["attendre_validation", "notifier"],
                                  "D-006": ["refuser"], "D-007": ["attendre_validation"],
                                  "D-008": ["refuser", "notifier"]})

    def test_avec_d009_identique_a_demo_plus_d009(self):
        demo = lire(DEMO)
        jeu, _ = self.verifier(AVEC_D009)
        self.assertEqual(jeu["jeu"]["id"], "demo")
        self.assertEqual(jeu["directives"][:-1], demo["directives"])
        d = jeu["directives"][-1]
        self.assertEqual(d["id"], "D-009")
        self.assertEqual(d["condition"], {"session.a_consulte": {"outil": "banque-synthetique/get_taux_interne",
                                                                 "critere": "transmis"}})
        self.assertEqual(d["effets"], [{"effet": "attendre_validation", "par_role": "analyste-risques"}])


if __name__ == "__main__":
    unittest.main()
