"""Les textes de la scène, par le service (contrats §4), rejoués sur les
réponses réelles de Mistral enregistrées : « week-end » et « base interne »
(D-009), de bout en bout jusqu'au jeu candidat version n+1 du jeu de démo,
accepté par directive verifier. Aucun appel réseau : mode rejeu.

Le second groupe de tests rejoue les mesures du choix du modèle
(RAPPORT_B.md, étape 6) : mêmes textes, mêmes réponses du juriste, sur
mistral-small-latest et mistral-medium-latest.

Pour réécrire les relectures attendues : RELECTURE_MAJ=1."""

import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest

from compilateur import compilateur_v2 as c2
from compilateur import directive_go, mistral, service

from .aides import CATALOGUE_SCENE, VOCABULAIRE_SCENE

ICI = os.path.dirname(os.path.abspath(__file__))
AGENTS = os.path.dirname(ICI)
ATTENDUS = os.path.join(ICI, "attendus", "service")
PARCOURS = os.path.join(AGENTS, "compilateur", "exemples", "service")
DEMO = os.path.join(directive_go.RACINE, "demo", "directives", "demo.json")
PRET = {"evenement": "appel_outil", "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"}}


def lire_demo():
    with open(DEMO, encoding="utf-8") as f:
        return json.load(f)


class Base(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.tmp)

    def derouler(self, nom, modele=mistral.MODELE_COMPILATEUR):
        requete, reponses = service.lire_parcours(os.path.join(PARCOURS, nom + ".json"))
        s = service.Service(mode="rejeu", modele=modele, dossier=self.tmp, catalogue=CATALOGUE_SCENE,
                            vocabulaire_=VOCABULAIRE_SCENE)
        return service.parcours(s, requete, reponses)

    def candidat(self, rep):
        with open(service.chemin_absolu(rep["jeu_candidat"]), encoding="utf-8") as f:
            return json.load(f)

    def verifier_candidat(self, rep):
        """Version n+1 du jeu de démo : existantes inchangées, une ajoutée
        en brouillon, accepté par directive verifier. Rend l'ajoutée."""
        cand, demo = self.candidat(rep), lire_demo()
        self.assertEqual((cand["jeu"]["id"], cand["jeu"]["version"]), ("demo", demo["jeu"]["version"] + 1))
        n = len(demo["directives"])
        self.assertEqual(cand["directives"][:n], demo["directives"])
        ok, err, _ = directive_go.verifier(cand, exiger_brouillon=False)
        self.assertTrue(ok, err)
        d, = cand["directives"][n:]
        self.assertEqual((d["id"], d["statut"]), ("D-009", "brouillon"))
        return d

    def relecture_stable(self, nom, rep):
        chemin = os.path.join(ATTENDUS, nom + ".txt")
        if os.environ.get("RELECTURE_MAJ"):
            os.makedirs(ATTENDUS, exist_ok=True)
            with open(chemin, "w", encoding="utf-8") as f:
                f.write(rep["relecture"])
        with open(chemin, encoding="utf-8") as f:
            self.assertEqual(rep["relecture"], f.read())


class TestScene(Base):
    def test_week_end(self):
        sorties = self.derouler("weekend")
        self.assertEqual([r["etat"] for r in sorties], [c2.QUESTIONS, c2.QUESTIONS, c2.JEU])
        self.assertEqual([[q["origine"] for q in r["questions"]] for r in sorties[:2]], [["mistral"], ["code"]])
        self.assertIn("fuseau", sorties[0]["questions"][0]["texte"])
        self.assertIn("Europe/Paris", sorties[0]["questions"][0]["options"])
        fin = sorties[-1]
        self.assertEqual(fin["erreurs"], [])
        d = self.verifier_candidat(fin)
        self.assertEqual(d["texte_fr"], "Désormais, aucune exécution de workflow agentique le week-end.")
        self.assertEqual(d["condition"], {"temps": {"jour_semaine": ["samedi", "dimanche"], "fuseau": "Europe/Paris"}})
        self.assertEqual(d["effets"], [{"effet": "refuser"}])
        self.assertEqual(d["fondements"], [{"texte": "Règlement intérieur", "article": "12"}])
        self.assertEqual((d["si_indetermine"], d["date_effet"]), ("refuser", "2026-10-04T11:05:00+02:00"))
        self.relecture_stable("weekend", fin)
        self.assertNotIn("D-008", fin["relecture"])

    def test_base_interne_d009(self):
        sorties = self.derouler("base-interne")
        self.assertEqual([r["etat"] for r in sorties], [c2.QUESTIONS, c2.QUESTIONS, c2.JEU])
        self.assertEqual(len(sorties[0]["questions"]), 3)
        self.assertEqual(sorties[0]["questions"][2]["options"], ["analyste-risques"])
        d = self.verifier_candidat(sorties[-1])
        self.assertEqual(d["declencheur"], PRET)
        self.assertEqual(d["condition"], {"session.a_consulte": {"outil": "banque-synthetique/get_taux_interne",
                                                                 "critere": "donnees_recues"}})
        self.assertEqual(d["effets"], [{"effet": "attendre_validation", "par_role": "analyste-risques"}])
        self.assertEqual(d["fondements"], [{"texte": "Code monétaire et financier", "article": "L313-5-1"}])
        self.relecture_stable("base-interne", sorties[-1])

    def test_d010_apres_d009(self):
        """Le jour J, D-009 est activée avant que le week-end soit dicté : le
        même enregistrement rend D-010, version n+2 (le jeu existant n'entre
        pas dans les requêtes à Mistral)."""
        cand = self.candidat(self.derouler("base-interne")[-1])
        cand["directives"][-1]["statut"] = "active"
        jeu = os.path.join(self.tmp, "demo-v2.json")
        with open(jeu, "w", encoding="utf-8") as f:
            json.dump(cand, f, ensure_ascii=False)
        requete, reponses = service.lire_parcours(os.path.join(PARCOURS, "weekend.json"))
        s = service.Service(mode="rejeu", dossier=self.tmp, catalogue=CATALOGUE_SCENE, vocabulaire_=VOCABULAIRE_SCENE)
        fin = service.parcours(s, dict(requete, jeu_existant=jeu), reponses)[-1]
        self.assertEqual(fin["etat"], c2.JEU, fin["erreurs"])
        j = self.candidat(fin)
        self.assertEqual((j["jeu"]["version"], [d["id"] for d in j["directives"][-2:]]), (3, ["D-009", "D-010"]))
        self.assertEqual(j["directives"][:-1], cand["directives"])
        self.assertTrue(fin["relecture"].startswith("Ajout au jeu demo, version 3 : 1 directive ajoutée (D-010), "
                                                    "évaluée après 9 directives existantes"))
        ok, err, _ = directive_go.verifier(j, exiger_brouillon=False)
        self.assertTrue(ok, err)

    def test_variantes_de_reponse_enregistrees(self):
        """Une réponse écrite en toutes lettres plutôt que l'option cliquée
        (« Europe/Paris, l'heure de Paris. ») est aussi enregistrée, et rend
        le même jeu."""
        for nom in ("weekend", "base-interne"):
            chemin = os.path.join(PARCOURS, nom + ".json")
            with open(chemin, encoding="utf-8") as f:
                variantes = json.load(f)["variantes"]
            requete, reponses = service.lire_parcours(chemin)
            attendu = self.candidat(self.derouler(nom)[-1])["directives"]
            for question, autres in variantes.items():
                for autre in autres:
                    s = service.Service(mode="rejeu", dossier=self.tmp, catalogue=CATALOGUE_SCENE, vocabulaire_=VOCABULAIRE_SCENE)
                    fin = service.parcours(s, requete, dict(reponses, **{question: autre}))[-1]
                    self.assertEqual(fin["etat"], c2.JEU, (nom, autre, fin["erreurs"]))
                    self.assertEqual(self.candidat(fin)["directives"], attendu)

    def test_rejeu_stable(self):
        a = self.derouler("weekend")[-1]
        b = self.derouler("weekend")[-1]
        self.assertNotEqual(a["conversation"], b["conversation"])
        self.assertEqual(a["relecture"], b["relecture"])
        self.assertEqual(self.candidat(a), self.candidat(b))


class TestCommeLaConsole(Base):
    """Le parcours week-end par la ligne de commande, une requête JSON par
    appel, comme la console l'appellera ; le rappel ne refait aucun appel."""

    def appeler(self, requete):
        r = subprocess.run([sys.executable, "-m", "compilateur.service", "--dossier", self.tmp,
                            "--catalogue", CATALOGUE_SCENE, "--vocabulaire", VOCABULAIRE_SCENE],
                           input=json.dumps(requete), capture_output=True, text=True, cwd=AGENTS,
                           env=dict(os.environ, COMPILATEUR_MODE="rejeu"))
        self.assertEqual(r.returncode, 0, r.stderr)
        return json.loads(r.stdout)

    def test_week_end_par_la_ligne_de_commande(self):
        requete, reponses = service.lire_parcours(os.path.join(PARCOURS, "weekend.json"))
        rep = self.appeler(dict(requete, conversation=None, reponses=[]))
        conv = rep["conversation"]
        while rep["etat"] == c2.QUESTIONS:
            envoi = [{"question": q["id"], "reponse": reponses[q["texte"] if q["origine"] == "mistral" else q["id"]]}
                     for q in rep["questions"]]
            rep = self.appeler({"conversation": conv, "reponses": envoi})
        self.assertEqual(rep["etat"], c2.JEU, rep["erreurs"])
        self.assertEqual(rep["conversation"], conv)
        self.verifier_candidat(rep)
        with open(os.path.join(self.tmp, conv, "conversation.json"), encoding="utf-8") as f:
            tours = json.load(f)["tours"]
        self.assertEqual([t["appels_nouveaux"] for t in tours], [1, 1, 0])
        # directive verifier, le binaire que la console appellera.
        r = subprocess.run([directive_go.binaire(), "verifier"], input=json.dumps(self.candidat(rep)),
                           capture_output=True, text=True)
        self.assertEqual(r.returncode, 0, r.stdout)


class TestChoixDuModele(Base):
    """Les mesures de l'étape 6, rejouées. small : condition du week-end
    inversée (refus en semaine), nœuds inventés sur la base interne ;
    medium : les deux jeux justes du premier coup."""

    def test_small_week_end_inverse(self):
        sorties = self.derouler("weekend", modele="mistral-small-latest")
        self.assertEqual(sorties[-1]["etat"], c2.JEU)
        d = self.candidat(sorties[-1])["directives"][-1]
        self.assertEqual(d["condition"], {"non": {"temps": {"jour_semaine": ["samedi", "dimanche"],
                                                            "fuseau": "Europe/Paris"}}})
        self.assertIn("Si la condition suivante est fausse", sorties[-1]["relecture"])

    def test_small_base_interne_refusee(self):
        sorties = self.derouler("base-interne", modele="mistral-small-latest")
        self.assertEqual([r["etat"] for r in sorties], [c2.QUESTIONS, c2.REFUSE])
        self.assertTrue(any("groupe 'analystes-credit' n'est ni écrit ni choisi" in e for e in sorties[-1]["erreurs"]))
        self.assertIn("durée d'un trimestre", sorties[0]["questions"][0]["texte"])

    def test_medium_du_premier_coup(self):
        for nom in ("weekend", "base-interne"):
            self.derouler(nom)
            for conv in os.listdir(self.tmp):
                with open(os.path.join(self.tmp, conv, "conversation.json"), encoding="utf-8") as f:
                    tours = json.load(f)["tours"]
                self.assertEqual(sum(t["appels_nouveaux"] for t in tours), 2, (nom, tours))
            shutil.rmtree(self.tmp)
            os.makedirs(self.tmp)


if __name__ == "__main__":
    unittest.main()
