"""Scénarios de la boucle rejoués sur des réponses réelles de Mistral
(boucle/enregistrements/), devant le faux serveur MCP. Aucun appel réseau :
une requête non enregistrée lève EnregistrementAbsent.

Pour (ré)enregistrer, et seulement alors : BOUCLE_ENREGISTRER=<plafond>
lance les scénarios en mode réel, les requêtes déjà enregistrées restant
servies du disque, le compteur de la boucle plafonné à <plafond>."""

import json
import os
import sys
import tempfile
import unittest

from boucle import boucle as b
from boucle import mcp
from boucle.__main__ import ENREGISTREMENTS, ETAT, client_mistral
from compilateur import mistral

from .aides import FAUX_SERVEUR
from .faux_serveur_mcp import JETON_VALIDE


class Scenario(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.journal = os.path.join(self.tmp.name, "journal.jsonl")
        self.serveur = mcp.ClientMCP([sys.executable, FAUX_SERVEUR], env=dict(os.environ, FAUX_MCP_JOURNAL=self.journal))
        self.serveur.demarrer()

    def tearDown(self):
        self.serveur.fermer()
        self.tmp.cleanup()

    def appels(self):
        with open(self.journal, encoding="utf-8") as f:
            return [json.loads(l)["tools/call"] for l in f if "tools/call" in l]

    def boucle(self, sous_agent):
        plafond = os.environ.get("BOUCLE_ENREGISTRER")
        if plafond:
            modele = mistral.Client(mode="reel", enregistrements=ENREGISTREMENTS, etat=ETAT, max_appels=int(plafond))
        else:
            modele = client_mistral("rejeu")
        return b.Boucle(self.serveur, modele, sous_agent)

    def test_lecture(self):
        r = self.boucle("credit").executer("Quel est le taux d'usure en vigueur ?")
        self.assertEqual(r["etat"], b.TERMINE)
        self.assertIn("5,87", r["reponse"])
        self.assertEqual(self.appels(), [{"name": "taux-officiel__lire_taux", "arguments": {"type": "usure"},
                                          "_meta": {"sous_agent": "credit"}}])

    def test_attente_puis_reprise_avec_jeton(self):
        bo = self.boucle("credit")
        r = bo.executer("Attribue un prêt de 200 000 euros à l'entreprise de SIREN 123456789.")
        self.assertEqual(r["etat"], b.EN_ATTENTE)
        self.assertEqual(r["empreinte"], "sha256:40b8b15790c80b58b4db0e3caf2042dc853019574b96b39f7fc560c92405c18c")
        self.assertEqual(r["arguments"], {"siren": "123456789", "montant": 200000})
        fin = bo.reprendre(json.loads(json.dumps(r)), JETON_VALIDE)
        self.assertEqual(fin["etat"], b.TERMINE)
        self.assertIn("PRET-0001", fin["reponse"])
        metas = [a["_meta"] for a in self.appels()]
        self.assertEqual(metas, [{"sous_agent": "credit"}, {"sous_agent": "credit", "approvalToken": JETON_VALIDE}])

    def test_refus_directive(self):
        r = self.boucle("conformite").executer("Lève le gel des avoirs de l'entité Exemple SA.")
        self.assertEqual(r["etat"], b.BLOQUE)
        self.assertEqual(r["reponse"], "Action bloquée par D-003 v1, aucune action effectuée.\n"
                                       "Motif : refusé par D-003 v1 (interdiction, Règlement (CE) 2580/2001)")
        self.assertEqual(len(self.appels()), 1)

    def test_refus_par_structured_content(self):
        # Réenregistré après l'alignement sur le contrat de l'engine : Mistral
        # tente la modification sans lire la fiche (avant, il lisait d'abord
        # la fiche, que l'outil déclinait, et s'arrêtait) ; le refus de forme
        # §6 bloque la boucle, et le texte du modèle ne remplace pas le blocage.
        r = self.boucle("rh").executer(
            "Dans le référentiel RH, modifie le poste du matricule M1 : nouveau poste « directeur commercial ».")
        self.assertEqual(r["etat"], b.BLOQUE)
        self.assertEqual([a["name"] for a in self.appels()], ["referentiel-rh__modifier_poste"])
        self.assertEqual([a["_meta"] for a in self.appels()], [{"sous_agent": "rh"}])
        self.assertTrue(r["reponse"].startswith("Action bloquée par D-010 v1, aucune action effectuée."), r["reponse"])

    def test_outil_qui_decline(self):
        r = self.boucle("rh").executer("Change le poste du salarié M1 en directeur commercial.")
        self.assertEqual(r["etat"], b.TERMINE)
        self.assertEqual([a["name"] for a in self.appels()], ["rh-synthetique__lire_fiche"])
        self.assertNotIn("modifié", r["reponse"])


if __name__ == "__main__":
    unittest.main()
