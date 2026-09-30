import unittest

from boucle import schemas

from .faux_serveur_mcp import OUTILS


def outil(schema, nom="eurlex__t"):
    return {"name": nom, "description": "d", "inputSchema": schema}


class TestConversion(unittest.TestCase):
    def test_noyau_conserve(self):
        s = {"type": "object", "properties": {"a": {"type": "string", "enum": ["x"], "description": "d"},
                                              "b": {"type": "array", "items": {"type": "number"}}},
             "required": ["a"]}
        d, sig = schemas.convertir(outil(s))
        self.assertEqual(d["function"]["parameters"], s)
        self.assertEqual(sig, [])
        self.assertEqual(d["type"], "function")

    def test_mots_cles_retires_et_signales(self):
        for cle, valeur in [("format", "date"), ("pattern", "^a$"), ("minimum", 1), ("maximum", 2),
                            ("default", 3), ("title", "t"), ("additionalProperties", False),
                            ("minLength", 1), ("maxItems", 3), ("examples", ["x"]), ("$schema", "u")]:
            s = {"type": "object", "properties": {"a": {"type": "string", cle: valeur}}}
            d, sig = schemas.convertir(outil(s))
            self.assertNotIn(cle, d["function"]["parameters"]["properties"]["a"], cle)
            self.assertEqual(len(sig), 1, cle)
            self.assertIn(repr(cle), sig[0])
            self.assertIn("eurlex__t.properties.a." + cle, sig[0])

    def test_structurants_excluent_l_outil(self):
        for cle, valeur in [("oneOf", [{"type": "string"}]), ("anyOf", [{"type": "string"}]),
                            ("allOf", [{"type": "string"}]), ("not", {"type": "string"}),
                            ("$ref", "#/$defs/x"), ("$defs", {}), ("if", {}), ("patternProperties", {}),
                            ("prefixItems", [])]:
            s = {"type": "object", "properties": {"a": {cle: valeur}}}
            with self.assertRaises(schemas.Exclu, msg=cle) as ctx:
                schemas.convertir(outil(s))
            self.assertIn(cle, str(ctx.exception))

    def test_inconnu_exclut(self):
        with self.assertRaises(schemas.Exclu):
            schemas.convertir(outil({"type": "object", "x-maison": 1}))

    def test_const_et_null(self):
        s = {"type": "object", "properties": {"l": {"const": "fr"}, "d": {"type": ["string", "null"]}}}
        d, sig = schemas.convertir(outil(s))
        p = d["function"]["parameters"]["properties"]
        self.assertEqual(p["l"], {"enum": ["fr"]})
        self.assertEqual(p["d"], {"type": "string"})
        self.assertEqual(len(sig), 2)
        with self.assertRaises(schemas.Exclu):
            schemas.convertir(outil({"type": "object", "properties": {"x": {"type": ["string", "number"]}}}))

    def test_items_tuple_exclu(self):
        with self.assertRaises(schemas.Exclu):
            schemas.convertir(outil({"type": "object", "properties": {"x": {"type": "array", "items": [{}]}}}))

    def test_nom_et_type(self):
        with self.assertRaises(schemas.Exclu):
            schemas.convertir(outil({"type": "object"}, nom="a.b"))
        with self.assertRaises(schemas.Exclu):
            schemas.convertir(outil({"type": "object"}, nom="x" * 65))
        with self.assertRaises(schemas.Exclu):
            schemas.convertir(outil({"type": "string"}))
        d, _ = schemas.convertir({"name": "eurlex__t"})
        self.assertEqual(d["function"]["parameters"], {"type": "object", "properties": {}})

    def test_faux_serveur(self):
        defs, sig, exclus = schemas.convertir_tous(OUTILS)
        noms = [d["function"]["name"] for d in defs]
        self.assertNotIn("legifrance__citer_article", noms)
        self.assertEqual(len(exclus), 1)
        self.assertIn("oneOf", exclus[0])
        self.assertIn("eurlex__chercher_texte", noms)
        eurlex = next(d for d in defs if d["function"]["name"] == "eurlex__chercher_texte")
        self.assertEqual(eurlex["function"]["parameters"]["properties"]["langue"], {"enum": ["fr"]})
        self.assertEqual(sorted(sig), sorted([
            "eurlex__chercher_texte.$schema : mot-clé '$schema' non accepté, retiré",
            "eurlex__chercher_texte.additionalProperties : mot-clé 'additionalProperties' non accepté, retiré",
            "eurlex__chercher_texte.properties.celex.pattern : mot-clé 'pattern' non accepté, retiré",
            "eurlex__chercher_texte.properties.depuis.format : mot-clé 'format' non accepté, retiré",
            "eurlex__chercher_texte.properties.depuis.type : type ['string', 'null'] réduit à 'string' (null retiré)",
            "eurlex__chercher_texte.properties.langue.const : const réécrit en enum à une valeur",
            "eurlex__chercher_texte.properties.limite.default : mot-clé 'default' non accepté, retiré",
            "eurlex__chercher_texte.properties.limite.maximum : mot-clé 'maximum' non accepté, retiré",
            "eurlex__chercher_texte.properties.limite.minimum : mot-clé 'minimum' non accepté, retiré",
        ]))


if __name__ == "__main__":
    unittest.main()
