"""Les seuls noms de serveurs autorisés. Tout nom de serveur présent
dans le catalogue, dans les énumérations offertes à Mistral, dans ses
propositions enregistrées ou dans les exemples appartient à cette liste.
Les enregistrements sont ceux du format v2 (proposer_jeu) ; le format v1
est retiré."""

import glob
import json
import os
import unittest

from .aides import EXEMPLES, catalogue_demo

SERVEURS = {"taux-officiel", "banque-synthetique", "recherche-entreprises",
            "gels-avoirs", "eurlex", "legifrance", "rh-synthetique", "referentiel-rh",
            "data-room", "messagerie", "bourse", "communication"}
ENREGISTREMENTS = os.path.join(os.path.dirname(EXEMPLES), "enregistrements")


def serveurs_d_une_proposition(p):
    """Les serveurs que nomme une proposition de proposer_jeu."""
    noms = set()
    for d in p.get("directives") or []:
        sv = (d.get("declencheur") or {}).get("serveur")
        if sv not in (None, "", "*"):
            noms.add(sv)
        for n in d.get("noeuds") or []:
            if n.get("serveur"):
                noms.add(n["serveur"])
            if n.get("outil"):
                noms.add(n["outil"].split("/", 1)[0])
    return noms


class TestNomsServeurs(unittest.TestCase):
    def test_catalogue(self):
        self.assertLessEqual(set(catalogue_demo().noms_serveurs()), SERVEURS)

    def test_enregistrements(self):
        fichiers = sorted(glob.glob(os.path.join(ENREGISTREMENTS, "*.json")))
        self.assertTrue(fichiers)
        for chemin in fichiers:
            with open(chemin, encoding="utf-8") as f:
                e = json.load(f)
            for outil in e["requete"]["tools"]:
                if outil["function"]["name"] != "proposer_jeu":
                    continue
                d = outil["function"]["parameters"]["properties"]["directives"]["items"]["properties"]
                enum = set(d["declencheur"]["properties"]["serveur"]["enum"]) - {"*"}
                self.assertLessEqual(enum, SERVEURS, chemin)
                self.assertLessEqual(set(d["noeuds"]["items"]["properties"]["serveur"]["enum"]), SERVEURS, chemin)
            for tc in e["reponse"]["choices"][0]["message"].get("tool_calls") or []:
                if tc["function"]["name"] == "proposer_jeu":
                    p = json.loads(tc["function"]["arguments"])
                    self.assertLessEqual(serveurs_d_une_proposition(p), SERVEURS, chemin)

    def test_exemples(self):
        for chemin in glob.glob(os.path.join(EXEMPLES, "*.json")):
            with open(chemin, encoding="utf-8") as f:
                donnees = json.load(f)
            if isinstance(donnees, dict) and "serveurs" in donnees:
                self.assertLessEqual({s["nom"] for s in donnees["serveurs"]}, SERVEURS, chemin)


if __name__ == "__main__":
    unittest.main()


class TestNomsFauxServeur(unittest.TestCase):
    def test_outils_serveur__outil(self):
        from .faux_serveur_mcp import OUTILS
        for o in OUTILS:
            serveur, sep, outil = o["name"].partition("__")
            self.assertTrue(sep and outil, o["name"])
            self.assertIn(serveur, SERVEURS, o["name"])

    def test_enregistrements_boucle(self):
        dossier = os.path.join(os.path.dirname(os.path.dirname(EXEMPLES)), "boucle", "enregistrements")
        fichiers = sorted(glob.glob(os.path.join(dossier, "*.json")))
        self.assertTrue(fichiers)
        for chemin in fichiers:
            with open(chemin, encoding="utf-8") as f:
                e = json.load(f)
            noms = [o["function"]["name"] for o in e["requete"]["tools"]]
            noms += [tc["function"]["name"] for tc in e["reponse"]["choices"][0]["message"].get("tool_calls") or []]
            for n in noms:
                self.assertIn(n.partition("__")[0], SERVEURS, chemin)
