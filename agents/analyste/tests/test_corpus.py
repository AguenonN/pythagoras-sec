"""Corpus des fondements : versionné, empreinte vérifiée, sources tracées,
rien d'écrit de mémoire."""

import json
import os
import shutil
import tempfile
import unittest

from analyste import corpus


class TestCorpus(unittest.TestCase):
    def test_charge_et_empreinte(self):
        entrees, index = corpus.charger()
        self.assertEqual([e["id"] for e in entrees], ["CMF-L313-5-1", "CONSO-L314-6", "RGPD-5", "RGPD-22", "RGPD-32"])
        self.assertTrue(index["empreinte"].startswith("sha256:"))
        for e in entrees:
            self.assertTrue(e["texte"])
            self.assertTrue(e["lien"].startswith("https://"))
            self.assertIn(e["source"]["serveur"], ("legifrance", "eurlex"))
            self.assertTrue(e["source"]["sha256"])

    def test_indisponibles_consignes(self):
        _, index = corpus.charger()
        self.assertEqual(sorted(i["id"] for i in index["indisponibles"]), ["CMF-L562-4", "CT-L1121-1", "CT-L2411-1"])

    def test_retouche_refusee(self):
        d = tempfile.mkdtemp()
        try:
            shutil.copytree(corpus.DOSSIER, d, dirs_exist_ok=True)
            p = os.path.join(d, "RGPD-22.json")
            with open(p, encoding="utf-8") as f:
                e = json.load(f)
            e["texte"] += " (ajout)"
            with open(p, "w", encoding="utf-8") as f:
                json.dump(e, f)
            with self.assertRaises(ValueError):
                corpus.charger(d)
        finally:
            shutil.rmtree(d)

    @unittest.skipUnless(os.path.exists(os.path.join(corpus.RACINE, "bin", "eurlex")) and
                         os.path.exists(os.path.join(corpus.RACINE, "bin", "legifrance")),
                         "serveurs non construits (scripts/construire-serveurs.sh)")
    def test_reconstruit_a_l_identique_en_rejeu(self):
        if os.environ.get("PISTE_CLIENT_ID"):
            self.skipTest("identifiants PISTE présents : la reconstruction irait en MIXTE")
        d = tempfile.mkdtemp()
        try:
            index = corpus.construire(d)
            _, attendu = corpus.charger()
            self.assertEqual(index["empreinte"], attendu["empreinte"])
        finally:
            shutil.rmtree(d)
