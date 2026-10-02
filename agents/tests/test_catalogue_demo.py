"""Le catalogue de démonstration est généré depuis le registre de démo
(go run ./cmd/registre-demo -catalogue compilateur/exemples/catalogue-demo.json,
depuis la racine du dépôt). Ce test échoue s'il diverge du registre : quand
demo/registre.json a été généré sur la machine, il en redérive le catalogue ;
sinon, il le redérive des schema.json versionnés des serveurs, que le
registre recopie tels quels. Côté Go, internal/registredemo fait la même
vérification."""

import json
import os
import unittest

from .aides import CATALOGUE_DEMO, catalogue_demo

RACINE = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
REGISTRE = os.path.join(RACINE, "demo", "registre.json")
SERVEURS = ["taux-officiel", "recherche-entreprises", "gels-avoirs", "eurlex",
            "legifrance", "banque-synthetique", "rh-synthetique", "referentiel-rh"]


def pointeurs(schema, prefixe=""):
    out = []
    for nom, sous in (schema.get("properties") or {}).items():
        p = prefixe + "/" + nom.replace("~", "~0").replace("/", "~1")
        out.append(p)
        if isinstance(sous, dict):
            out += pointeurs(sous, p)
    return out


def depuis_outils(serveurs):
    """serveurs : [(nom, [(nom, acces, description, input_schema)])]"""
    return [{"nom": s, "outils": [{"nom": o, "acces": a, "description": d, "arguments": sorted(pointeurs(sch))}
                                  for o, a, d, sch in outils]} for s, outils in serveurs]


def depuis_registre(reg):
    return depuis_outils([(s["nom"], [(o["nom"], o["acces"], o["description"], o["input_schema"])
                                      for o in s["outils"]]) for s in reg["serveurs"]])


def depuis_schemas():
    serveurs = []
    for s in SERVEURS:
        with open(os.path.join(RACINE, "servers", s, "schema.json"), encoding="utf-8") as f:
            liste = json.load(f)
        serveurs.append((s, [(t["name"], "read" if (t.get("annotations") or {}).get("readOnlyHint") is True else "action",
                              t.get("description", ""), t["inputSchema"]) for t in liste]))
    return depuis_outils(serveurs)


class TestCatalogueDemo(unittest.TestCase):
    def setUp(self):
        with open(CATALOGUE_DEMO, encoding="utf-8") as f:
            self.versionne = json.load(f)

    def test_conforme_aux_schemas(self):
        self.assertEqual(self.versionne["serveurs"], depuis_schemas())

    def test_conforme_au_registre(self):
        if not os.path.exists(REGISTRE):
            self.skipTest("demo/registre.json absent (go run ./cmd/registre-demo)")
        with open(REGISTRE, encoding="utf-8") as f:
            self.assertEqual(self.versionne["serveurs"], depuis_registre(json.load(f)))

    def test_divergence_detectee(self):
        altere = json.loads(json.dumps(self.versionne))
        altere["serveurs"][0]["outils"][0]["arguments"].append("/invente")
        self.assertNotEqual(altere["serveurs"], depuis_schemas())

    def test_lisible_par_le_compilateur(self):
        c = catalogue_demo()
        self.assertEqual(c.noms_serveurs(), sorted(SERVEURS))
        self.assertTrue(c.cible_existe("banque-synthetique", "attribuer_pret"))
        self.assertIn("/montant", c.arguments_de("banque-synthetique", "attribuer_pret"))
        self.assertIn("/siren", c.arguments_de("recherche-entreprises", "*"))
        self.assertFalse(c.cible_existe("taux-imposteur", "*"))
        # Le catalogue entre tel quel dans le message montré au modèle.
        self.assertIn("get_taux_usure", c.pour_le_modele())


if __name__ == "__main__":
    unittest.main()
