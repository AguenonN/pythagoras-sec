"""Les seuls noms de serveurs autorisés. Tout nom de serveur présent
dans le catalogue, dans les énumérations offertes à Mistral, dans ses
propositions enregistrées ou dans les exemples appartient à cette liste.
Les tests négatifs emploient gels-avoirs, autorisé mais absent du catalogue
d'exemple, pour vérifier qu'un serveur hors catalogue est refusé."""

import glob
import json
import os
import unittest

from .aides import EXEMPLES, catalogue_exemple

SERVEURS = {"taux-officiel", "banque-synthetique", "recherche-entreprises",
            "gels-avoirs", "eurlex", "legifrance", "rh-synthetique", "referentiel-rh"}
ENREGISTREMENTS = os.path.join(os.path.dirname(EXEMPLES), "enregistrements")


def serveurs_d_une_proposition(p):
    noms = set()
    cible = p.get("cible") or {}
    if cible.get("serveur") not in (None, "*"):
        noms.add(cible["serveur"])
    pr = p.get("provenance") or {}
    noms.update(pr.get("serveurs") or [])
    noms.update(o.split("/", 1)[0] for o in pr.get("outils") or [])
    return noms


class TestNomsServeurs(unittest.TestCase):
    def test_catalogue(self):
        self.assertLessEqual(set(catalogue_exemple().noms_serveurs()), SERVEURS)

    def test_enregistrements(self):
        fichiers = sorted(glob.glob(os.path.join(ENREGISTREMENTS, "*.json")))
        self.assertTrue(fichiers)
        for chemin in fichiers:
            with open(chemin, encoding="utf-8") as f:
                e = json.load(f)
            for outil in e["requete"]["tools"]:
                props = outil["function"]["parameters"]["properties"]
                if "cible" in props:
                    enum = set(props["cible"]["properties"]["serveur"]["enum"]) - {"*"}
                    self.assertLessEqual(enum, SERVEURS, chemin)
                    self.assertLessEqual(set(props["provenance"]["properties"]["serveurs"]["items"]["enum"]), SERVEURS, chemin)
            for tc in e["reponse"]["choices"][0]["message"].get("tool_calls") or []:
                if tc["function"]["name"] == "proposer_directive":
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
