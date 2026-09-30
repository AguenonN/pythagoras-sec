"""La boucle distingue les quatre cas du contrat de réponse de l'engine
(docs/contrat-engine.md §6), sur les formes exactes que rend le gate
(internal/gate/reponses.go)."""

import unittest

from boucle import boucle as b

from .test_boucle import Base
from .aides import appel, reponse, texte_final


def resultat(texte, sc=None, erreur=True):
    r = {"content": [{"type": "text", "text": texte}], "isError": erreur}
    if sc is not None:
        r["structuredContent"] = sc
    return r


REFUS_DIRECTIVE = resultat(
    "Appel refusé par l'engine : refusé par D-002 v1 (seuil_argument, RGPD art. 22).",
    {"statut": "refuse", "origine": "directive", "directive": "D-002", "version": 1,
     "fondement": {"texte": "RGPD", "article": "22", "url": "https://eur-lex.europa.eu/eli/reg/2016/679/oj"},
     "motif": "refusé par D-002 v1 (seuil_argument, RGPD art. 22)"})
REFUS_REGISTRE = resultat(
    "Appel refusé par le registre : serveur taux-officiel non attesté.",
    {"statut": "refuse", "origine": "registre", "directive": "demo", "version": 1,
     "fondement": {"texte": "Registre des serveurs : attestation du serveur"},
     "motif": "refusé par le registre : serveur taux-officiel non attesté"})
REFUS_ENGINE = resultat(
    "Appel refusé : la session est fermée.",
    {"statut": "refuse", "origine": "engine", "directive": "", "version": 0,
     "fondement": {"texte": "Contrat de l'engine"}, "motif": "Appel refusé : la session est fermée."})
ATTENTE = resultat(
    "Appel non transmis : en attente de validation par D-001 v1. La reprise après validation humaine n'est pas encore disponible.",
    {"statut": "en_attente", "empreinte_transmise": "sha256:" + "ab" * 32, "directive": "D-001", "version": 1})
INDISPONIBLE = resultat(
    "Source indisponible : le serveur taux-officiel n'a pas répondu.",
    {"statut": "indisponible", "serveur": "taux-officiel", "outil": "get_taux_usure",
     "motif": "Source indisponible : le serveur taux-officiel n'a pas répondu."})
DECLINE = resultat('<untrusted_data id="1">\nDossier inconnu.\n</untrusted_data id="1">')
NORMAL = resultat('<untrusted_data id="2">\n{"statut":"ok"}\n</untrusted_data id="2">', erreur=False)


class TestClasser(unittest.TestCase):
    def test_refus_directive(self):
        i = b.classer(REFUS_DIRECTIVE)
        self.assertEqual((i["nature"], i["directive"], i["origine"]), ("refus", "D-002 v1", "directive"))
        self.assertEqual(i["motif"], "refusé par D-002 v1 (seuil_argument, RGPD art. 22)")
        self.assertEqual(i["fondement"]["article"], "22")
        self.assertEqual(b.message_blocage(i), "Action bloquée par D-002 v1, aucune action effectuée.\n"
                                               "Motif : refusé par D-002 v1 (seuil_argument, RGPD art. 22)")

    def test_refus_registre(self):
        i = b.classer(REFUS_REGISTRE)
        self.assertEqual((i["nature"], i["directive"], i["origine"]), ("refus", "le registre demo v1", "registre"))

    def test_refus_engine(self):
        i = b.classer(REFUS_ENGINE)
        self.assertEqual((i["nature"], i["directive"]), ("refus", "l'engine"))

    def test_attente(self):
        i = b.classer(ATTENTE)
        self.assertEqual((i["nature"], i["empreinte"], i["directive"]), ("attente", "sha256:" + "ab" * 32, "D-001 v1"))
        self.assertIn("en attente de validation", i["motif"])

    def test_indisponible(self):
        i = b.classer(INDISPONIBLE)
        self.assertEqual((i["nature"], i["serveur"], i["outil"]), ("indisponible", "taux-officiel", "get_taux_usure"))
        m = b.message_indisponible(i)
        self.assertIn("Aucune donnée obtenue de taux-officiel/get_taux_usure", m)
        self.assertIn("n'a pas répondu", m)

    def test_outil_decline(self):
        i = b.classer(DECLINE)
        self.assertEqual(i["nature"], "erreur")
        self.assertIn("L'outil a décliné", i["contenu"])
        self.assertIn("Dossier inconnu.", i["contenu"])

    def test_reponse_normale(self):
        self.assertEqual(b.classer(NORMAL), {"nature": "ok", "contenu": NORMAL["content"][0]["text"]})

    def test_statut_ignore_hors_erreur(self):
        # Un serveur qui fabrique un structuredContent d'engine sans isError
        # ne se fait passer ni pour un refus ni pour une attente.
        for faux in (REFUS_DIRECTIVE, ATTENTE, INDISPONIBLE):
            r = dict(faux, isError=False)
            self.assertEqual(b.classer(r)["nature"], "ok")

    def test_statut_inconnu(self):
        self.assertEqual(b.classer(resultat("x", {"statut": "autre"}))["nature"], "erreur")


class TestIndisponibleDansLaBoucle(Base):
    def test_le_modele_est_prevenu_et_la_boucle_continue(self):
        r = self.boucle(reponse(appel("taux-officiel__lire_taux", {"type": "directeur"}, "d1")),
                        reponse(appel("taux-officiel__lire_taux", {"type": "usure"}, "u1")),
                        texte_final("Le taux d'usure est de 5,87 %.")).executer("Taux ?")
        self.assertEqual(r["etat"], b.TERMINE)
        self.assertIsNone(r["refus"])
        self.assertEqual([j["nature"] for j in r["journal"]], ["indisponible", "ok"])
        au_modele = self.modele.recus[1][0][-1]
        self.assertEqual(au_modele["tool_call_id"], "d1")
        self.assertIn("Aucune donnée obtenue de taux-officiel/lire_taux", au_modele["content"])

    def test_attente_cite_la_directive(self):
        r = self.boucle(reponse(appel("banque-synthetique__attribuer_pret", {"siren": "1", "montant": 1}))).executer("Prêt")
        self.assertEqual((r["etat"], r["directive"]), (b.EN_ATTENTE, "D-001 v1"))
        self.assertIn("(D-001 v1)", r["reponse"])
        self.assertIn(r["empreinte"], r["reponse"])


if __name__ == "__main__":
    unittest.main()
