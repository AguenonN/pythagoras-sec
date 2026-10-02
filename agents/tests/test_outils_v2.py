"""Outils offerts à Mistral en v2 : aucun schéma récursif, tout nom fermé
borné par une énumération tirée du vocabulaire (catalogue généré, complément,
fuseaux du chargeur Go)."""

import json
import os
import unittest

from compilateur import arbre, directive_go, outils_v2, vocabulaire

from .aides import CATALOGUE_DEMO, EXEMPLES

VOCABULAIRE_DEMO = os.path.join(EXEMPLES, "vocabulaire-demo.json")


def vocab_demo():
    return vocabulaire.Vocabulaire.lire(CATALOGUE_DEMO, VOCABULAIRE_DEMO)


def profondeur(schema, d=0):
    if isinstance(schema, dict):
        return max([d] + [profondeur(v, d + 1) for v in schema.values()])
    if isinstance(schema, list):
        return max([d] + [profondeur(v, d + 1) for v in schema])
    return d


class TestOutilsV2(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.vocab = vocab_demo()
        cls.outils = {o["function"]["name"]: o["function"]["parameters"] for o in outils_v2.outils(cls.vocab)}

    def test_trois_outils(self):
        self.assertEqual(sorted(self.outils), ["poser_question", "proposer_jeu", "signaler_non_observable"])

    def test_aucune_recursion(self):
        texte = json.dumps(self.outils)
        for interdit in ("$ref", "$defs", "definitions", "anyOf", "oneOf", "allOf"):
            self.assertNotIn(interdit, texte)
        # Profondeur finie et petite : directive → noeuds → nœud → propriété.
        self.assertLess(profondeur(self.outils["proposer_jeu"]), 16)

    def props(self, *chemin):
        s = self.outils["proposer_jeu"]["properties"]["directives"]["items"]
        for c in chemin:
            s = s["properties"][c] if "properties" in s else s["items"]["properties"][c]
        return s

    def test_enumerations_du_vocabulaire(self):
        v, cat = self.vocab, self.vocab.catalogue
        noeud = self.props("noeuds")["items"]["properties"]
        self.assertEqual(noeud["type"]["enum"], arbre.TYPES)
        self.assertEqual(noeud["op"]["enum"], outils_v2.OPERATEURS)
        self.assertEqual(noeud["serveur"]["enum"], cat.noms_serveurs())
        self.assertEqual(noeud["outil"]["enum"], cat.outils_qualifies())
        self.assertEqual(noeud["champ"]["enum"], cat.arguments() + [""])
        self.assertEqual(noeud["etiquette"]["enum"], v.etiquettes)
        self.assertEqual(noeud["groupe"]["enum"], v.groupes)
        self.assertEqual(noeud["fuseau"]["enum"], directive_go.fuseaux())
        self.assertEqual(noeud["jours"]["items"]["enum"], vocabulaire.JOURS)
        effet = self.props("effets")["items"]["properties"]
        self.assertEqual(effet["effet"]["enum"], ["refuser", "attendre_validation", "caviarder", "notifier", "marquer"])
        self.assertEqual(effet["par_role"]["enum"], v.roles)
        self.assertEqual(effet["destinataire_role"]["enum"], v.roles)
        self.assertEqual(effet["etiquette"]["enum"], v.etiquettes)
        self.assertEqual(effet["champs_autorises"]["items"]["enum"], cat.arguments())
        dc = self.props("declencheur")["properties"]
        self.assertEqual(dc["serveur"]["enum"], cat.noms_serveurs() + ["*"])
        self.assertEqual(dc["outil"]["enum"], cat.noms_outils() + ["*"])
        self.assertEqual(dc["acces"]["enum"], ["action", "read"])

    def test_ce_que_mistral_ne_fixe_jamais(self):
        cles = set()

        def collecter(s):
            if isinstance(s, dict):
                cles.update((s.get("properties") or {}).keys())
                for v in s.values():
                    collecter(v)
            elif isinstance(s, list):
                for v in s:
                    collecter(v)
        collecter(self.outils["proposer_jeu"])
        for cle in ("statut", "version", "texte_fr", "date_effet", "si_indetermine", "si_indetermine_valide_par",
                    "rythme", "si_echec", "evenement"):
            self.assertNotIn(cle, cles)

    def test_types_de_noeuds_du_schema(self):
        # Les types offerts sont exactement les nœuds et feuilles du schéma v2.
        self.assertEqual(set(arbre.TYPES), {"et", "ou", "non", "existe", "tous", "argument", "a_consulte", "appelant",
                                            "sous_agent_declare", "demandeur", "attestation", "temps",
                                            "etiquette_cible", "metadonnee_reponse", "sujet"})


class TestVocabulaire(unittest.TestCase):
    def test_complement_et_fuseaux(self):
        v = vocab_demo()
        self.assertIn("donnees_client", v.etiquettes)
        self.assertIn("production_rapport", v.etiquettes)
        self.assertIn("directeur-risques", v.roles)
        self.assertIn("salaries-proteges", v.groupes)
        self.assertIn("Europe/Paris", v.fuseaux)
        self.assertEqual(v.fuseaux, sorted(v.fuseaux))

    def test_outils_vises(self):
        v = vocab_demo()
        self.assertEqual(v.outils_vises({"serveur": "rh-synthetique", "acces": "action"}),
                         [("rh-synthetique", n) for n in (
                             "attribuer_prime", "convoquer_entretien_prealable", "decider_prime", "envoyer_message_salarie",
                             "exporter_paie", "licencier", "proposer_sanction", "sanctionner")])
        self.assertEqual(v.outils_vises({"serveur": "banque-synthetique", "outil": "attribuer_pret"}),
                         [("banque-synthetique", "attribuer_pret")])

    def test_complement_invalide(self):
        from compilateur.catalogue import CatalogueInvalide
        with open(CATALOGUE_DEMO, encoding="utf-8") as f:
            cat = json.load(f)
        with self.assertRaises(CatalogueInvalide):
            vocabulaire.Vocabulaire(cat, {"roles": [{"nom": "a"}, {"nom": "a"}]}, ["UTC"])
        with self.assertRaises(CatalogueInvalide):
            vocabulaire.Vocabulaire(cat, {}, [])
