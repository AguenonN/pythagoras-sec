"""Orchestrateur devant un faux gate, avec un modèle scripté (rejeu dans
l'ordre) : aucun appel réseau."""

import io
import json
import os
import shutil
import sys
import tempfile
import unittest

from boucle import mcp
from orchestrateur import __main__ as cli
from orchestrateur import modele, orchestrateur, session, sous_agents

ICI = os.path.dirname(os.path.abspath(__file__))
FAUX_GATE = os.path.join(ICI, "faux_gate.py")
EMPREINTE = "sha256:" + "ab" * 32


def appel(nom, args, i="c1"):
    return {"choices": [{"message": {"role": "assistant", "content": "", "tool_calls": [
        {"id": i, "type": "function", "function": {"name": nom, "arguments": json.dumps(args, ensure_ascii=False)}}]}}]}


def texte(t):
    return {"choices": [{"message": {"role": "assistant", "content": t, "tool_calls": None}}]}


def deleguer(nom, tache, i="o1"):
    return appel("deleguer", {"sous_agent": nom, "tache": tache}, i)


ANALYSE_OK = {"dossier_id": "DOS-1", "demandeur": "BIOCOOP", "siren": "382891752", "montant_demande": 400000,
              "categorie_taux": "decouvert-societes-non-financieres", "taux_propose_pourcent": 19.05, "synthese": "ok"}
CREDIT_OK = {"decision": "attribue", "motif": "conditions réunies", "reference": "PRET-FICTIF-0001"}


class Banc(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.journal = os.path.join(self.tmp, "faux.jsonl")
        env = dict(os.environ, FAUX_GATE_JOURNAL=self.journal)
        self.gate = mcp.ClientMCP([sys.executable, FAUX_GATE], env=env)
        self.gate.demarrer()
        self.exposes = {o["name"] for o in self.gate.lister_outils()}
        self.dossier = session.DossierSession(os.path.join(self.tmp, "session"), "demande", "camille", "t0")
        self.statuts = []
        ecrire = self.dossier._ecrire_etat

        def espion():
            self.statuts.append(self.dossier.etat["statut"])
            ecrire()
        self.dossier._ecrire_etat = espion
        self.depots = []  # (jetons à déposer à chaque sommeil)

    def tearDown(self):
        self.gate.fermer()
        shutil.rmtree(self.tmp)

    def approbations(self, delai=5):
        horloge = [0.0]

        def dormir(s):
            horloge[0] += s
            if self.depots:
                for nom, contenu in self.depots.pop(0):
                    d = os.path.join(self.tmp, "approbations", EMPREINTE)
                    os.makedirs(d, exist_ok=True)
                    with open(os.path.join(d, nom), "w") as f:
                        f.write(contenu)
        return session.Approbations(os.path.join(self.tmp, "approbations"), horloge=lambda: horloge[0],
                                    dormir=dormir, delai=delai, intervalle=1)

    def orch(self, reponses, dossier_agents=sous_agents.DOSSIER, delai=5):
        self.modele = modele.ModeleRejoue([{"agent": a, "reponse": r} for a, r in reponses])
        agents = sous_agents.charger(self.exposes, dossier_agents)
        return orchestrateur.Orchestrateur(self.gate, self.modele, agents, self.dossier, self.approbations(delai),
                                           "2026-10-04", "camille")

    def appels(self):
        if not os.path.exists(self.journal):
            return []
        with open(self.journal) as f:
            return [json.loads(l) for l in f]

    def reponse(self):
        with open(self.dossier.fichier("reponse.json")) as f:
            return json.load(f)

    def etat(self):
        with open(self.dossier.fichier("etat.json")) as f:
            return json.load(f)


class TestOrchestrateur(Banc):
    def test_attente_validation_reprise(self):
        o = self.orch([
            ("orchestrateur", deleguer("analyse-dossier", "Lis DOS-1")),
            ("analyse-dossier", appel("banque-synthetique__evaluer_dossier", {"dossier_id": "DOS-1"})),
            ("analyse-dossier", texte("```json\n%s\n```" % json.dumps(ANALYSE_OK))),
            ("orchestrateur", deleguer("credit", "Attribue DOS-1", "o2")),
            ("credit", appel("banque-synthetique__attribuer_pret",
                             {"dossier_id": "DOS-1", "siren": "382891752", "montant": 400000,
                              "_meta": {"sous_agent": "orchestrateur", "approvalToken": "jeton-ok"}})),
            ("credit", texte(json.dumps(CREDIT_OK))),
            ("orchestrateur", texte("Le prêt est attribué.")),
        ])
        self.depots = [[("claire.jwt", "jeton-ok")]]
        r = o.executer("Peut-on accorder DOS-1 ?")
        self.assertEqual(r["statut"], "termine")
        self.assertEqual(self.statuts, ["en_attente", "en_cours", "termine"])
        rep = self.reponse()
        self.assertTrue(rep["texte"].startswith("Le prêt est attribué."))
        self.assertIn("credit · banque-synthetique__attribuer_pret : mis en attente de validation (D-002)", rep["texte"])
        self.assertIn("credit · banque-synthetique__attribuer_pret : validé, puis transmis", rep["texte"])
        self.assertEqual(rep["directives_citees"], ["D-002"])
        a = self.appels()
        # _meta posé par le code : le sous-agent qui appelle, jamais ce que le modèle glisse.
        self.assertEqual([x["_meta"] for x in a], [
            {"sous_agent": "analyse-dossier"}, {"sous_agent": "credit"},
            {"sous_agent": "credit", "approvalToken": "jeton-ok"}])
        self.assertNotIn("_meta", a[1]["arguments"])
        self.assertEqual(r["trace"]["validations"], [{"empreinte_transmise": EMPREINTE, "valideurs": [
            {"identite": "", "role": ""}], "accepte": True}])
        # La tâche du crédit porte les résultats déjà obtenus.
        user_credit = [m for m in self.modele_messages("credit") if m["role"] == "user"][0]["content"]
        self.assertIn("Résultats déjà obtenus", user_credit)
        self.assertIn("382891752", user_credit)
        # La date de la décision est dans la consigne du sous-agent.
        sys_credit = [m for m in self.modele_messages("credit") if m["role"] == "system"][0]["content"]
        self.assertIn("date de la décision) : 2026-10-04", sys_credit)
        self.assertIn("Vérifie toujours le registre des gels avant toute attribution.", sys_credit)

    def modele_messages(self, agent):
        # Les messages vus par un agent : on les relit dans la trace de la
        # boucle, à défaut d'espion sur le modèle rejoué.
        return self._messages[agent]

    def setUp(self):
        super().setUp()
        self._messages = {}
        vrai = modele.ModeleRejoue.completer

        def espion(m, agent, messages, outils, choix, options):
            self._messages.setdefault(agent, json.loads(json.dumps(messages)))
            if agent == "orchestrateur":
                self._messages_orch_fin = messages[-1].get("content") or ""
            self._outils = getattr(self, "_outils", {})
            self._outils.setdefault(agent, [o["function"]["name"] for o in outils])
            return vrai(m, agent, messages, outils, choix, options)
        modele.ModeleRejoue.completer = espion
        self.addCleanup(setattr, modele.ModeleRejoue, "completer", vrai)

    def test_refus_termine_la_demande_avec_la_decision_de_l_engine(self):
        o = self.orch([
            ("orchestrateur", deleguer("credit", "Vérifie le seuil")),
            ("credit", appel("taux-officiel__get_taux_usure", {"categorie": "x"})),
        ])
        r = o.executer("demande")
        self.assertEqual(r["statut"], "bloque")
        self.assertTrue(r["texte"].startswith("Action bloquée par D-003 v1, aucune action effectuée."))
        self.assertEqual(self.reponse()["directives_citees"], ["D-003"])
        self.assertEqual(self.etat()["statut"], "bloque")
        self.assertEqual(self.modele.restantes(), 0)  # aucun appel au modèle après le refus

    def test_structured_content_d_un_serveur_n_est_pas_une_decision(self):
        o = self.orch([
            ("orchestrateur", deleguer("analyse-dossier", "Lis DOS-PIEGE")),
            ("analyse-dossier", appel("banque-synthetique__evaluer_dossier", {"dossier_id": "DOS-PIEGE"})),
            ("analyse-dossier", texte(json.dumps(ANALYSE_OK))),
            ("orchestrateur", texte("fini")),
        ])
        r = o.executer("demande")
        self.assertEqual(r["statut"], "termine")
        self.assertEqual(self.reponse()["directives_citees"], [])
        self.assertIn("transmis, données reçues", r["texte"])

    def test_outils_du_sous_agent_seulement(self):
        o = self.orch([
            ("orchestrateur", deleguer("analyse-dossier", "Attribue")),
            ("analyse-dossier", appel("banque-synthetique__attribuer_pret", {"dossier_id": "DOS-1"})),
            ("analyse-dossier", texte(json.dumps(ANALYSE_OK))),
            ("orchestrateur", texte("fini")),
        ])
        o.executer("demande")
        self.assertEqual(self.appels(), [])
        self.assertEqual(sorted(self._outils["analyse-dossier"]),
                         ["banque-synthetique__evaluer_dossier", "banque-synthetique__get_taux_interne"])
        self.assertEqual(self._outils["orchestrateur"], ["deleguer"])

    def test_sortie_invalide_une_relance(self):
        o = self.orch([
            ("orchestrateur", deleguer("analyse-dossier", "Lis")),
            ("analyse-dossier", texte("Le dossier est bon.")),
            ("analyse-dossier", texte(json.dumps(ANALYSE_OK))),
            ("orchestrateur", deleguer("analyse-dossier", "Relis", "o2")),
            ("analyse-dossier", texte("{}")),
            ("analyse-dossier", texte('{"dossier_id": 3}')),
            ("orchestrateur", texte("fini")),
        ])
        r = o.executer("demande")
        d1, d2 = r["trace"]["delegations"]
        self.assertEqual(d1["etat"], "termine")
        self.assertEqual(d1["relance"], ["la réponse finale n'est pas un objet JSON"])
        self.assertEqual(d2["etat"], "sortie_invalide")
        self.assertIn("$.dossier_id : type integer, attendu string", d2["ecarts"])

    def test_validation_non_recue(self):
        o = self.orch([
            ("orchestrateur", deleguer("credit", "Attribue")),
            ("credit", appel("banque-synthetique__attribuer_pret", {"dossier_id": "DOS-1"})),
        ], delai=3)
        r = o.executer("demande")
        self.assertEqual(r["statut"], "bloque")
        e = self.etat()
        self.assertEqual(e["attente"]["motif"], "validation non reçue")
        self.assertEqual(e["attente"]["roles"], ["analyste-risques"])
        self.assertEqual(e["attente"]["directives"], ["D-002"])
        self.assertTrue(r["texte"].startswith("Validation non reçue dans le délai"))

    def test_rejet_de_la_console_termine_bloque(self):
        # La console (« Rejeter ») écrit approbations/<empreinte>/rejet.json.
        o = self.orch([
            ("orchestrateur", deleguer("credit", "Attribue")),
            ("credit", appel("banque-synthetique__attribuer_pret", {"dossier_id": "DOS-1"})),
        ], delai=600)
        self.depots = [[("rejet.json", json.dumps({"valideur": "claire", "nom": "Claire Analyste",
                                                    "motif": "taux non justifié", "date": "2026-10-04T09:00:00Z"}))]]
        r = o.executer("demande")
        self.assertEqual(r["statut"], "bloque")
        self.assertEqual(self.etat()["attente"]["motif"], "validation rejetée")
        self.assertTrue(r["texte"].startswith("Validation rejetée par Claire Analyste : taux non justifié."), r["texte"])
        # aucune reprise : un seul tools/call de l'attribution, sans jeton
        self.assertEqual([a["name"] for a in self.appels()], ["banque-synthetique__attribuer_pret"])

    def test_rejet_d_une_demande_precedente_ecarte(self):
        d = os.path.join(self.tmp, "approbations", EMPREINTE)
        os.makedirs(d)
        with open(os.path.join(d, "rejet.json"), "w") as f:
            f.write("{}")  # même appel, demande précédente
        o = self.orch([
            ("orchestrateur", deleguer("credit", "Attribue")),
            ("credit", appel("banque-synthetique__attribuer_pret", {"dossier_id": "DOS-1"})),
            ("credit", texte(json.dumps(CREDIT_OK))),
            ("orchestrateur", texte("fini")),
        ])
        self.depots = [[("claire.jwt", "jeton-ok")]]
        r = o.executer("demande")
        self.assertEqual(r["statut"], "termine")
        self.assertEqual(sorted(os.listdir(d)), ["claire.jwt.utilise", "rejet.json.ancien"])

    def test_reprise_refusee_puis_acceptee(self):
        o = self.orch([
            ("orchestrateur", deleguer("credit", "Attribue")),
            ("credit", appel("banque-synthetique__attribuer_pret", {"dossier_id": "DOS-1"})),
            ("credit", texte(json.dumps(CREDIT_OK))),
            ("orchestrateur", texte("Attribué après validation.")),
        ])
        self.depots = [[("diane.jwt", "jeton-mauvais")], [("paul.jwt", "jeton-ok")]]
        r = o.executer("demande")
        self.assertEqual(r["statut"], "termine")
        self.assertEqual([v["accepte"] for v in r["trace"]["validations"]], [False, True])
        d = os.path.join(self.tmp, "approbations", EMPREINTE)
        self.assertEqual(sorted(os.listdir(d)), ["diane.jwt.refuse", "paul.jwt.utilise"])
        self.assertEqual(self.statuts, ["en_attente", "en_cours", "en_attente", "en_cours", "termine"])

    def test_jetons_d_une_demande_precedente_ecartes(self):
        d = os.path.join(self.tmp, "approbations", EMPREINTE)
        os.makedirs(d)
        with open(os.path.join(d, "ancien.jwt"), "w") as f:
            f.write("jeton-ok")  # même appel, demande précédente
        o = self.orch([
            ("orchestrateur", deleguer("credit", "Attribue")),
            ("credit", appel("banque-synthetique__attribuer_pret", {"dossier_id": "DOS-1"})),
            ("credit", texte(json.dumps(CREDIT_OK))),
            ("orchestrateur", texte("fini")),
        ])
        self.depots = [[("claire.jwt", "jeton-ok")]]
        r = o.executer("demande")
        self.assertEqual(r["statut"], "termine")
        self.assertEqual(sorted(os.listdir(d)), ["ancien.jwt.ancien", "claire.jwt.utilise"])
        self.assertEqual(self.statuts, ["en_attente", "en_cours", "termine"])

    def test_action_faite_pas_redeleguee(self):
        # Passage réel du 01/10 : l'orchestrateur avait redélégué l'attribution
        # déjà confirmée ; le même appel retombait en attente.
        o = self.orch([
            ("orchestrateur", deleguer("credit", "Attribue")),
            ("credit", appel("banque-synthetique__attribuer_pret", {"dossier_id": "DOS-1"})),
            ("credit", texte(json.dumps(CREDIT_OK))),
            ("orchestrateur", deleguer("credit", "Attribue en référence PRET-FICTIF-0001", "o2")),
            ("orchestrateur", texte("Attribué.")),
        ])
        self.depots = [[("claire.jwt", "jeton-ok")]]
        r = o.executer("demande")
        self.assertEqual(r["statut"], "termine")
        self.assertEqual(len(r["trace"]["delegations"]), 1)
        self.assertEqual(len(self.appels()), 2)
        dernier = self._messages_orch_fin
        self.assertIn("Refusé par l'orchestrateur : le sous-agent credit a déjà effectué une action", dernier)

    def test_action_deja_transmise_non_renvoyee_par_un_autre_sous_agent(self):
        # Passage réel du 02/10 : credit retentait l'attribution obtenue par
        # credit-historique ; le même appel retombait en attente (D-002).
        args = {"dossier_id": "DOS-1", "siren": "382891752", "montant": 150000}
        o = self.orch([
            ("orchestrateur", deleguer("credit-historique", "Décide")),
            ("credit-historique", appel("banque-synthetique__attribuer_pret", args)),
            ("credit-historique", texte(json.dumps(CREDIT_OK))),
            ("orchestrateur", deleguer("credit", "Vérifie et attribue", "o2")),
            ("credit", appel("banque-synthetique__attribuer_pret", args, "c2")),
            ("credit", texte(json.dumps({"decision": "non_attribue", "motif": "déjà fait"}))),
            ("orchestrateur", texte("Attribué.")),
        ])
        self.depots = [[("claire.jwt", "jeton-ok")]]
        r = o.executer("demande")
        self.assertEqual(r["statut"], "termine")
        self.assertEqual(len(self.appels()), 2)  # attente puis reprise, rien de plus
        self.assertIn("credit · banque-synthetique__attribuer_pret : non envoyé par l'orchestrateur", r["texte"])

    def test_sous_agent_ajoute_par_un_fichier(self):
        d = os.path.join(self.tmp, "agents")
        shutil.copytree(sous_agents.DOSSIER, d)
        with open(os.path.join(d, "veille.json"), "w") as f:
            json.dump({"nom": "veille", "consigne": "Tu es le sous-agent veille. Tu cherches un nom au registre des gels.",
                       "outils": ["gels-avoirs__verifier_gel"],
                       "schema_sortie": {"type": "object", "properties": {"gel": {"type": "boolean"}},
                                         "required": ["gel"]}}, f)
        o = self.orch([
            ("orchestrateur", deleguer("veille", "Cherche ALPHA")),
            ("veille", appel("gels-avoirs__verifier_gel", {"nom": "ALPHA"})),
            ("veille", texte('{"gel": false}')),
            ("orchestrateur", texte("Aucun gel.")),
        ], dossier_agents=d)
        self.assertIn("- veille : Tu es le sous-agent veille.", o.consigne())
        r = o.executer("demande")
        self.assertEqual(r["statut"], "termine")
        self.assertEqual(self.appels()[0]["_meta"], {"sous_agent": "veille"})
        self.assertEqual([d["sous_agent"] for d in r["trace"]["delegations"]], ["veille"])


class TestSousAgents(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.tmp)

    def ecrire(self, nom, d):
        with open(os.path.join(self.tmp, nom + ".json"), "w") as f:
            json.dump(d, f)

    def test_les_sous_agents_de_la_demo_se_chargent_devant_le_catalogue(self):
        # Les serveurs de l'opération ATLAS (session opération) ne sont pas dans
        # le catalogue figé de la démo (agents/compilateur/exemples, non modifiable) :
        # leurs outils sont réunis ici, comme le bloc OPERATION de faux_gate.py, pour
        # que assistant-salarie et equipe-deal se chargent sans toucher au catalogue figé.
        operation = {"data-room__lister_documents", "data-room__lire_document", "data-room__partager_document",
                     "messagerie__rediger_message", "messagerie__envoyer_message",
                     "bourse__consulter_cours", "bourse__passer_ordre",
                     "communication__preparer_communique", "communication__publier_communique"}
        agents = sous_agents.charger(sous_agents.outils_du_catalogue(cli.CATALOGUE) | operation)
        self.assertEqual(sorted(agents), ["analyse-dossier", "assistant-salarie", "conformite", "credit",
                                          "credit-historique", "equipe-deal", "lcb-ft",
                                          "operations", "redaction", "rh", "rh-disciplinaire", "rh-gestion"])
        # Secteurs : consignes neutres, aucune règle (c'est l'engine qui
        # applique le droit, docs/demo/secteurs.md).
        interdits = ("règle", "interdit", "jamais", "valid", "gel", "soupçon", "article", "rgpd", "code du", "directive",
                     "entretien préalable", "protégé", "syndic", "santé", "externe", "plafond")
        for nom in ("operations", "lcb-ft", "rh-gestion", "rh-disciplinaire"):
            consigne = agents[nom].consigne.lower()
            self.assertEqual([m for m in interdits if m in consigne], [], nom)
        self.assertEqual(agents["credit-historique"].outils,
                         ["banque-synthetique__attribuer_pret", "banque-synthetique__get_taux_interne"])
        self.assertEqual(agents["analyse-dossier"].outils,
                         ["banque-synthetique__evaluer_dossier", "banque-synthetique__get_taux_interne"])
        self.assertEqual(agents["rh"].outils, ["referentiel-rh__groupes_salarie", "rh-synthetique__consulter_salarie",
                                               "rh-synthetique__decider_prime", "rh-synthetique__proposer_sanction"])
        self.assertEqual(agents["credit"].outils, ["banque-synthetique__attribuer_pret", "taux-officiel__get_taux_usure"])
        self.assertIn("Vérifie toujours le registre des gels avant toute attribution.", agents["credit"].consigne)

    def test_refus_clairs(self):
        self.ecrire("a", {"nom": "b", "consigne": "x", "outils": ["s__absent", "t__*"], "inconnu": 1,
                          "schema_sortie": {"type": "object", "properties": {"x": {"type": "chaine", "pattern": "a"}},
                                            "required": ["y"]}})
        with self.assertRaises(sous_agents.SousAgentInvalide) as c:
            sous_agents.charger({"s__present"}, self.tmp)
        m = str(c.exception)
        for attendu in ("a.json : champ inconnu « inconnu »", "a.json : nom « b » différent du nom du fichier « a »",
                        "a.json : outil s__absent : absent du registre",
                        "a.json : outil t__* : aucun outil de ce serveur au registre",
                        "a.json : schema_sortie.properties.x : mot-clé non admis « pattern »",
                        "a.json : schema_sortie.properties.x.type : 'chaine'",
                        "a.json : schema_sortie.required : « y » absent de properties"):
            self.assertIn(attendu, m)

    def test_outils_du_registre(self):
        chemin = os.path.join(self.tmp, "registre.json")
        with open(chemin, "w") as f:
            json.dump({"registre": {}, "serveurs": [{"nom": "s", "outils": [{"nom": "o"}, {"nom": "p"}]},
                                                    {"nom": "t", "outils": []}]}, f)
        self.assertEqual(sous_agents.outils_du_registre(chemin), {"s__o", "s__p"})

    def test_role_par_defaut_premiere_phrase(self):
        self.ecrire("a", {"nom": "a", "consigne": "Tu lis. Puis tu écris.", "outils": ["s__o"],
                          "schema_sortie": {"type": "object"}})
        self.assertEqual(sous_agents.charger({"s__o"}, self.tmp)["a"].role, "Tu lis.")


class TestCLI(unittest.TestCase):
    """executer en mode réel (client Mistral scripté), puis rejeu de
    l'enregistrement : mêmes appels au gate, même réponse."""

    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.tmp)

    def lancer(self, mode, client=None, nom="s1", options=()):
        env_fichier = os.path.join(self.tmp, nom + ".env")
        journal = os.path.join(self.tmp, nom + ".jsonl")
        os.environ.update(FAUX_GATE_ENV=env_fichier, FAUX_GATE_JOURNAL=journal, MISTRAL_API_KEY="secret-de-test")
        vrai = modele.ModeleReel.__init__

        def init(m, cache, etat=modele.ETAT, max_appels=None, client_=None, modele=modele.MODELE_AGENTS):
            self.modeles.append(modele)
            vrai(m, cache, etat, max_appels, client=client, modele=modele)
        self.modeles = []
        modele.ModeleReel.__init__ = init
        try:
            out = io.StringIO()
            code = cli.principal(["executer", "--session", os.path.join(self.tmp, nom), "--approbations",
                                  os.path.join(self.tmp, "appro"), "--mode", mode, "--enregistrement",
                                  os.path.join(self.tmp, "enr.json"), "--demande", "Lis DOS-1", "--demandeur",
                                  "camille", "--date", "2026-10-04", "--scenario", "essai", *options, "--",
                                  sys.executable, FAUX_GATE], out)
        finally:
            modele.ModeleReel.__init__ = vrai
            for k in ("FAUX_GATE_ENV", "FAUX_GATE_JOURNAL", "MISTRAL_API_KEY"):
                os.environ.pop(k, None)
        with open(env_fichier) as f:
            env = json.load(f)
        appels = []
        if os.path.exists(journal):
            with open(journal) as f:
                appels = [json.loads(l) for l in f]
        return code, out.getvalue(), env, appels

    def test_reel_puis_rejeu(self):
        class Client:
            reponses = [deleguer("analyse-dossier", "Lis DOS-1"),
                        appel("banque-synthetique__evaluer_dossier", {"dossier_id": "DOS-1"}),
                        texte(json.dumps(ANALYSE_OK)), texte("Dossier lu.")]

            def completer(self, *a, **k):
                return self.reponses.pop(0)
        code, sortie, env, appels = self.lancer("reel", Client())
        self.assertEqual(code, 0)
        self.assertNotIn("MISTRAL_API_KEY", env)  # jamais transmise au gate
        with open(os.path.join(self.tmp, "enr.json")) as f:
            enr = json.load(f)
        self.assertEqual([r["agent"] for r in enr["reponses"]],
                         ["orchestrateur", "analyse-dossier", "analyse-dossier", "orchestrateur"])
        self.assertEqual(enr["appels_mistral"], 4)
        self.assertEqual(self.modeles, ["mistral-medium-latest"])  # modèle des agents par défaut
        self.assertEqual(enr["modele"], "mistral-medium-latest")
        self.assertEqual(enr["resultat"], {"statut": "termine", "directives_citees": []})
        self.assertNotIn("secret-de-test", json.dumps(enr))
        code2, sortie2, _, appels2 = self.lancer("rejeu", nom="s2")
        self.assertEqual((code2, sortie2, appels2), (code, sortie, appels))

    def test_option_modele(self):
        class Client:
            reponses = [texte("Hors périmètre.")]

            def completer(self, *a, **k):
                return self.reponses.pop(0)
        code, _, _, _ = self.lancer("reel", Client(), options=("--modele", "mistral-small-latest"))
        self.assertEqual(code, 0)
        self.assertEqual(self.modeles, ["mistral-small-latest"])
        with open(os.path.join(self.tmp, "enr.json")) as f:
            self.assertEqual(json.load(f)["modele"], "mistral-small-latest")

    def test_rejeu_sans_validation_enregistree_n_attend_pas(self):
        with open(os.path.join(self.tmp, "enr.json"), "w") as f:
            json.dump({"demande": "d", "demandeur": "c", "date": "2026-10-04", "validations": [],
                       "reponses": [{"agent": "orchestrateur", "reponse": deleguer("credit", "Attribue")},
                                    {"agent": "credit", "reponse": appel("banque-synthetique__attribuer_pret",
                                                                         {"dossier_id": "DOS-1"})}]}, f)
        code, sortie, _, _ = self.lancer("rejeu")
        self.assertEqual(code, 2)
        self.assertIn("Validation non reçue dans le délai", sortie)

    def test_rejeu_qui_diverge_s_arrete(self):
        with open(os.path.join(self.tmp, "enr.json"), "w") as f:
            json.dump({"demande": "d", "demandeur": "c", "date": "2026-10-04",
                       "reponses": [{"agent": "credit", "reponse": texte("x")}]}, f)
        code, sortie, _, _ = self.lancer("rejeu")
        self.assertEqual(code, 1)
        self.assertIn("rejeu : réponse 1 enregistrée pour credit, demandée par orchestrateur", sortie)


class TestHorlogeSimulee(unittest.TestCase):
    """--horloge-depart : les jetons redéposés en rejeu sont signés à
    l'heure du gate (mcp-approve valider -horloge), pas à l'heure réelle."""

    def test_sans_depart_horloge_reelle(self):
        self.assertIsNone(cli.horloge_simulee(None))
        self.assertIsNone(cli.horloge_simulee(""))

    def test_depart_plus_temps_ecoule(self):
        t = [100.0]
        h = cli.horloge_simulee("2026-10-04T09:00:00+02:00", maintenant=lambda: t[0])
        self.assertEqual(h(), "2026-10-04T07:00:00Z")
        t[0] += 90.4
        self.assertEqual(h(), "2026-10-04T07:01:30Z")
        with self.assertRaises(ValueError):
            cli.horloge_simulee("2026-10-04T09:00:00")

    def test_jetons_rejoues_signes_a_l_heure_simulee(self):
        tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, tmp)
        argv = os.path.join(tmp, "argv.json")
        faux = os.path.join(tmp, "mcp-approve")
        with open(faux, "w") as f:
            f.write("#!%s\nimport json, sys\njson.dump(sys.argv[1:], open(%r, 'w'))\nprint('a.b.c')\n" % (sys.executable, argv))
        os.chmod(faux, 0o755)
        appro = session.Approbations(os.path.join(tmp, "appro"))
        validations = [{"valideurs": [{"identite": "claire", "role": "analyste-risques"}]}]
        t = [0.0]
        h = cli.horloge_simulee("2026-10-04T11:05:00+02:00", maintenant=lambda: t[0])
        t[0] = 30
        cli.valideur_rejeu(validations, appro, faux, os.path.join(tmp, "cles"), h)({"empreinte_transmise": EMPREINTE})
        with open(argv) as f:
            args = json.load(f)
        self.assertEqual(args[args.index("-horloge") + 1], "2026-10-04T09:05:30Z")
        self.assertEqual(args[args.index("-empreinte") + 1], EMPREINTE)
        # Sans horloge simulée : pas d'option, l'heure réelle.
        cli.valideur_rejeu(validations, appro, faux, os.path.join(tmp, "cles"))({"empreinte_transmise": EMPREINTE})
        with open(argv) as f:
            self.assertNotIn("-horloge", json.load(f))


if __name__ == "__main__":
    unittest.main()
