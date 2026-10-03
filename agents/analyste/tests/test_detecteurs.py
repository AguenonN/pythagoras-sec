"""Détecteurs sur les journaux des passages de la démo (rejoués hors ligne
le 02/10 et figés dans donnees/sessions), et sur des sessions écrites à la
main pour les cas que ces passages n'ont pas."""

import json
import os
import shutil
import tempfile
import unittest

from analyste import couverture, detecteurs as det, journaux, propositions

ICI = os.path.dirname(os.path.abspath(__file__))
SESSIONS = os.path.join(ICI, "donnees", "sessions")
# Jeu figé (version 5 : directives des secteurs et d'ATLAS en brouillon),
# celui des données de test : depuis le 03/10, demo.json les a toutes
# actives (le juriste), ce qui couvre les constats que ces tests attendent.
JEU = os.path.join(det.RACINE, "demo", "directives", "historique", "demo.v5.json")

NOMINAL = "20261002-170137-nominal-rejeu"
TOP = "20261002-170140-top-management-rejeu"
SANS_D009 = "20261002-170144-base-interne-sans-d009-rejeu"
AVEC_D009 = "20261002-170145-base-interne-avec-d009-rejeu"
CONSIGNE_SANS = "20261002-170148-consigne-sans-d001-rejeu"
CONSIGNE_AVEC = "20261002-170149-consigne-avec-d001-rejeu"


def detecter(sessions=None, **k):
    sessions = sessions if sessions is not None else journaux.lire_sessions(SESSIONS)
    return det.detecter(sessions, jeu_courant=couverture.lire_jeu(JEU), **k)


def appel(seq, serveur, outil, acces="read", etiquettes=(), statut="ok", effet="autoriser", empreinte=None,
          ts="2026-10-02T10:00:00Z", source=None, applications=(), approbation=None):
    e = {"type": "appel", "seq": seq, "ts": ts, "serveur": serveur, "outil": outil, "acces": acces,
         "etiquettes_cible": list(etiquettes), "appel": {"empreinte_origine": empreinte or "e%d" % seq},
         "decision": {"effet": effet, "applications": list(applications)},
         "resultat": {"statut": statut, "source": source or {"origine": "non_declaree"}}}
    if approbation:
        e["approbation"] = approbation
    return e


class SessionsEcrites(unittest.TestCase):
    """Sessions synthétiques dans un dossier temporaire."""

    def setUp(self):
        self.dossier = tempfile.mkdtemp()

    def tearDown(self):
        shutil.rmtree(self.dossier)

    def session(self, nom, entrees, coffre):
        d = os.path.join(self.dossier, nom)
        os.makedirs(d)
        with open(os.path.join(d, "journal.jsonl"), "w", encoding="utf-8") as f:
            for e in entrees:
                f.write(json.dumps(e) + "\n")
        with open(os.path.join(d, "coffre.jsonl"), "w", encoding="utf-8") as f:
            for emp, args in coffre.items():
                f.write(json.dumps({"empreinte": emp, "arguments_origine": args}) + "\n")
        return journaux.lire_session(d)


class TestSourceInternePerimee(unittest.TestCase):
    def setUp(self):
        self.occ, _ = detecter()
        self.o = {o["session"]: o for o in self.occ[det.SOURCE_INTERNE_PERIMEE]}

    def test_trouve_dos_fictif_002(self):
        o = self.o[NOMINAL]
        self.assertEqual(o["valeurs_coffre"]["dossier"], "DOS-FICTIF-002")
        self.assertEqual(o["action"], "banque-synthetique/attribuer_pret")
        self.assertEqual(o["seq_action"], 7)  # la reprise transmise
        self.assertEqual(o["consultation_interne"]["seq"], 2)
        self.assertEqual(o["consultation_interne"]["trimestre"], "2026-Q3")
        self.assertTrue(o["source_officielle_consultee"])
        self.assertEqual(o["consultations_officielles"][0]["seq"], 5)
        self.assertEqual(o["trimestre_reference"], "2026-Q4")
        self.assertEqual(o["origine_reference"], "source officielle consultée")

    def test_base_interne_sans_taux_officiel(self):
        # Cas attendu : décision fondée sur la base interne sans
        # consultation du taux officiel ; référence : trimestre en vigueur.
        o = self.o[SANS_D009]
        self.assertFalse(o["source_officielle_consultee"])
        self.assertEqual(o["trimestre_reference"], "2026-Q4")
        self.assertEqual(o["origine_reference"], "trimestre en vigueur à la date de l'action")
        self.assertEqual(o["decision"], {"effet": "autoriser", "directives_appliquees": [],
                                         "validation_humaine": "aucune"})
        self.assertEqual(o["valeurs_coffre"]["dossier"], "DOS-FICTIF-003")

    def test_couvert_par_d009_pendant_la_session(self):
        self.assertNotIn(AVEC_D009, self.o)

    def test_rien_sans_base_interne(self):
        self.assertNotIn(CONSIGNE_SANS, self.o)
        self.assertEqual(set(self.o), {NOMINAL, TOP, SANS_D009})

    def test_base_interne_a_jour_rien(self):
        cat = det.lire_catalogue()
        with tempfile.TemporaryDirectory() as racine:
            p = cat["taux"]["interne"]["periode"]
            os.makedirs(os.path.join(racine, os.path.dirname(p["fichier"])))
            with open(os.path.join(racine, p["fichier"]), "w") as f:
                json.dump({p["champ"]: "2026-Q4"}, f)
            occ = det.source_interne_perimee(journaux.lire_sessions(SESSIONS), cat, racine)
        self.assertEqual(occ, [])


class TestEntrepriseNonRetrouvee(SessionsEcrites):
    def test_sirene_introuvable(self):
        occ, nv = detecter()
        o = {x["session"]: x for x in occ[det.ENTREPRISE_NON_RETROUVEE]}
        self.assertEqual(set(o), {CONSIGNE_SANS, CONSIGNE_AVEC})
        x = o[CONSIGNE_SANS]
        self.assertEqual(x["motif"], "introuvable")
        self.assertEqual(x["valeurs_coffre"], {"siren": "000000000", "dossier": "DOS-FICTIF-004"})
        self.assertEqual(x["recherches"][0]["seq"], 2)
        self.assertEqual(x["recherches"][0]["nombre_resultats"], 0)
        self.assertFalse(x["transmise"])
        self.assertEqual(x["decision"]["directives_appliquees"], ["D-002"])
        self.assertEqual(nv[det.ENTREPRISE_NON_RETROUVEE], [])

    def test_sirene_jamais_recherche(self):
        s = self.session("s-non-recherche", [
            appel(1, "banque-synthetique", "attribuer_pret", acces="action"),
        ], {"e1": {"siren": "123456789", "montant": 1000}})
        occ, _ = detecter([s])
        (o,) = occ[det.ENTREPRISE_NON_RETROUVEE]
        self.assertEqual(o["motif"], "non_recherchee")

    def test_reponse_non_relisible_pas_de_constat(self):
        s = self.session("s-inconnu", [
            appel(1, "recherche-entreprises", "rechercher_entreprise",
                  source={"origine": "direct", "date": "2030-01-01T00:00:00Z"}),
            appel(2, "banque-synthetique", "attribuer_pret", acces="action"),
        ], {"e1": {"siren": "123456789"}, "e2": {"siren": "123456789"}})
        occ, nv = detecter([s])
        self.assertEqual(occ[det.ENTREPRISE_NON_RETROUVEE], [])
        self.assertEqual(nv[det.ENTREPRISE_NON_RETROUVEE], [{"session": "s-inconnu", "seq_action": 2}])

    def test_trouvee_rien(self):
        occ, _ = detecter()
        self.assertNotIn(NOMINAL, [o["session"] for o in occ[det.ENTREPRISE_NON_RETROUVEE]])


class TestMontant(SessionsEcrites):
    def test_sans_controle(self):
        occ, _ = detecter()
        (o,) = occ[det.ACTION_SANS_CONTROLE]
        self.assertEqual(o["session"], SANS_D009)
        self.assertEqual(o["valeurs_coffre"]["montant"], 150000)

    def test_seuil(self):
        occ, _ = detecter(seuil_montant=200000)
        self.assertEqual(occ[det.ACTION_SANS_CONTROLE], [])

    def test_validee_ou_retenue_rien(self):
        s = self.session("s", [
            appel(1, "banque-synthetique", "attribuer_pret", acces="action", effet="attendre_validation",
                  applications=[{"id": "D-002", "appliquee": True}],
                  approbation={"statut": "acceptee", "roles": ["analyste-risques"]}),
        ], {"e1": {"montant": 900000}})
        occ, _ = detecter([s])
        self.assertEqual(occ[det.ACTION_SANS_CONTROLE], [])


class TestDonneesPersonnelles(SessionsEcrites):
    def test_vers_externe(self):
        s = self.session("s-rh", [
            appel(1, "rh-synthetique", "consulter_salarie", etiquettes=["donnees_personnelles"]),
            appel(2, "messagerie", "envoyer", acces="action", etiquettes=["externe"]),
        ], {"e1": {"salarie_id": "SAL-FICTIF-001"}, "e2": {"a": "x"}})
        occ, _ = detecter([s])
        (o,) = occ[det.DONNEES_PERSONNELLES_EXTERNE]
        self.assertEqual(o["consultations_personnelles"], [{"seq": 1, "outil": "rh-synthetique/consulter_salarie"}])

    def test_ordre_et_destination(self):
        s = self.session("s-rh", [
            appel(1, "messagerie", "envoyer", acces="action", etiquettes=["externe"]),
            appel(2, "rh-synthetique", "consulter_salarie", etiquettes=["donnees_personnelles"]),
            appel(3, "interne", "ecrire", acces="action", etiquettes=[]),
        ], {})
        occ, _ = detecter([s])
        self.assertEqual(occ[det.DONNEES_PERSONNELLES_EXTERNE], [])

    def test_aucune_dans_les_passages(self):
        occ, _ = detecter()
        self.assertEqual(occ[det.DONNEES_PERSONNELLES_EXTERNE], [])


class TestRegroupement(unittest.TestCase):
    def test_un_constat_par_risque(self):
        occ, _ = detecter()
        cs, couverts = propositions.regrouper(occ, det.lire_catalogue(), couverture.lire_jeu(JEU))
        self.assertEqual([c["id"] for c in cs], [
            "A-source-interne-perimee-banque-synthetique.attribuer_pret",
            "A-action-sans-controle-banque-synthetique.attribuer_pret",
            "A-entreprise-non-retrouvee-banque-synthetique.attribuer_pret"])
        self.assertEqual(couverts, [])
        self.assertEqual(cs[0]["sessions"], [NOMINAL, TOP, SANS_D009])
        self.assertIsInstance(cs[0]["detail"], str)  # la console lit une chaîne
        self.assertIn("DOS-FICTIF-002", cs[0]["detail"])

    def test_couvert_par_le_jeu_courant(self):
        jeu = couverture.lire_jeu(os.path.join(det.RACINE, "demo", "scenarios", "jeux", "avec-d009.json"))
        occ, _ = det.detecter(journaux.lire_sessions(SESSIONS), jeu_courant=jeu)
        cs, couverts = propositions.regrouper(occ, det.lire_catalogue(), jeu)
        self.assertIn({"id": "A-source-interne-perimee-banque-synthetique.attribuer_pret", "directives": ["D-009"]},
                      couverts)
        self.assertNotIn("A-source-interne-perimee-banque-synthetique.attribuer_pret", [c["id"] for c in cs])

    def test_seuil_existant_trop_haut_ne_couvre_pas(self):
        # D-005 (/montant > 2 000 000) vise attribuer_pret mais ne retient
        # pas 150 000 €.
        occ, _ = detecter()
        cs, _ = propositions.regrouper(occ, det.lire_catalogue(), couverture.lire_jeu(JEU))
        self.assertIn("A-action-sans-controle-banque-synthetique.attribuer_pret", [c["id"] for c in cs])


if __name__ == "__main__":
    unittest.main()
