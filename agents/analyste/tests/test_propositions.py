"""Rédaction (réponses enregistrées, jamais d'appel réel), écriture des
propositions, mesure et critère D-009."""

import json
import os
import shutil
import subprocess
import tempfile
import unittest

from analyste import __main__ as cli
from analyste import critere, detecteurs as det, mesure, redaction

from .test_detecteurs import JEU, SANS_D009, SESSIONS

ICI = os.path.dirname(os.path.abspath(__file__))
EXEMPLES = os.path.join(os.path.dirname(ICI), "exemples", "propositions")
ID_INTERNE = "A-source-interne-perimee-banque-synthetique.attribuer_pret"
CHAMPS_CONTRAT = {"id", "date", "constat", "sessions", "texte_propose", "fondement_propose", "explication"}


def client_rejeu():
    os.environ.pop("MISTRAL_API_KEY", None)
    return redaction.client("rejeu")


def constats():
    return cli.constats(SESSIONS, JEU)["constats"]


class TestVueModele(unittest.TestCase):
    def test_aucune_valeur_du_coffre(self):
        for c in constats():
            vue = json.dumps(redaction.messages(c, redaction.roles()), ensure_ascii=False)
            for o in c["occurrences"]:
                for v in o["valeurs_coffre"].values():
                    self.assertNotIn(str(v), vue)
            self.assertNotIn("valeurs_coffre", vue)
            self.assertNotIn(c["detail"], vue)

    def test_contexte_du_journal(self):
        c = next(c for c in constats() if c["id"] == ID_INTERNE)
        self.assertEqual(c["contexte"]["roles_ayant_valide_cette_action"], ["analyste-risques", "directeur-risques"])
        self.assertIn("banque-synthetique/get_taux_interne", c["contexte"]["fait_observable_commun"])


class TestVerifier(unittest.TestCase):
    def setUp(self):
        self.c = next(c for c in constats() if c["id"] == ID_INTERNE)
        self.ok = {"texte_propose": "T", "fondement_propose": "Code monétaire et financier, L313-5-1",
                   "explication": "Voir " + SANS_D009 + ".", "sessions_citees": [SANS_D009]}

    def test_fondement_marque_et_sessions_completees(self):
        r = redaction.verifier(self.ok, self.c)
        self.assertTrue(r["fondement_propose"].endswith("— à valider par le juriste"))
        for s in self.c["sessions"]:
            self.assertIn(s, r["explication"])

    def test_session_inventee(self):
        with self.assertRaises(redaction.RedactionInvalide):
            redaction.verifier(dict(self.ok, explication="Voir 20261002-000000-inventee."), self.c)

    def test_valeur_du_coffre(self):
        with self.assertRaises(redaction.RedactionInvalide):
            redaction.verifier(dict(self.ok, texte_propose="Le dossier DOS-FICTIF-002 attend."), self.c)

    def test_vide(self):
        with self.assertRaises(redaction.RedactionInvalide):
            redaction.verifier(dict(self.ok, texte_propose=" "), self.c)


class TestProposer(unittest.TestCase):
    def setUp(self):
        self.sortie = tempfile.mkdtemp()

    def tearDown(self):
        shutil.rmtree(self.sortie)

    def test_ecrit_les_propositions_enregistrees(self):
        r = cli.proposer(SESSIONS, JEU, self.sortie, client_rejeu())
        self.assertEqual(r["erreurs"], [])
        self.assertEqual(len(r["ecrites"]), 3)
        for p in r["ecrites"]:
            with open(os.path.join(self.sortie, p["id"] + ".json"), encoding="utf-8") as f:
                ecrite = json.load(f)
            self.assertEqual(set(ecrite), CHAMPS_CONTRAT)
            self.assertEqual(set(ecrite["constat"]), {"type", "detail", "occurrences"})
            with open(os.path.join(EXEMPLES, p["id"] + ".json"), encoding="utf-8") as f:
                exemple = json.load(f)
            for k in ("texte_propose", "fondement_propose", "explication", "sessions"):
                self.assertEqual(ecrite[k], exemple[k], k)
        self.assertEqual([n for n in os.listdir(self.sortie) if "decision" in n], [])

    def test_texte_d009(self):
        cli.proposer(SESSIONS, JEU, self.sortie, client_rejeu())
        with open(os.path.join(self.sortie, ID_INTERNE + ".json"), encoding="utf-8") as f:
            p = json.load(f)
        self.assertIn("banque-synthetique/get_taux_interne", p["texte_propose"])
        self.assertIn("analyste-risques", p["texte_propose"])
        self.assertIn("à valider par le juriste", p["fondement_propose"])

    def test_jamais_reecrite(self):
        cli.proposer(SESSIONS, JEU, self.sortie, client_rejeu())
        chemin = os.path.join(self.sortie, ID_INTERNE + ".json")
        with open(chemin, "w") as f:
            f.write('{"id": "modifiee"}')
        r = cli.proposer(SESSIONS, JEU, self.sortie, client_rejeu())
        self.assertEqual(r["ecrites"], [])
        self.assertIn(ID_INTERNE, r["deja_proposees"])
        with open(chemin) as f:
            self.assertEqual(json.load(f), {"id": "modifiee"})

    def test_rejetee_pas_reproposee(self):
        with open(os.path.join(self.sortie, ID_INTERNE + ".decision.json"), "w") as f:
            json.dump({"decision": "rejetee", "motif": "m"}, f)
        r = cli.proposer(SESSIONS, JEU, self.sortie, client_rejeu())
        self.assertIn(ID_INTERNE, r["deja_proposees"])
        self.assertFalse(os.path.exists(os.path.join(self.sortie, ID_INTERNE + ".json")))


class TestMesure(unittest.TestCase):
    def setUp(self):
        self.d = tempfile.mkdtemp()
        self.prop = os.path.join(self.d, "prop")
        self.comp = os.path.join(self.d, "comp")
        os.makedirs(self.prop)
        shutil.copy(os.path.join(EXEMPLES, ID_INTERNE + ".json"), self.prop)

    def tearDown(self):
        shutil.rmtree(self.d)

    def decider(self, decision, conversation="conv1"):
        with open(os.path.join(self.prop, ID_INTERNE + ".decision.json"), "w") as f:
            json.dump({"decision": decision, "motif": "m", "texte_retenu": "t", "auteur": "j",
                       "date": "2026-10-04T09:00:00Z", "conversation_compilateur": conversation}, f)

    def candidat(self):
        with open(JEU, encoding="utf-8") as f:
            demo = json.load(f)
        r = critere.derouler()
        os.makedirs(os.path.join(self.comp, "conv1"))
        jeu = {"jeu": {"id": "demo", "version": 2, "date_effet": "2026-10-04T11:05:00+02:00", "auteur": "j"},
               "directives": demo["directives"] + r["ajoutees"]}
        with open(os.path.join(self.comp, "conv1", "demo.v2.json"), "w") as f:
            json.dump(jeu, f)

    def mesurer(self):
        return mesure.mesurer(ID_INTERNE, propositions=self.prop, sessions=SESSIONS, compilation=self.comp)

    def test_avant_decision(self):
        r = self.mesurer()
        self.assertEqual(r["etat"], "avant_decision")
        self.assertEqual(len(r["sessions_concernees"]), 3)

    def test_rejetee(self):
        self.decider("rejetee")
        self.assertEqual(self.mesurer()["etat"], "rejetee")

    def test_avant_compilation(self):
        self.decider("reprise_modifiee")
        self.assertEqual(self.mesurer()["etat"], "avant_compilation")

    def test_si_avait_existe_ne_touche_pas_le_candidat(self):
        self.candidat()
        chemin = os.path.join(self.comp, "conv1", "demo.v2.json")
        with open(chemin) as f:
            avant = f.read()
        with open(JEU) as f:
            jeu, ajoutees = mesure.jeu_si_avait_existe(json.loads(avant), json.load(f))
        self.assertEqual(ajoutees, ["D-009"])
        d = jeu["directives"][-1]
        self.assertEqual((d["statut"], d["date_effet"]), ("active", mesure.DATE_SI_AVAIT_EXISTE))
        with open(chemin) as f:
            self.assertEqual(f.read(), avant)

    @unittest.skipUnless(os.path.exists(os.path.join(det.RACINE, "bin", "journal")),
                         "bin/journal absent (scripts/lancer-demo.sh le construit)")
    def test_rejeu(self):
        self.decider("reprise_telle_quelle")
        self.candidat()
        ancienne = os.environ.get("MCP_GATE_CLE_JOURNAL")
        # Clé de test des journaux figés (64 zéros), pas un secret.
        os.environ["MCP_GATE_CLE_JOURNAL"] = "0" * 64
        try:
            r = self.mesurer()
        finally:
            if ancienne is None:
                os.environ.pop("MCP_GATE_CLE_JOURNAL")
            else:
                os.environ["MCP_GATE_CLE_JOURNAL"] = ancienne
        self.assertEqual(r["etat"], "mesuree", r)
        self.assertEqual(r["erreurs"], [])
        self.assertEqual(len(r["sessions_rejouees"]), 6)  # toutes les sessions, pas seulement le constat
        self.assertEqual(r["appels_changes"], 1)
        self.assertEqual(r["changements"], [{"seq": 5, "outil": "banque-synthetique/attribuer_pret",
                                             "avant": "autoriser", "apres": "attendre_validation",
                                             "session": SANS_D009}])

    def test_cle_absente(self):
        self.decider("reprise_telle_quelle")
        self.candidat()
        ancienne = os.environ.pop("MCP_GATE_CLE_JOURNAL", None)
        try:
            self.assertEqual(self.mesurer()["etat"], "cle_absente")
        finally:
            if ancienne is not None:
                os.environ["MCP_GATE_CLE_JOURNAL"] = ancienne

    def test_lire_avant_apres(self):
        sortie = ("Avant / après : journal tel quel, puis sous demo v2\n"
                  "SESSION   SEQ  SERVEUR/OUTIL    AVANT      APRÈS\n"
                  "4db879fc  4    a/b              autoriser  autoriser\n"
                  "4db879fc  5    a/c              autoriser  attendre_validation  ←\n"
                  "Transitions :\n")
        self.assertEqual(mesure.lire_avant_apres(sortie),
                         [{"seq": 5, "outil": "a/c", "avant": "autoriser", "apres": "attendre_validation"}])


class TestCritere(unittest.TestCase):
    def test_d009_hors_ligne(self):
        os.environ.pop("MISTRAL_API_KEY", None)
        r = critere.derouler()
        self.assertTrue(r["conforme_d009"], r.get("ecarts"))
        (d,) = r["ajoutees"]
        self.assertEqual(d["id"], "D-009")
        self.assertEqual(d["condition"], {"session.a_consulte": {"outil": "banque-synthetique/get_taux_interne",
                                                                 "critere": "donnees_recues"}})
        self.assertEqual(d["effets"], [{"effet": "attendre_validation", "par_role": "analyste-risques"}])

    def test_conforme_rend_les_ecarts(self):
        d = {"declencheur": {"evenement": "appel_outil", "cible": {"serveur": "banque-synthetique",
                                                                     "outil": "attribuer_pret"}},
             "condition": {"non": {"session.a_consulte": {"outil": critere.INTERNE, "critere": "transmis"}}},
             "effets": [{"effet": "refuser"}]}
        e = critere.conforme(d)
        self.assertEqual(len(e), 3)


class TestCli(unittest.TestCase):
    def test_constats_sans_modele(self):
        p = subprocess.run(["python3", "-m", "analyste", "constats", "--sessions", SESSIONS],
                           cwd=os.path.dirname(os.path.dirname(ICI)), capture_output=True, text=True,
                           env={k: v for k, v in os.environ.items() if k != "MISTRAL_API_KEY"})
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertEqual(len(json.loads(p.stdout)["constats"]), 3)


if __name__ == "__main__":
    unittest.main()
