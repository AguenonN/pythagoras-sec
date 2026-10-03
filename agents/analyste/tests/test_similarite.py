"""Embeddings : sessions similaires, fondements suggérés, rejeu sans
réseau, budget. Les embeddings proposent, ne décident jamais."""

import json
import os
import shutil
import tempfile
import unittest

from compilateur import mistral

from analyste import __main__ as cli
from analyste import corpus, plongements, redaction, similarite as sim

from .test_detecteurs import JEU, SESSIONS

ID_INTERNE = "A-source-interne-perimee-banque-synthetique.attribuer_pret"
EXEMPLES = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "exemples", "propositions")


def constats():
    return cli.constats(SESSIONS, JEU)


def exemple(id_):
    with open(os.path.join(EXEMPLES, id_ + ".json"), encoding="utf-8") as f:
        return json.load(f)


class TestSessionsSimilaires(unittest.TestCase):
    def setUp(self):
        os.environ.pop("MISTRAL_API_KEY", None)
        self.res = constats()
        self.p = plongements.Plongeur("rejeu")
        self.c = next(c for c in self.res["constats"] if c["id"] == ID_INTERNE)

    def similaires(self, **k):
        return sim.sessions_similaires(self.c, self.res["_sessions"], self.p, redaction.NATURES, **k)

    def test_trois_au_dessus_du_seuil_hors_fondatrices(self):
        r = self.similaires()
        self.assertEqual([x["session"] for x in r], ["20261002-170145-base-interne-avec-d009-rejeu",
                                                     "20261002-170148-consigne-sans-d001-rejeu",
                                                     "20261002-170149-consigne-avec-d001-rejeu"])
        for x in r:
            self.assertGreaterEqual(x["score"], sim.SEUIL_SESSIONS)
            self.assertNotIn(x["session"], self.c["sessions"])
            self.assertIn(x["proche_de"], self.c["sessions"])
            self.assertIn("score_constat", x)

    def test_parcours_rh_sous_le_seuil(self):
        tous = self.similaires(seuil=0.0, n=99)
        rh = [x for x in tous if "rh-" in x["session"]]
        self.assertEqual(len(rh), 2)
        for x in rh:
            self.assertLess(x["score"], sim.SEUIL_SESSIONS)

    def test_seuil_haut_rien(self):
        self.assertEqual(self.similaires(seuil=0.999), [])

    def test_session_non_terminee_ignoree(self):
        s = next(s for s in self.res["_sessions"] if "avec-d009" in s.nom)
        s.etat = dict(s.etat, statut="en_attente")
        self.assertNotIn(s.nom, [x["session"] for x in self.similaires()])


class TestTextes(unittest.TestCase):
    def test_aucune_valeur_du_coffre_hors_demande(self):
        res = constats()
        for s in res["_sessions"]:
            texte = sim.texte_session(s)
            demande = (s.etat or {}).get("demande", "")
            for a in s.appels:
                for v in a.arguments.values():
                    for x in (v if isinstance(v, list) else [v]):
                        if str(x) and str(x) not in demande:
                            self.assertNotIn(str(x), texte, s.nom)
        for c in res["constats"]:
            t = sim.texte_constat(c, redaction.NATURES)
            for o in c["occurrences"]:
                for v in o["valeurs_coffre"].values():
                    self.assertNotIn(str(v), t)


class TestFondements(unittest.TestCase):
    def test_deux_articles_marques(self):
        os.environ.pop("MISTRAL_API_KEY", None)
        res = constats()
        c = next(c for c in res["constats"] if c["id"] == ID_INTERNE)
        r = sim.fondements_suggeres(c, exemple(ID_INTERNE), plongements.Plongeur("rejeu"), redaction.NATURES)
        self.assertEqual([x["id"] for x in r], ["CONSO-L314-6", "CMF-L313-5-1"])  # l'usure
        for x in r:
            self.assertEqual(x["mention"], "suggestion — à valider par le juriste")
            self.assertTrue(x["lien"].startswith("https://"))


class TestPlongeur(unittest.TestCase):
    def setUp(self):
        self.d = tempfile.mkdtemp()

    def tearDown(self):
        shutil.rmtree(self.d)

    def test_rejeu_sans_enregistrement(self):
        p = plongements.Plongeur("rejeu", enregistrements=self.d)
        with self.assertRaises(mistral.EnregistrementAbsent):
            p.vecteurs(["texte inconnu"])

    def test_reel_par_lot_puis_cache(self):
        recus = []

        def transport(corps, cle):
            recus.append(corps)
            return {"data": [{"index": i, "embedding": [float(i), 1.0]} for i in range(len(corps["input"]))]}

        os.environ["MISTRAL_API_KEY"] = "factice-pour-test"
        try:
            p = plongements.Plongeur("reel", enregistrements=os.path.join(self.d, "e"), etat=os.path.join(self.d, "s"),
                                     max_appels=1, transport=transport)
            v = p.vecteurs(["b", "a", "b"])
            self.assertEqual(len(recus), 1)
            self.assertEqual(recus[0], {"model": "mistral-embed", "input": ["a", "b"]})
            self.assertEqual(v[0], v[2])
            # Servi par les enregistrements : ni appel, ni budget.
            self.assertEqual(plongements.Plongeur("rejeu", enregistrements=os.path.join(self.d, "e")).vecteurs(["a"]),
                             [[0.0, 1.0]])
            with self.assertRaises(mistral.BudgetEpuise):
                p.vecteurs(["c"])
        finally:
            os.environ.pop("MISTRAL_API_KEY", None)

    def test_cosinus(self):
        self.assertAlmostEqual(plongements.cosinus([1, 0], [1, 0]), 1.0)
        self.assertAlmostEqual(plongements.cosinus([1, 0], [0, 1]), 0.0)
        self.assertEqual(plongements.cosinus([0, 0], [1, 0]), 0.0)


class TestProposer(unittest.TestCase):
    def setUp(self):
        os.environ.pop("MISTRAL_API_KEY", None)
        self.sortie = tempfile.mkdtemp()

    def tearDown(self):
        shutil.rmtree(self.sortie)

    def test_champs_facultatifs(self):
        r = cli.proposer(SESSIONS, JEU, self.sortie, redaction.client("rejeu"), plongeur=plongements.Plongeur("rejeu"))
        self.assertEqual((r["erreurs"], r["avertissements"]), ([], []))
        for p in r["ecrites"]:
            e = exemple(p["id"])
            self.assertEqual(p["sessions_similaires"], e["sessions_similaires"])
            self.assertEqual(p["fondements_suggeres"], e["fondements_suggeres"])
            self.assertEqual(len(p["fondements_suggeres"]), 2)

    def test_embedding_absent_n_empeche_pas_la_proposition(self):
        vide = tempfile.mkdtemp()
        try:
            r = cli.proposer(SESSIONS, JEU, self.sortie, redaction.client("rejeu"),
                             plongeur=plongements.Plongeur("rejeu", enregistrements=vide))
        finally:
            shutil.rmtree(vide)
        self.assertEqual(len(r["ecrites"]), 3)
        self.assertEqual(len(r["avertissements"]), 3)
        for p in r["ecrites"]:
            self.assertNotIn("sessions_similaires", p)

    def test_les_embeddings_ne_decident_pas(self):
        # Mêmes constats, mêmes textes, avec ou sans embeddings.
        a = cli.proposer(SESSIONS, JEU, self.sortie, redaction.client("rejeu"))
        d = tempfile.mkdtemp()
        try:
            b = cli.proposer(SESSIONS, JEU, d, redaction.client("rejeu"), plongeur=plongements.Plongeur("rejeu"))
        finally:
            shutil.rmtree(d)
        cle = ("id", "constat", "sessions", "texte_propose", "fondement_propose", "explication")
        self.assertEqual([{k: p[k] for k in cle} for p in a["ecrites"]], [{k: p[k] for k in cle} for p in b["ecrites"]])


class TestCorpusEmbarque(unittest.TestCase):
    def test_vecteurs_du_corpus_enregistres(self):
        os.environ.pop("MISTRAL_API_KEY", None)
        arts, _ = corpus.charger()
        self.assertEqual(len(plongements.Plongeur("rejeu").vecteurs([sim.texte_article(a) for a in arts])), 5)



if __name__ == "__main__":
    unittest.main()
