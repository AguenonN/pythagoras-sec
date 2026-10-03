"""Détecteur « contournement d'un caviardage » sur une session construite à
la main (forme des entrées du journal du gate, scénario D-023 des secteurs :
salaires retirés de la liste, puis lus fiche par fiche), et proposition
rédigée (réponse enregistrée)."""

import json
import os
import shutil
import tempfile
import unittest

from analyste import __main__ as cli
from analyste import couverture, detecteurs as det, journaux, plongements, propositions, redaction

from .test_detecteurs import JEU, SessionsEcrites, appel
from .annuaire_fige import figer

ICI = os.path.dirname(os.path.abspath(__file__))
CONSTRUITES = os.path.join(ICI, "donnees", "sessions-construites")
NOM = "20261002-180000-contournement-d023-construit"
ID = "A-contournement-retrait-rh-synthetique.consulter_fiche"
LISTE = ["donnees_personnelles", "salaire"]
FICHE = ["appartenance_syndicale", "donnees_personnelles", "donnees_sante", "salaire"]
D023 = {"id": "D-023", "appliquee": True, "effet": "caviarder", "champs_retires": ["/inclure_salaires"]}


def detecter(sessions, jeu=None):
    occ, _ = det.detecter(sessions, jeu_courant=jeu or couverture.lire_jeu(JEU))
    return occ[det.CONTOURNEMENT_RETRAIT]



def setUpModule():
    globals()["_fige"] = figer()


def tearDownModule():
    _fige.stop()

class TestSessionConstruite(unittest.TestCase):
    def test_contournement_trouve(self):
        (o,) = detecter(journaux.lire_sessions(CONSTRUITES))
        self.assertEqual(o["session"], NOM)
        self.assertEqual(o["action"], "rh-synthetique/consulter_fiche")
        self.assertEqual(o["retrait"], {"seq": 1, "outil": "rh-synthetique/lister_salaries", "directives": ["D-023"],
                                           "champs_retires": ["/inclure_salaires"], "etiquettes_propres": ["salaire"]})
        self.assertEqual([c["seq"] for c in o["contournements"]], [2, 3, 4])
        self.assertEqual(o["contournements"][0]["etiquettes_communes"], ["salaire"])

    def test_etiquette_de_serveur_ne_suffit_pas(self):
        # consulter_salarie ne partage que donnees_personnelles, étiquette
        # de tout rh-synthetique : pas un contournement.
        actions = [o["action"] for o in detecter(journaux.lire_sessions(CONSTRUITES))]
        self.assertNotIn("rh-synthetique/consulter_salarie", actions)

    def test_proposition_enregistree(self):
        os.environ.pop("MISTRAL_API_KEY", None)
        d = tempfile.mkdtemp()
        try:
            r = cli.proposer(CONSTRUITES, JEU, d, redaction.client("rejeu"), plongeur=plongements.Plongeur("rejeu"))
        finally:
            shutil.rmtree(d)
        self.assertEqual(r["erreurs"], [])
        (p,) = r["ecrites"]
        self.assertEqual(p["id"], ID)
        self.assertIn("rh-synthetique/lister_salaries", p["texte_propose"])
        self.assertIn("responsable-rh", p["texte_propose"])
        self.assertEqual(p["sessions_similaires"], [])  # aucune autre session dans ce dossier
        self.assertEqual(len(p["fondements_suggeres"]), 2)
        with open(os.path.join(os.path.dirname(ICI), "exemples", "propositions", ID + ".json"), encoding="utf-8") as f:
            self.assertEqual(p["texte_propose"], json.load(f)["texte_propose"])

    def test_contexte_cite_la_directive_contournee(self):
        res = cli.constats(CONSTRUITES, JEU)
        (c,) = [c for c in res["constats"] if c["id"] == ID]
        (d,) = c["contexte"]["directives_contournees"]
        self.assertEqual(d["id"], "D-023")
        self.assertEqual(d["effets"], ["retirer de l'appel les champs autres que /service"])
        self.assertEqual(c["contexte"]["roles_des_directives_du_meme_serveur"], ["responsable-rh"])


class TestCas(SessionsEcrites):
    def test_meme_outil_pas_un_contournement(self):
        s = self.session("s", [
            appel(1, "rh-synthetique", "lister_salaries", etiquettes=LISTE, effet="caviarder", applications=[D023]),
            appel(2, "rh-synthetique", "lister_salaries", etiquettes=LISTE),
        ], {})
        self.assertEqual(detecter([s]), [])

    def test_suite_caviardee_ou_refusee(self):
        s = self.session("s", [
            appel(1, "rh-synthetique", "lister_salaries", etiquettes=LISTE, effet="caviarder", applications=[D023]),
            appel(2, "rh-synthetique", "consulter_fiche", etiquettes=FICHE, effet="caviarder",
                  applications=[dict(D023, id="D-099", champs_retires=["/salaire"])]),
            appel(3, "rh-synthetique", "exporter_paie", etiquettes=["salaire"], effet="refuser", statut="non_transmis"),
        ], {})
        self.assertEqual(detecter([s]), [])

    def test_avant_le_caviardage_rien(self):
        s = self.session("s", [
            appel(1, "rh-synthetique", "consulter_fiche", etiquettes=FICHE),
            appel(2, "rh-synthetique", "lister_salaries", etiquettes=LISTE, effet="caviarder", applications=[D023]),
        ], {})
        self.assertEqual(detecter([s]), [])

    def test_couvert_aujourdhui_par_un_caviardage_de_la_fiche(self):
        jeu = couverture.lire_jeu(JEU)
        jeu = dict(jeu, directives=jeu["directives"] + [{
            "id": "D-099", "statut": "active", "declencheur": {"evenement": "appel_outil", "cible": {
                "serveur": "rh-synthetique", "outil": "consulter_fiche"}},
            "effets": [{"effet": "caviarder", "champs_autorises": ["/salarie_id"], "mode": "caviarder"}]}])
        occ, _ = det.detecter(journaux.lire_sessions(CONSTRUITES), jeu_courant=jeu)
        cs, couverts = propositions.regrouper(occ, det.lire_catalogue(), jeu)
        self.assertIn({"id": ID, "directives": ["D-099"]}, couverts)
        self.assertNotIn(ID, [c["id"] for c in cs])


if __name__ == "__main__":
    unittest.main()


SECTEURS = os.path.join(ICI, "donnees", "sessions-secteurs")
PIEGE = "20261003-130000-secteur-d023-piege-rejeu"


class TestJournauxDeSecteurs(unittest.TestCase):
    """Journaux réels de la session secteurs (rejeu hors ligne des
    scénarios D-023 enregistrés) : le piège donne un constat, la variante
    fermée et la propre n'en donnent aucun."""

    def test_un_constat_sur_le_piege_seulement(self):
        (o,) = detecter(journaux.lire_sessions(SECTEURS))
        self.assertEqual(o["session"], PIEGE)
        self.assertEqual(o["action"], "rh-synthetique/consulter_fiche")
        self.assertEqual(o["retrait"]["directives"], ["D-023"])
        self.assertEqual(o["retrait"]["champs_retires"], ["/inclure_salaires"])
