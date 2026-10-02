"""Secteurs banque et RH (docs/demo/secteurs.md, rapport docs/rapports/secteurs.md).

- Les textes du juriste, compilés le 02/10 (demo/scenarios/compilation/<clé>.json),
  se rejouent hors ligne (enregistrements de Mistral) jusqu'au jeu attendu ;
- le jeu de travail demo/scenarios/jeux/secteurs.json (D-011 à D-023
  actives) et son double en brouillon passent le chargeur strict ; ils
  reprennent demo.json tel quel ;
- demo/directives/demo.json v2 est ce jeu en brouillon (v1 archivée dans
  demo/directives/historique/) ;
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
# Scénarios écrits mais pas passés en réel (budget Mistral épuisé).
NON_ENREGISTRES = set()


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
        cls.demo = lire(os.path.join(RACINE, "demo", "directives", "historique", "demo.v1.json"))
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

    def test_demo_installe(self):
        """demo.json v2 = le jeu en brouillon (décision de Manu, 02/10)."""
        self.assertEqual(lire(os.path.join(RACINE, "demo", "directives", "demo.json")), self.brouillon)

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
            # Le parcours vise demo/directives/demo.json, quelle qu'en soit
            # la version : les ajoutées sont les dernières.
            courant = lire(os.path.join(RACINE, "demo", "directives", "demo.json"))
            tout = lire(service.chemin_absolu(fin["jeu_candidat"]))["directives"]
            self.assertEqual(len(tout), len(courant["directives"]) + len(ids), cle)
            ajout = tout[len(courant["directives"]):]
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
        self.assertEqual(vus, 27)

    def test_chaque_directive_a_son_piege_enregistre(self):
        """Pour chaque directive D-011 à D-023, un scénario piège enregistré en
        réel où l'agent a tenté l'action : l'enregistrement finit comme
        attendu (refus ou attente : la directive est citée ; caviardage :
        terminé)."""
        for i in range(11, 24):
            nom = "secteur-d%03d-piege" % i
            s = lire(os.path.join(SCENARIOS, nom + ".json"))
            enr = sorted(glob.glob(os.path.join(SCENARIOS, "enregistrements", nom, "*.json")))
            self.assertTrue(enr, nom)
            r = lire(enr[-1])
            self.assertEqual(r["demande"], s["demande"], nom)
            self.assertEqual(r["resultat"]["statut"], s["attendu"]["statut"], nom)
            self.assertEqual(r["resultat"]["directives_citees"], s["attendu"]["directives"], nom)
            self.assertEqual(r["options"]["jeu"], "demo/scenarios/jeux/secteurs.json", nom)

    def test_validations_enregistrees(self):
        """Les attentes rejetées ne laissent aucune validation enregistrée (le
        rejeu se termine aussitôt) ; les acceptées sont celles du scénario."""
        for chemin in sorted(glob.glob(os.path.join(SCENARIOS, "secteur-*.json"))):
            s = lire(chemin)
            nom = os.path.basename(chemin)[:-5]
            if nom in NON_ENREGISTRES:
                self.assertFalse(glob.glob(os.path.join(SCENARIOS, "enregistrements", nom, "*.json")), nom)
                continue
            r = lire(sorted(glob.glob(os.path.join(SCENARIOS, "enregistrements", nom, "*.json")))[-1])
            acceptees = [v["valideurs"] for v in s["validations"] if v["attendu"] == "acceptee"]
            self.assertEqual([v["valideurs"] for v in r["validations"]], acceptees, nom)


if __name__ == "__main__":
    unittest.main()


class TestNumerotationScene(unittest.TestCase):
    """Sur un jeu D-001 à D-008 + D-011 à D-023, les textes de la scène
    reçoivent D-009 (« base interne ») puis D-010 (« week-end »), parcours du
    service rejoués (enregistrements de la scène, catalogue gelé)."""

    def test_d009_puis_d010(self):
        from tests.aides import CATALOGUE_SCENE, VOCABULAIRE_SCENE
        tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, tmp)
        exemples = os.path.join(RACINE, "agents", "compilateur", "exemples", "service")

        def derouler(nom, jeu):
            requete, reponses = service.lire_parcours(os.path.join(exemples, nom + ".json"))
            s = service.Service(mode="rejeu", dossier=tmp, catalogue=CATALOGUE_SCENE, vocabulaire_=VOCABULAIRE_SCENE)
            fin = service.parcours(s, dict(requete, jeu_existant=jeu), reponses)[-1]
            self.assertEqual(fin["etat"], "jeu", fin["erreurs"])
            return lire(service.chemin_absolu(fin["jeu_candidat"]))

        cand = derouler("base-interne", os.path.join(JEUX, "secteurs-brouillon.json"))
        self.assertEqual(cand["directives"][-1]["id"], "D-009")
        cand["directives"][-1]["statut"] = "active"
        chemin = os.path.join(tmp, "avec-d009.json")
        with open(chemin, "w", encoding="utf-8") as f:
            json.dump(cand, f, ensure_ascii=False)
        self.assertEqual(derouler("weekend", chemin)["directives"][-1]["id"], "D-010")
