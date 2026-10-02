"""Le jeu de la démo (contrats-demo.md §1) et le jeu de travail du scénario
« base interne » passent le chargeur strict (directive verifier) et se
relisent sans gabarit de repli (directive relire). Le démarrage du gate
devant le registre de démo est contrôlé par
internal/registredemo.TestTousLesJeuxDemo, qui charge demo.json, son
historique et les jeux de test de l'intégration."""

import json
import os
import re
import unittest

from compilateur import directive_go

RACINE = directive_go.RACINE
DEMO = os.path.join(RACINE, "demo", "directives", "demo.json")
AVEC_D009 = os.path.join(RACINE, "demo", "scenarios", "jeux", "avec-d009.json")
HISTORIQUE_V1 = os.path.join(RACINE, "demo", "directives", "historique", "demo.v1.json")
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
        # Version 2 (02/10, décision de Manu) : D-001 à D-008 actives, puis
        # les directives des secteurs D-011 à D-023 en brouillon ; D-009 et
        # D-010 restent libres pour la scène.
        self.assertEqual((jeu["jeu"]["id"], jeu["jeu"]["version"]), ("demo", 2))
        self.assertEqual([d["id"] for d in jeu["directives"]],
                         ["D-00%d" % i for i in range(1, 9)] + ["D-%03d" % i for i in range(11, 24)])
        self.assertEqual(lire(HISTORIQUE_V1)["directives"], jeu["directives"][:8])
        for d in jeu["directives"]:
            statut = "active" if d["id"] < "D-009" else "brouillon"
            self.assertEqual(d["statut"], statut)
            self.assertIn("à valider par le juriste", " ".join(f["texte"] for f in d["fondements"]))
            self.assertIn("Directive %s, version 1, %s" % (d["id"], statut), relecture)
        effets = {d["id"]: [e["effet"] for e in d["effets"]] for d in jeu["directives"][:8]}
        self.assertEqual(effets, {"D-001": ["refuser"], "D-002": ["attendre_validation"], "D-003": ["refuser"],
                                  "D-004": ["caviarder"], "D-005": ["attendre_validation", "notifier"],
                                  "D-006": ["refuser"], "D-007": ["attendre_validation"],
                                  "D-008": ["refuser", "notifier"]})

    def test_avec_d009_identique_a_demo_plus_d009(self):
        demo = lire(DEMO)
        jeu, _ = self.verifier(AVEC_D009)
        self.assertEqual(jeu["jeu"]["id"], "demo")
        self.assertEqual(jeu["directives"][:-1], demo["directives"][:8])
        d = jeu["directives"][-1]
        self.assertEqual(d["id"], "D-009")
        # Base interne consultée, taux officiel non consulté (D-002 ne s'applique alors pas).
        self.assertEqual(d["condition"], {"et": [
            {"session.a_consulte": {"outil": "banque-synthetique/get_taux_interne", "critere": "transmis"}},
            {"non": {"session.a_consulte": {"serveur": "taux-officiel", "critere": "transmis"}}}]})
        self.assertEqual(d["effets"], [{"effet": "attendre_validation", "par_role": "analyste-risques"}])


if __name__ == "__main__":
    unittest.main()
