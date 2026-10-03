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
from tests.aides import CATALOGUE_SECTEURS, VOCABULAIRE_SECTEURS

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
        demo = lire(os.path.join(RACINE, "demo", "directives", "demo.json"))
        self.assertEqual(demo["directives"][:21], self.brouillon["directives"])
        self.assertEqual(lire(os.path.join(RACINE, "demo", "directives", "historique", "demo.v2.json")), self.brouillon)
        # D-024 à D-029 (A4) : celles de secteurs-ppe.json, en brouillon.
        ppe = lire(os.path.join(JEUX, "secteurs-ppe.json"))["directives"][21:27]
        self.assertEqual([dict(d, statut="active") for d in demo["directives"][21:27]], ppe)

    def test_origines(self):
        o = lire(os.path.join(RACINE, "demo", "directives", "origines.json"))
        # D-011 à D-031 (secteurs), D-040 à D-046 (opération ATLAS, ajoutées
        # en brouillon par la session gel).
        self.assertEqual(sorted(o), ["D-%03d" % i for i in list(range(11, 32)) + list(range(40, 47))])
        for i, e in o.items():
            self.assertIn("à valider par le", e["texte"])
            self.assertEqual("note" in e and "rétablie" in e["note"], i in RETABLIES)

    def test_compilation_rejouee(self):
        tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, tmp)
        par_id = {d["id"]: d for d in self.actif["directives"]}
        for cle, ids in TEXTES.items():
            requete, reponses = service.lire_parcours(os.path.join(COMPILATION, cle + ".json"))
            s = service.Service(mode="rejeu", dossier=tmp, catalogue=CATALOGUE_SECTEURS,
                                vocabulaire_=VOCABULAIRE_SECTEURS)
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
        self.assertEqual(vus, 35)  # 13 pièges, 13 propres, 3 PPE (A4), 3 rémunération (A5), D-023 fermée, propres D-030 et D-031

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


def normaliser(v):
    """L'ordre des champs autorisés d'un caviardage n'a pas de sens : le
    compilateur peut les rendre dans un autre ordre d'un enregistrement à
    l'autre."""
    if isinstance(v, dict):
        return {k: (sorted(x) if k == "champs_autorises" else normaliser(x)) for k, x in v.items()}
    if isinstance(v, list):
        return [normaliser(x) for x in v]
    return v


class TestPPE(unittest.TestCase):
    """A4 : le texte du juriste se rejoue jusqu'aux six directives, une par
    outil bancaire, refus + notification de la conformité, sujet résolu au
    référentiel ; installées D-024 à D-029."""

    def test_compilation_a4_rejouee(self):
        tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, tmp)
        requete, reponses = service.lire_parcours(os.path.join(COMPILATION, "A4.json"))
        # Rejoué sur le jeu de la compilation (demo.json v2, archivé) : sur un
        # jeu qui contient déjà D-024 à D-029, le compilateur fait une
        # requête de plus à Mistral, qui n'est pas enregistrée.
        requete = dict(requete, jeu_existant="demo/directives/historique/demo.v2.json")
        fin = service.parcours(service.Service(mode="rejeu", dossier=tmp), requete, reponses)[-1]
        self.assertEqual(fin["etat"], "jeu", fin["erreurs"])
        compilees = lire(service.chemin_absolu(fin["jeu_candidat"]))["directives"][-6:]
        installees = lire(os.path.join(JEUX, "secteurs-ppe.json"))["directives"][21:27]
        self.assertEqual([sans_marque(d) for d in compilees], [sans_marque(d) for d in installees])
        for d in installees:
            self.assertEqual(d["condition"]["sujet"]["groupe"], "personne-politiquement-exposee")
            self.assertEqual([e["effet"] for e in d["effets"]], ["refuser", "notifier"])


class TestRemuneration(unittest.TestCase):
    """A5 : le texte brut du juriste se rejoue jusqu'à ses questions (pas de
    jeu) ; les textes a et b se rejouent jusqu'aux directives installées
    D-030 et D-031 (sur le jeu de la compilation, demo.json v3)."""

    def derouler(self, cle):
        tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, tmp)
        requete, reponses = service.lire_parcours(os.path.join(COMPILATION, cle + ".json"))
        requete = dict(requete, jeu_existant="demo/directives/historique/demo.v3.json")
        return service.parcours(service.Service(mode="rejeu", dossier=tmp), requete, reponses)

    def test_texte_brut_questions(self):
        sorties = self.derouler("A5-brut")
        self.assertEqual([s["etat"] for s in sorties], ["questions"])
        self.assertTrue(any("inclure_remuneration" in q["texte"] for q in sorties[0]["questions"]))

    def test_a_et_b(self):
        jeu = lire(os.path.join(JEUX, "secteurs-a5ab.json"))["directives"]
        installees = {d["id"]: d for d in jeu}
        for cle, ident in (("A5a", "D-030"), ("A5b", "D-031")):
            fin = self.derouler(cle)[-1]
            self.assertEqual(fin["etat"], "jeu", fin["erreurs"])
            compilee = lire(service.chemin_absolu(fin["jeu_candidat"]))["directives"][-1]
            attendue = installees[ident]
            for k in ("declencheur", "condition", "effets"):
                self.assertEqual(normaliser(compilee[k]), normaliser(attendue[k]), (cle, k))
