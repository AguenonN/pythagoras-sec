"""Arbre de condition à plat : reconstruction, bornes, cycles (arbre.py)."""

import json
import unittest

from compilateur import arbre, directive_go


def n(id_, type_, *enfants, **p):
    d = {"id": id_, "type": type_}
    if enfants:
        d["enfants"] = list(enfants)
    d.update(p)
    return d


def jeu_avec(condition):
    return {"jeu": {"id": "t", "version": 1, "date_effet": "2026-10-04T00:00:00Z", "auteur": "j"},
            "directives": [{"id": "D-001", "version": 1, "statut": "brouillon", "date_effet": "2026-10-04T00:00:00Z",
                            "texte_fr": "t", "fondements": [{"texte": "RGPD"}],
                            "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "banque-synthetique"}},
                            "condition": condition, "effets": [{"effet": "refuser"}]}]}


class TestReconstruction(unittest.TestCase):
    def test_chaque_type_accepte_par_le_chargeur(self):
        noeuds = [
            n("r", "ou", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"),
            n("a", "et", "a1", "a2"),
            n("a1", "argument", champ="/montant", op=">", valeur=500000),
            n("a2", "argument", champ="/siren", op="present", valeur=None),
            n("b", "non", "b1"),
            n("b1", "a_consulte", serveur="gels-avoirs", critere="donnees_recues"),
            n("c", "existe", "c1", champ="/salarie_ids"),
            n("c1", "sujet", groupe="salaries-proteges", champ=""),
            n("d", "tous", "d1", champ="/salarie_ids"),
            n("d1", "argument", champ="", op="prefixe", valeur="SAL-"),
            n("e", "demandeur", groupe="prestataires-externes"),
            n("f", "demandeur", identites=["alice@exemple.test"]),
            n("g", "temps", jours=["samedi", "dimanche"], heure_de="20:00", heure_a="07:00",
              date_op=">=", date_valeur="2026-12-24", fuseau="Europe/Paris"),
            n("h", "etiquette_cible", etiquette="donnees_client"),
            n("i", "attestation", verifiee=False, mode="binaire"),
            n("j", "metadonnee_reponse", serveur="taux-officiel", meta="source.origine", op="==", valeur="direct"),
            n("k", "appelant", prefixe="spiffe://lab/agents/"),
            n("l", "sous_agent_declare", dans=["analyste"]),
        ]
        a, err = arbre.reconstruire(noeuds, "r")
        self.assertEqual(err, [])
        self.assertEqual(a["ou"][0], {"et": [{"argument": {"champ": "/montant", "op": ">", "valeur": 500000}},
                                             {"argument": {"champ": "/siren", "op": "present"}}]})
        self.assertEqual(a["ou"][1], {"non": {"session.a_consulte": {"serveur": "gels-avoirs", "critere": "donnees_recues"}}})
        self.assertEqual(a["ou"][2], {"existe": {"champ": "/salarie_ids", "condition": {"sujet": {"groupe": "salaries-proteges"}}}})
        self.assertEqual(a["ou"][5], {"demandeur": {"identite": {"egal": "alice@exemple.test"}}})
        self.assertEqual(a["ou"][6]["temps"]["heure"], {"de": "20:00", "a": "07:00"})
        ok, erreurs, _ = directive_go.verifier(jeu_avec(a))
        self.assertTrue(ok, erreurs)

    def test_sans_racine_pas_de_condition(self):
        self.assertEqual(arbre.reconstruire([], None), (None, []))
        self.assertEqual(arbre.reconstruire(None, ""), (None, []))
        _, err = arbre.reconstruire([n("a", "etiquette_cible", etiquette="x")], "")
        self.assertIn("sans racine", err[0])

    def test_identifiants_entiers_acceptes(self):
        a, err = arbre.reconstruire([{"id": 1, "type": "non", "enfants": [2]},
                                     {"id": 2, "type": "etiquette_cible", "etiquette": "x"}], 1)
        self.assertEqual(err, [])
        self.assertEqual(a, {"non": {"etiquette_cible": {"contient": "x"}}})

    def test_prefixe_des_erreurs(self):
        _, err = arbre.reconstruire([n("a", "non")], "a", prefixe="directive 2 : ")
        self.assertTrue(err[0].startswith("directive 2 : nœud a (non) : exactement 1 enfant"), err)


class TestRefus(unittest.TestCase):
    def refuse(self, noeuds, racine, attendu):
        a, err = arbre.reconstruire(noeuds, racine)
        self.assertIsNone(a)
        self.assertTrue(any(attendu in e for e in err), err)

    def test_cycle(self):
        self.refuse([n("a", "non", "b"), n("b", "non", "a")], "a", "cycle")
        self.refuse([n("a", "non", "a")], "a", "cycle")

    def test_cycle_hors_racine(self):
        # b et c se citent l'un l'autre ; ni l'un ni l'autre n'est rattaché.
        self.refuse([n("a", "etiquette_cible", etiquette="x"), n("b", "non", "c"), n("c", "non", "b")], "a",
                    "non rattaché")

    def test_deux_parents(self):
        self.refuse([n("r", "et", "a", "b"), n("a", "non", "c"), n("b", "non", "c"),
                     n("c", "etiquette_cible", etiquette="x")], "r", "déjà enfant de")

    def test_orphelin(self):
        self.refuse([n("a", "etiquette_cible", etiquette="x"), n("b", "etiquette_cible", etiquette="y")], "a",
                    "b non rattaché")

    def test_identifiants(self):
        self.refuse([n("a", "etiquette_cible", etiquette="x"), n("a", "etiquette_cible", etiquette="y")], "a", "en double")
        self.refuse([n("a", "non", "z")], "a", "enfant z inconnu")
        self.refuse([n("a", "etiquette_cible", etiquette="x")], "b", "racine b")
        self.refuse([{"type": "et"}], "a", "id manquant")

    def test_profondeur(self):
        chaine = [n("n%d" % i, "non", "n%d" % (i + 1)) for i in range(6)] + [n("n6", "etiquette_cible", etiquette="x")]
        self.refuse(chaine, "n0", "profondeur 7 (au plus 6)")
        a, err = arbre.reconstruire(chaine[1:], "n1")
        self.assertEqual(err, [])

    def test_taille(self):
        feuilles = [n("f%d" % i, "etiquette_cible", etiquette="x") for i in range(64)]
        self.refuse([n("r", "ou", *["f%d" % i for i in range(64)])] + feuilles, "r", "65 nœuds (au plus 64)")

    def test_arite_et_feuilles(self):
        self.refuse([n("a", "et")], "a", "au moins 1 enfant")
        self.refuse([n("a", "non", "b", "c"), n("b", "etiquette_cible", etiquette="x"),
                     n("c", "etiquette_cible", etiquette="y")], "a", "exactement 1 enfant(s) attendu(s), 2 donné(s)")
        self.refuse([n("a", "etiquette_cible", "b", etiquette="x"), n("b", "etiquette_cible", etiquette="y")], "a",
                    "une feuille n'a pas d'enfant")

    def test_parametres(self):
        self.refuse([n("a", "temps", jours=["samedi"])], "a", "paramètre fuseau manquant")
        self.refuse([n("a", "etiquette_cible", etiquette="x", groupe="g")], "a",
                    "le paramètre groupe n'est pas admis pour ce type")
        self.refuse([n("a", "regex", valeur=".*")], "a", "type 'regex' inconnu")


class TestSansRecursion(unittest.TestCase):
    def test_boucle_impossible(self):
        # Un arbre généré par Mistral ne peut pas faire tourner la
        # reconstruction indéfiniment : chaque nœud n'a qu'un parent.
        noeuds = [n("n%d" % i, "non", "n%d" % ((i + 1) % 100)) for i in range(100)]
        a, err = arbre.reconstruire(noeuds, "n0")
        self.assertIsNone(a)
        self.assertTrue(err)
        json.dumps(err)
