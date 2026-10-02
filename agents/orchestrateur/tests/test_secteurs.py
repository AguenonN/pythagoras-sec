"""Secteurs banque et RH (docs/demo/secteurs.md, rapport docs/rapports/secteurs.md).

- Les textes du juriste, compilés le 02/10 (demo/scenarios/compilation/<clé>.json),
  se rejouent hors ligne (enregistrements de Mistral) jusqu'au jeu attendu ;
- le jeu de travail demo/scenarios/jeux/secteurs.json (D-011 à D-023
  actives) et son double en brouillon passent le chargeur strict ; ils
  reprennent demo.json tel quel ;
- demo/directives/demo.json n'est pas modifié (numérotation réservée à la
  scène, voir le rapport) ;
- chaque scénario des secteurs a un titre, un demandeur de l'annuaire et un
  jeu de travail qui existe."""

import copy
import glob
import json
import os
import shutil
import tempfile
import unittest

from compilateur import directive_go, service

RACINE = directive_go.RACINE
JEUX = os.path.join(RACINE, "demo", "scenarios", "jeux")
COMPILATION = os.path.join(RACINE, "demo", "scenarios", "compilation")
SCENARIOS = os.path.join(RACINE, "demo", "scenarios")

# Clé du texte → identifiants de ses directives dans le jeu de travail, et
# directives dont la condition a été rétablie à la main (défaut du
# compilateur : condition proposée par Mistral sous une clé « condition »,
# écartée sans erreur).
TEXTES = {"B1": ["D-011"], "B2": ["D-012"], "B3": ["D-013"], "B4": ["D-014", "D-015"], "B5": ["D-016"],
          "B6": ["D-017"], "R1": ["D-018"], "R2": ["D-019"], "R3": ["D-020"], "R4": ["D-021"], "R5": ["D-022"],
          "R6": ["D-023"]}
RETABLIES = {"D-012", "D-014", "D-015"}


def lire(chemin):
    with open(chemin, encoding="utf-8") as f:
        return json.load(f)


def sans_marque(d):
    """La directive sans ce que l'assemblage a changé : identifiant, statut,
    mention « à valider par le juriste » ajoutée au premier fondement."""
    d = copy.deepcopy(d)
    for k in ("id", "statut"):
        d.pop(k)
    d["fondements"][0]["texte"] = d["fondements"][0]["texte"].replace(" — à valider par le juriste", "")
    return d


class TestSecteurs(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.demo = lire(os.path.join(RACINE, "demo", "directives", "demo.json"))
        cls.actif = lire(os.path.join(JEUX, "secteurs.json"))
        cls.brouillon = lire(os.path.join(JEUX, "secteurs-brouillon.json"))

    def test_jeux_de_travail(self):
        n = len(self.demo["directives"])
        for jeu, statut in ((self.actif, "active"), (self.brouillon, "brouillon")):
            ok, err, _ = directive_go.verifier(jeu, exiger_brouillon=False)
            self.assertTrue(ok, err)
            self.assertEqual(jeu["jeu"]["id"], "demo")
            self.assertEqual(jeu["directives"][:n], self.demo["directives"])
            ajout = jeu["directives"][n:]
            self.assertEqual([d["id"] for d in ajout], ["D-%03d" % i for i in range(11, 24)])
            for d in ajout:
                self.assertEqual(d["statut"], statut)
                self.assertIn("à valider par le juriste", " ".join(f["texte"] for f in d["fondements"]))
        self.assertEqual([sans_marque(d) for d in self.actif["directives"][n:]],
                         [sans_marque(d) for d in self.brouillon["directives"][n:]])

    def test_demo_inchange(self):
        self.assertEqual([d["id"] for d in self.demo["directives"]], ["D-00%d" % i for i in range(1, 9)])

    def test_origines(self):
        o = lire(os.path.join(RACINE, "demo", "directives", "origines.json"))
        self.assertEqual(sorted(o), ["D-%03d" % i for i in range(11, 24)])
        for i, e in o.items():
            self.assertIn("à valider par le juriste", e["texte"])
            self.assertEqual("note" in e and "rétablie" in e["note"], i in RETABLIES)

    def test_compilation_rejouee(self):
        tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, tmp)
        par_id = {d["id"]: d for d in self.actif["directives"]}
        for cle, ids in TEXTES.items():
            requete, reponses = service.lire_parcours(os.path.join(COMPILATION, cle + ".json"))
            s = service.Service(mode="rejeu", dossier=tmp)
            fin = service.parcours(s, requete, reponses)[-1]
            self.assertEqual(fin["etat"], "jeu", (cle, fin["erreurs"]))
            ajout = lire(service.chemin_absolu(fin["jeu_candidat"]))["directives"][len(self.demo["directives"]):]
            self.assertEqual(len(ajout), len(ids), cle)
            for compilee, i in zip(ajout, ids):
                attendue = sans_marque(par_id[i])
                if i in RETABLIES:
                    self.assertNotIn("condition", compilee, i)
                    attendue.pop("condition")
                self.assertEqual(sans_marque(compilee), attendue, i)

    def test_scenarios_des_secteurs(self):
        annuaire = {e["identite"] for e in lire(os.path.join(RACINE, "demo", "annuaire-source.json"))["identites"]}
        vus = 0
        for chemin in sorted(glob.glob(os.path.join(SCENARIOS, "secteur-*.json"))):
            s = lire(chemin)
            vus += 1
            self.assertTrue(s.get("titre"), chemin)
            self.assertIn(s["demandeur"], annuaire, chemin)
            self.assertTrue(os.path.exists(os.path.join(RACINE, s["jeu"])), chemin)
            self.assertIn(s["attendu"]["statut"], ("termine", "bloque", "en_attente", "erreur"), chemin)
        self.assertGreaterEqual(vus, 0)


if __name__ == "__main__":
    unittest.main()
