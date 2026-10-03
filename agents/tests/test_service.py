"""Le service du compilateur (contrats §4) sur un client scripté : requête et
réponse JSON, état de conversation sur disque, aucun appel refait au rappel,
jeu candidat version n+1 du jeu de démo (contrats §1)."""

import io
import json
import os
import shutil
import tempfile
import unittest

from compilateur import compilateur_v2 as c2
from compilateur import directive_go, service

from .aides import ClientScripte, appel, reponse
from .test_outils_v2 import VOCABULAIRE_DEMO

RACINE = directive_go.RACINE
DEMO = "demo/directives/demo.json"
CLES_REPONSE = ["conversation", "etat", "questions", "non_observable", "relecture", "jeu_candidat", "erreurs"]

TEXTE = "Désormais, aucune action d'agent le week-end, heure de Paris."
FONDEMENT = "Règlement intérieur, article 12"


def proposition_weekend():
    return reponse(appel("proposer_jeu", {"directives": [{
        "extrait": TEXTE, "fondements": [{"texte": "Règlement intérieur", "article": "12"}],
        "declencheur": {"serveur": "*", "outil": "*", "acces": "action"},
        "racine": "n1", "noeuds": [{"id": "n1", "type": "temps", "jours": ["samedi", "dimanche"],
                                    "fuseau": "Europe/Paris"}],
        "effets": [{"effet": "refuser"}]}]}))


def requete(**k):
    r = {"conversation": None, "texte": TEXTE, "fondement": FONDEMENT, "auteur": "juriste@exemple.test",
         "reponses": [], "jeu_existant": DEMO, "date_effet": "2026-10-04T11:05:00+02:00"}
    r.update(k)
    return r


class Interdit:
    """Client qui échoue s'il est appelé : le rappel ne doit rien redemander."""
    modele = "mistral-small-latest"

    def completer(self, *a, **k):
        raise AssertionError("appel Mistral refait")


class Base(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.tmp)

    def service(self, client):
        return service.Service(dossier=self.tmp, client=lambda _m: client, vocabulaire_=VOCABULAIRE_DEMO)

    def canonique(self, d):
        return json.dumps(d, sort_keys=True, ensure_ascii=False, separators=(",", ":"))


class TestParcours(Base):
    def test_week_end_jusqu_au_jeu_candidat(self):
        cl = ClientScripte(proposition_weekend())
        rep, code = self.service(cl).traiter(requete())
        self.assertEqual(code, 0)
        self.assertEqual(list(rep), CLES_REPONSE)
        self.assertEqual(rep["etat"], c2.QUESTIONS, rep["erreurs"])
        self.assertEqual(rep["questions"], [{"id": "si_indetermine", "texte": rep["questions"][0]["texte"],
                                             "options": ["refuser", "ignorer"], "origine": "code"}])
        self.assertIn("D-009", rep["questions"][0]["texte"])
        self.assertIsNone(rep["jeu_candidat"])
        self.assertEqual(rep["relecture"], "")
        # Le fondement saisi à part entre dans la requête à Mistral.
        self.assertIn("Fondement donné par le juriste :\n<<<\n" + FONDEMENT, cl.recus[0][0][1]["content"])

        conv = rep["conversation"]
        rep, code = self.service(Interdit()).traiter(
            {"conversation": conv, "reponses": [{"question": "si_indetermine", "reponse": "refuser"}]})
        self.assertEqual(code, 0)
        self.assertEqual(rep["etat"], c2.JEU, rep["erreurs"])
        self.assertEqual(rep["conversation"], conv)
        with open(os.path.join(RACINE, DEMO), encoding="utf-8") as f:
            v = json.load(f)["jeu"]["version"] + 1
        self.assertTrue(rep["relecture"].startswith("Ajout au jeu demo, version %d : 1 directive ajoutée (D-009)" % v))
        self.assertNotIn("D-008", rep["relecture"])

        chemin = rep["jeu_candidat"]
        self.assertTrue(os.path.isabs(chemin), "dossier hors de la racine : chemin absolu")
        self.assertEqual(os.path.dirname(chemin), os.path.join(self.tmp, conv))
        self.assertEqual(os.path.basename(chemin), "demo.v%d.json" % v)
        with open(chemin, encoding="utf-8") as f:
            cand = json.load(f)
        with open(os.path.join(RACINE, DEMO), encoding="utf-8") as f:
            demo = json.load(f)
        self.assertEqual(cand["jeu"], {"id": "demo", "version": demo["jeu"]["version"] + 1,
                                       "date_effet": "2026-10-04T11:05:00+02:00", "auteur": "juriste@exemple.test"})
        n = len(demo["directives"])
        self.assertEqual([self.canonique(d) for d in cand["directives"][:n]],
                         [self.canonique(d) for d in demo["directives"]], "directives existantes inchangées")
        self.assertEqual([list(d) for d in cand["directives"][:n]], [list(d) for d in demo["directives"]])
        d9, = cand["directives"][n:]
        self.assertEqual((d9["id"], d9["statut"], d9["texte_fr"]), ("D-009", "brouillon", TEXTE))
        ok, err, _ = directive_go.verifier(cand, exiger_brouillon=False)
        self.assertTrue(ok, err)

        with open(os.path.join(self.tmp, conv, "conversation.json"), encoding="utf-8") as f:
            etat = json.load(f)
        self.assertEqual([t["etat"] for t in etat["tours"]], [c2.QUESTIONS, c2.JEU])
        self.assertEqual([t["appels_nouveaux"] for t in etat["tours"]], [1, 0])

    def test_questions_de_mistral_identifiees(self):
        q = reponse(appel("poser_question", {"question": "Quel fuseau horaire ?", "sujet": "fuseau",
                                             "options": ["Europe/Paris"]}))
        cl = ClientScripte(q, proposition_weekend())
        rep, _ = self.service(cl).traiter(requete(texte="Désormais, aucune action d'agent le week-end."))
        self.assertEqual(rep["etat"], c2.QUESTIONS)
        q0, = rep["questions"]
        self.assertEqual((q0["texte"], q0["options"], q0["origine"]), ("Quel fuseau horaire ?", ["Europe/Paris"], "mistral"))
        self.assertTrue(q0["id"].startswith("m-"))
        rep2, _ = self.service(cl).traiter({"conversation": rep["conversation"],
                                            "reponses": [{"question": q0["id"], "reponse": "L'heure de Paris."}]})
        self.assertIn("Réponse : L'heure de Paris.", cl.recus[1][0][1]["content"])
        self.assertEqual(rep2["etat"], c2.QUESTIONS)
        self.assertEqual([q["id"] for q in rep2["questions"]], ["si_indetermine"])

    def test_non_observable(self):
        no = reponse(appel("signaler_non_observable", {"extrait": "ne pas discriminer",
                                                        "raison": "Une intention ne se constate pas.",
                                                        "question": "Par quel critère observable ?"}))
        rep, code = self.service(ClientScripte(no)).traiter(requete(texte="Les agents ne doivent pas discriminer."))
        self.assertEqual((code, rep["etat"]), (0, c2.NON_OBSERVABLE))
        self.assertEqual(rep["non_observable"], {"extrait": "ne pas discriminer",
                                                 "raison": "Une intention ne se constate pas."})
        self.assertEqual([q["origine"] for q in rep["questions"]], ["mistral"])

    def test_numerotation_premier_numero_libre(self):
        """Premier numéro libre à partir de D-001 (décision de Manu, 02/10)."""
        demo = {"directives": [{"id": "D-%03d" % i} for i in range(1, 9)]}
        self.assertEqual(c2.numeros_suivants(demo, "D", 2), ["D-009", "D-010"])
        demo["directives"].append({"id": "D-012"})
        self.assertEqual(c2.numeros_suivants(demo, "D", 1), ["D-009"])
        self.assertEqual(c2.numeros_suivants(demo, "D", 4), ["D-009", "D-010", "D-011", "D-013"])
        demo["directives"] += [{"id": "D-%03d" % i} for i in range(11, 24)]
        self.assertEqual(c2.numeros_suivants(demo, "D", 3), ["D-009", "D-010", "D-024"])
        self.assertEqual(c2.numeros_suivants({"directives": []}, "D", 1), ["D-001"])
        self.assertEqual(c2.numeros_suivants({"directives": [{"id": "R-0001"}]}, "R", 1), ["R-0002"])


class TestRequetesInvalides(Base):
    def test_champs(self):
        s = self.service(Interdit())
        for req, motif in [(requete(texte=""), "texte manquant"), (requete(auteur=None), "auteur manquant"),
                           (requete(jeu_existant=None), "jeu_existant manquant"),
                           (requete(date_effet="2026-10-04"), "date_effet"),
                           (dict(requete(), inconnu=1), "champs inconnus : inconnu"),
                           ({"conversation": "absente", "reponses": []}, "conversation absente inconnue"),
                           ({"conversation": "../x"}, "identifiant invalide")]:
            rep, code = s.traiter(req)
            self.assertEqual((code, rep["etat"]), (2, c2.ERREURS), req)
            self.assertIn(motif, rep["erreurs"][0])
        self.assertEqual(os.listdir(self.tmp), [], "aucune conversation créée pour une requête invalide")

    def test_reponses_invalides(self):
        rep, _ = self.service(ClientScripte(proposition_weekend())).traiter(requete())
        conv = rep["conversation"]
        s = self.service(Interdit())
        for reponses, motif in [([{"question": "inconnue", "reponse": "x"}], "question inconnue"),
                                ([{"question": "si_indetermine", "reponse": "peut-être"}], "réponse attendue parmi refuser, ignorer"),
                                ([{"question": "si_indetermine", "reponse": " "}], "réponse vide")]:
            r, code = s.traiter({"conversation": conv, "reponses": reponses})
            self.assertEqual((code, r["etat"]), (2, c2.ERREURS))
            self.assertIn(motif, r["erreurs"][0])
        r, code = s.traiter({"conversation": conv, "texte": "Autre texte."})
        self.assertEqual(code, 2)
        self.assertIn("ouvrir une nouvelle conversation", r["erreurs"][0])
        # Le même texte, renvoyé par la console, est admis.
        r, code = s.traiter({"conversation": conv, "texte": TEXTE,
                             "reponses": [{"question": "si_indetermine", "reponse": "refuser"}]})
        self.assertEqual((code, r["etat"]), (0, c2.JEU), r["erreurs"])

    def test_jeu_existant_refuse(self):
        mauvais = os.path.join(self.tmp, "mauvais.json")
        with open(mauvais, "w", encoding="utf-8") as f:
            json.dump({"jeu": {"id": "demo"}, "directives": []}, f)
        rep, code = self.service(Interdit()).traiter(requete(jeu_existant=mauvais))
        self.assertEqual((code, rep["etat"]), (0, c2.ERREURS))
        self.assertIn("refusé par le chargeur", rep["erreurs"][0])

    def test_mistral_injoignable(self):
        from compilateur import mistral
        rep, code = service.Service(dossier=self.tmp, vocabulaire_=VOCABULAIRE_DEMO,
                                    client=lambda m: mistral.Client(mode="rejeu", enregistrements=self.tmp, modele=m)
                                    ).traiter(requete(texte="Un texte jamais enregistré."))
        self.assertEqual((code, rep["etat"]), (0, c2.ERREURS))
        self.assertIn("EnregistrementAbsent", rep["erreurs"][0])


class TestLigneDeCommande(Base):
    def test_json_entree_sortie(self):
        sortie = io.StringIO()
        code = service.principal(["--dossier", self.tmp], entree=io.StringIO("pas du json"), sortie=sortie)
        self.assertEqual(code, 2)
        rep = json.loads(sortie.getvalue())
        self.assertEqual((list(rep), rep["etat"]), (CLES_REPONSE, c2.ERREURS))

    def test_chemin_rendu_relatif_a_la_racine(self):
        self.assertEqual(service.chemin_rendu(os.path.join(RACINE, "demo", "execution", "x.json")),
                         "demo/execution/x.json")
        self.assertEqual(service.chemin_rendu("/ailleurs/x.json"), "/ailleurs/x.json")


if __name__ == "__main__":
    unittest.main()
