import json
import os
import sys
import tempfile
import unittest

from boucle import mcp

from .aides import FAUX_SERVEUR
from .faux_serveur_mcp import ENTITE_JSON_RPC


class TestClientMCP(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.journal = os.path.join(self.tmp.name, "journal.jsonl")
        env = dict(os.environ, FAUX_MCP_JOURNAL=self.journal)
        self.client = mcp.ClientMCP([sys.executable, FAUX_SERVEUR], env=env)
        self.client.demarrer()

    def tearDown(self):
        self.client.fermer()
        self.tmp.cleanup()

    def entrees(self):
        with open(self.journal, encoding="utf-8") as f:
            return [json.loads(l) for l in f]

    def test_liste_paginee(self):
        noms = [o["name"] for o in self.client.lister_outils()]
        self.assertEqual(len(noms), 7)
        self.assertTrue(all("__" in n for n in noms))

    def test_requete_du_serveur_refusee(self):
        self.client.lister_outils()
        self.client.appeler("taux-officiel__lire_taux", {"type": "usure"})
        reponses = [e["reponse_hote"] for e in self.entrees() if "reponse_hote" in e]
        self.assertEqual(len(reponses), 1)
        self.assertEqual(reponses[0]["id"], "s-1")
        self.assertEqual(reponses[0]["error"]["code"], -32601)

    def test_appel_et_meta(self):
        r = self.client.appeler("taux-officiel__lire_taux", {"type": "usure"}, {"sous_agent": "credit"})
        self.assertEqual(r["structuredContent"]["taux"], 5.87)
        appel = self.entrees()[-1]["tools/call"]
        self.assertEqual(appel["_meta"], {"sous_agent": "credit"})

    def test_refus_json_rpc(self):
        with self.assertRaises(mcp.ErreurMCP) as ctx:
            self.client.appeler("gels-avoirs__lever_gel", {"entite": ENTITE_JSON_RPC})
        self.assertEqual(ctx.exception.code, mcp.CODE_REFUS)
        self.assertEqual(ctx.exception.data, {"directive": "D-003 v1"})


if __name__ == "__main__":
    unittest.main()
