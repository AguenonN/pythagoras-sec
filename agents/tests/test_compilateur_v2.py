"""Compilateur v2 sur un client scripté : un texte → un jeu complet en
brouillon, questions, refus motivés, vérifications, questions du code."""

import json
import os
import unittest

from compilateur import compilateur_v2 as c2
from compilateur import directive_go

from .aides import ClientScripte, appel, reponse
from .test_outils_v2 import vocab_demo

CREDIT = ("Politique de crédit. Aucun prêt ne peut être attribué sans consultation préalable du registre des gels "
          "d'avoirs (Code monétaire et financier, art. L562-4). Au-delà de 500 000 euros, l'attribution d'un prêt "
          "est bloquée (Politique de crédit, art. 3.2). Un prêt attribué à partir du taux officiel attend une "
          "validation humaine (RGPD, art. 22). Le serveur de recherche d'entreprises ne reçoit que le SIREN "
          "(RGPD, art. 5).")
PRET = {"serveur": "banque-synthetique", "outil": "attribuer_pret"}


def d_gels():
    return {"extrait": "Aucun prêt ne peut être attribué sans consultation préalable du registre des gels d'avoirs",
            "fondements": [{"texte": "Code monétaire et financier", "article": "L562-4"}], "declencheur": dict(PRET),
            "racine": "n1", "noeuds": [{"id": "n1", "type": "non", "enfants": ["n2"]},
                                       {"id": "n2", "type": "a_consulte", "serveur": "gels-avoirs", "critere": "donnees_recues"}],
            "effets": [{"effet": "refuser"}]}


def d_seuil(valeur=500000):
    return {"extrait": "Au-delà de 500 000 euros, l'attribution d'un prêt est bloquée",
            "fondements": [{"texte": "Politique de crédit", "article": "3.2"}], "declencheur": dict(PRET),
            "racine": "a", "noeuds": [{"id": "a", "type": "argument", "champ": "/montant", "op": ">", "valeur": valeur}],
            "effets": [{"effet": "refuser"}]}


def d_taux():
    return {"extrait": "Un prêt attribué à partir du taux officiel attend une validation humaine",
            "fondements": [{"texte": "RGPD", "article": "22"}], "declencheur": dict(PRET),
            "racine": "a", "noeuds": [{"id": "a", "type": "a_consulte", "serveur": "taux-officiel", "critere": "donnees_recues"}],
            "effets": [{"effet": "attendre_validation"}]}


def d_siren():
    return {"extrait": "Le serveur de recherche d'entreprises ne reçoit que le SIREN",
            "fondements": [{"texte": "RGPD", "article": "5"}],
            "declencheur": {"serveur": "recherche-entreprises", "outil": "*"}, "racine": "", "noeuds": [],
            "effets": [{"effet": "caviarder", "champs_autorises": ["/siren"], "mode": "caviarder"}]}


def jeu(*directives):
    return reponse(appel("proposer_jeu", {"directives": list(directives)}))


def repondre(questions, valeur):
    return [dict(q, reponse=valeur) for q in questions]


class Base(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.vocab = vocab_demo()

    def compiler(self, texte, client, reponses=()):
        return c2.compiler_jeu(texte, "essai", self.vocab, client, "juriste@cabinet.fr",
                               "2026-10-04T00:00:00Z", reponses=reponses)


class TestUnTexteUnJeu(Base):
    def test_plusieurs_directives_toutes_en_brouillon(self):
        propo = jeu(d_gels(), d_seuil(), d_taux(), d_siren())
        r = self.compiler(CREDIT, ClientScripte(propo))
        self.assertEqual(r["etat"], c2.QUESTIONS)
        self.assertEqual([q["champ"] for q in r["questions"]], ["si_indetermine"])
        self.assertIn("D-001, D-002, D-003", r["questions"][0]["question"])
        self.assertNotIn("D-004", r["questions"][0]["question"], "sans condition, rien d'indéterminé")
        self.assertIsNone(r["jeu"])

        r = self.compiler(CREDIT, ClientScripte(propo), repondre(r["questions"], "refuser"))
        self.assertEqual(r["etat"], c2.JEU, r["erreurs"])
        ds = r["jeu"]["directives"]
        self.assertEqual([d["id"] for d in ds], ["D-001", "D-002", "D-003", "D-004"])
        self.assertTrue(all(d["statut"] == "brouillon" for d in ds))
        self.assertEqual(ds[0]["texte_fr"], d_gels()["extrait"])
        self.assertEqual(ds[0]["condition"], {"non": {"session.a_consulte": {"serveur": "gels-avoirs", "critere": "donnees_recues"}}})
        self.assertEqual(ds[1]["si_indetermine"], "refuser")
        self.assertNotIn("si_indetermine", ds[3])
        self.assertEqual(ds[3]["effets"], [{"effet": "caviarder", "champs_autorises": ["/siren"], "mode": "caviarder"}])
        self.assertEqual(r["consignation"][0]["champ"], "si_indetermine")
        ok, err, _ = directive_go.verifier(r["jeu"])
        self.assertTrue(ok, err)
        self.assertIn("Directive D-004", r["relecture"])
        self.assertIn("Si la session n'a reçu aucune donnée du serveur gels-avoirs", r["relecture"])

    def test_texte_fr_intact(self):
        p = d_taux()
        p["extrait"] = "un prêt attribué à partir du taux officiel"  # pas mot pour mot (casse)
        r = self.compiler(CREDIT, ClientScripte(jeu(p)), [{"question": "q", "reponse": "refuser",
                                                           "champ": "si_indetermine", "origine": "compilateur"}])
        self.assertEqual(r["etat"], c2.JEU, r["erreurs"])
        self.assertEqual(r["jeu"]["directives"][0]["texte_fr"], CREDIT)

    def test_ignorer_valide_par_le_juriste(self):
        r = self.compiler(CREDIT, ClientScripte(jeu(d_seuil())), [{"question": "q", "reponse": "Ignorer.",
                                                                    "champ": "si_indetermine", "origine": "compilateur"}])
        d = r["jeu"]["directives"][0]
        self.assertEqual((d["si_indetermine"], d["si_indetermine_valide_par"]), ("ignorer", "juriste@cabinet.fr"))
        self.assertEqual(r["consignation"][0]["reponse"], "Ignorer.")

    def test_reponses_du_code_jamais_envoyees_a_mistral(self):
        cl = ClientScripte(jeu(d_seuil()))
        self.compiler(CREDIT, cl, [{"question": "QUESTION-DU-CODE", "reponse": "REPONSE-DU-CODE",
                                    "champ": "si_indetermine", "origine": "compilateur"}])
        self.assertNotIn("REPONSE-DU-CODE", json.dumps(cl.recus, ensure_ascii=False))

    def test_valeurs_normalisees(self):
        r = self.compiler(CREDIT, ClientScripte(jeu(d_seuil("500000"))),
                          [{"question": "q", "reponse": "refuser", "champ": "si_indetermine", "origine": "compilateur"}])
        self.assertEqual(r["etat"], c2.JEU, r["erreurs"])
        self.assertEqual(r["jeu"]["directives"][0]["condition"]["argument"]["valeur"], 500000)
        r = self.compiler(CREDIT, ClientScripte(jeu(d_seuil(500000.0))),
                          [{"question": "q", "reponse": "refuser", "champ": "si_indetermine", "origine": "compilateur"}])
        self.assertEqual(r["jeu"]["directives"][0]["condition"]["argument"]["valeur"], 500000)


class TestQuestionsEtRefus(Base):
    def test_question_l_emporte(self):
        q = appel("poser_question", {"question": "Quel fuseau ?", "sujet": "fuseau", "extrait": "le week-end",
                                     "options": ["Europe/Paris", "UTC"]}, "q1")
        r = self.compiler("Aucune action le week-end.", ClientScripte(reponse(q, appel("proposer_jeu", {"directives": [d_seuil()]}, "p1"))))
        self.assertEqual(r["etat"], c2.QUESTIONS)
        self.assertEqual(r["questions"], [{"question": "Quel fuseau ?", "sujet": "fuseau", "extrait": "le week-end",
                                           "options": ["Europe/Paris", "UTC"]}])

    def test_non_observable_refuse_et_demande(self):
        s = appel("signaler_non_observable", {"extrait": "ne pas discriminer",
                                              "raison": "Aucune feuille ne constate une discrimination dans un appel.",
                                              "question": "Par quel critère observable ?"})
        r = self.compiler("L'agent ne doit pas discriminer.", ClientScripte(reponse(s)))
        self.assertEqual(r["etat"], c2.NON_OBSERVABLE)
        self.assertIsNone(r["jeu"])
        self.assertEqual(r["non_observables"][0]["extrait"], "ne pas discriminer")
        self.assertEqual(r["questions"][0]["sujet"], "non_observable")
        self.assertIn("discrimination", r["questions"][0]["raison"])

    def test_reponses_envoyees_a_mistral(self):
        cl = ClientScripte(jeu(d_seuil()))
        self.compiler(CREDIT, cl, [{"question": "Quel seuil ?", "reponse": "Au-delà de 500 000 euros."}])
        self.assertIn("Au-delà de 500 000 euros.", cl.recus[0][0][1]["content"])

    def test_sans_outil(self):
        r = self.compiler(CREDIT, ClientScripte({"choices": [{"message": {"content": "bonjour"}}]}))
        self.assertEqual(r["etat"], c2.ERREURS)


class TestVerifications(Base):
    def refuse(self, texte, propo, attendu, reponses=()):
        r = self.compiler(texte, ClientScripte(jeu(propo), jeu(propo)), reponses)
        self.assertEqual(r["etat"], c2.REFUSE, r)
        self.assertIsNone(r["jeu"])
        self.assertTrue(any(attendu in e for e in r["erreurs"]), r["erreurs"])
        return r

    def test_montant_invente(self):
        self.refuse("Au-delà d'un montant significatif, l'attribution d'un prêt est bloquée.", d_seuil(),
                    "la valeur 500000 n'est pas écrite")

    def test_correction_unique(self):
        bon, mauvais = d_seuil(), d_seuil(400000)
        cl = ClientScripte(jeu(mauvais), jeu(bon))
        r = self.compiler(CREDIT, cl, [{"question": "q", "reponse": "refuser", "champ": "si_indetermine", "origine": "compilateur"}])
        self.assertEqual(r["etat"], c2.JEU, r["erreurs"])
        self.assertEqual(r["tentatives"], 2)
        corr = cl.recus[1][0]
        self.assertEqual(corr[-1]["role"], "tool")
        self.assertIn("400000", corr[-1]["content"])

    def test_fuseau_non_ecrit(self):
        p = {"extrait": "x", "fondements": [{"texte": "Règlement intérieur"}],
             "declencheur": {"serveur": "*", "outil": "*", "acces": "action"}, "racine": "t",
             "noeuds": [{"id": "t", "type": "temps", "jours": ["samedi", "dimanche"], "fuseau": "Europe/Paris"}],
             "effets": [{"effet": "refuser"}]}
        texte = "Aucune action d'agent le week-end (Règlement intérieur)."
        self.refuse(texte, p, "le fuseau Europe/Paris n'est pas écrit")
        r = self.compiler(texte, ClientScripte(jeu(p)), [
            {"question": "Quel fuseau ?", "reponse": "Heure de Paris."},
            {"question": "q", "reponse": "refuser", "champ": "si_indetermine", "origine": "compilateur"}])
        self.assertEqual(r["etat"], c2.JEU, r["erreurs"])

    def test_jour_non_ecrit(self):
        p = {"extrait": "x", "fondements": [{"texte": "Règlement intérieur"}],
             "declencheur": {"serveur": "*", "outil": "*"}, "racine": "t",
             "noeuds": [{"id": "t", "type": "temps", "jours": ["vendredi"], "fuseau": "Europe/Paris"}],
             "effets": [{"effet": "refuser"}]}
        self.refuse("Aucune action le week-end, heure de Paris (Règlement intérieur).", p, "le jour vendredi")

    def test_etiquette_et_groupe_non_choisis(self):
        p = {"extrait": "x", "fondements": [{"texte": "RGPD"}], "declencheur": {"etiquette": "production_rapport"},
             "racine": "e", "noeuds": [{"id": "e", "type": "et", "enfants": ["a", "b"]},
                                       {"id": "a", "type": "demandeur", "groupe": "prestataires-externes"},
                                       {"id": "b", "type": "a_consulte", "etiquette": "donnees_client", "critere": "donnees_recues"}],
             "effets": [{"effet": "refuser"}]}
        texte = "Si l'utilisateur est membre du groupe X, toute rédaction de rapport basée sur des données client est bloquée (RGPD)."
        r = self.refuse(texte, p, "etiquette 'production_rapport' n'est ni écrit ni choisi")
        self.assertTrue(any("groupe 'prestataires-externes'" in e for e in r["erreurs"]))
        self.assertTrue(any("etiquette 'donnees_client'" in e for e in r["erreurs"]))
        # Choisis par le juriste dans ses réponses : acceptés.
        r = self.compiler(texte, ClientScripte(jeu(p)), [
            {"question": "Quel groupe ?", "reponse": "prestataires-externes"},
            {"question": "Rapport ?", "reponse": "production_rapport"},
            {"question": "Données client ?", "reponse": "donnees_client"},
            {"question": "q", "reponse": "demandeur", "champ": "designation_personne", "origine": "compilateur"},
            {"question": "q", "reponse": "refuser", "champ": "si_indetermine", "origine": "compilateur"}])
        self.assertEqual(r["etat"], c2.JEU, r["erreurs"])

    def test_role_et_quatre_yeux(self):
        texte = "Au-delà de 2 M€, l'attribution d'un prêt attend une validation (Politique de crédit, art. 7.1)."
        p = {"extrait": "x", "fondements": [{"texte": "Politique de crédit", "article": "7.1"}], "declencheur": dict(PRET),
             "racine": "a", "noeuds": [{"id": "a", "type": "argument", "champ": "/montant", "op": ">", "valeur": 2000000}],
             "effets": [{"effet": "attendre_validation", "par_role": "directeur-risques", "quatre_yeux": True}]}
        r = self.refuse(texte, p, "le rôle 'directeur-risques' n'est ni écrit ni choisi")
        self.assertTrue(any("quatre_yeux" in e for e in r["erreurs"]))

    def test_argument_absent_d_un_outil_vise(self):
        p = {"extrait": "x", "fondements": [{"texte": "Code du travail"}],
             "declencheur": {"serveur": "rh-synthetique", "acces": "action"}, "racine": "q",
             "noeuds": [{"id": "q", "type": "existe", "champ": "/salarie_ids", "enfants": ["s"]},
                        {"id": "s", "type": "sujet", "groupe": "salaries-proteges", "champ": ""}],
             "effets": [{"effet": "refuser"}]}
        self.refuse("Aucune décision RH visant un salarié du groupe salaries-proteges (Code du travail).", p,
                    "l'argument /salarie_ids n'existe pas pour rh-synthetique/proposer_sanction")

    def test_arbre_invalide_rendu_a_mistral(self):
        p = d_seuil()
        p["noeuds"].append({"id": "b", "type": "etiquette_cible", "etiquette": "officiel"})
        self.refuse(CREDIT, p, "nœud(s) b non rattaché(s)")

    def test_chargeur_go_juge(self):
        p = d_siren()
        p["declencheur"] = {"serveur": "*", "outil": "rechercher_entreprise"}
        self.refuse(CREDIT, p, "caviarder exige une cible nommée")

    def test_deux_jeux(self):
        cl = ClientScripte(reponse(appel("proposer_jeu", {"directives": [d_seuil()]}, "a"),
                                   appel("proposer_jeu", {"directives": [d_taux()]}, "b")))
        cl.reponses.append(cl.reponses[0])
        r = self.compiler(CREDIT, cl)
        self.assertEqual(r["etat"], c2.REFUSE)
        self.assertIn("un seul jeu", r["erreurs"][0])


class TestNotifications(Base):
    TEXTE = ("Au-delà de 2 M€, l'attribution d'un prêt attend la validation du directeur-risques en quatre yeux, "
             "et le juridique est notifié (Politique de crédit, art. 7.1).")

    def propo(self):
        return jeu({"extrait": "x", "fondements": [{"texte": "Politique de crédit", "article": "7.1"}],
                    "declencheur": dict(PRET), "racine": "a",
                    "noeuds": [{"id": "a", "type": "argument", "champ": "/montant", "op": ">", "valeur": 2000000}],
                    "effets": [{"effet": "attendre_validation", "par_role": "directeur-risques", "quatre_yeux": True},
                               {"effet": "notifier", "destinataire_role": "juridique"}]})

    def test_rythme_et_echec_demandes_par_le_code(self):
        r = self.compiler(self.TEXTE, ClientScripte(self.propo()))
        self.assertEqual(r["etat"], c2.QUESTIONS, r["erreurs"])
        self.assertEqual([q["champ"] for q in r["questions"]],
                         ["si_indetermine", "rythme:D-001:juridique", "si_echec:D-001:juridique"])
        rep = [dict(r["questions"][0], reponse="refuser"), dict(r["questions"][1], reponse="temps réel"),
               dict(r["questions"][2], reponse="refuser")]
        r = self.compiler(self.TEXTE, ClientScripte(self.propo()), rep)
        self.assertEqual(r["etat"], c2.JEU, r["erreurs"])
        self.assertEqual(r["jeu"]["directives"][0]["effets"][1],
                         {"effet": "notifier", "destinataire_role": "juridique", "rythme": "temps_reel", "si_echec": "refuser"})
        self.assertEqual(len(r["consignation"]), 3)

    def test_reponse_hors_options_redemandee(self):
        r = self.compiler(self.TEXTE, ClientScripte(self.propo()))
        rep = [dict(q, reponse="peut-être") for q in r["questions"]]
        r = self.compiler(self.TEXTE, ClientScripte(self.propo()), rep)
        self.assertEqual(r["etat"], c2.QUESTIONS)
        self.assertEqual(len(r["questions"]), 3)


class TestJustifications(unittest.TestCase):
    def test_noms_fermes(self):
        from compilateur.verifications_v2 import nom_ferme_ecrit
        self.assertTrue(nom_ferme_ecrit("donnees_client", "Rattacher à donnees_client."))
        self.assertTrue(nom_ferme_ecrit("directeur-risques", "Le rôle Directeur-Risques."))
        self.assertTrue(nom_ferme_ecrit("juridique", "le juridique est notifié"))
        self.assertFalse(nom_ferme_ecrit("donnees_client", "des données client"))
        self.assertFalse(nom_ferme_ecrit("directeur-risques", "le directeur des risques"))
        self.assertFalse(nom_ferme_ecrit("dpo", "dpo-adjoint"))

    def test_temps(self):
        from compilateur.verifications_v2 import fuseau_ecrit, heures_ecrites, jours_ecrits, quatre_yeux_ecrit
        self.assertTrue(fuseau_ecrit("Europe/Paris", "Heure de Paris."))
        self.assertTrue(fuseau_ecrit("America/New_York", "heure de New York"))
        self.assertFalse(fuseau_ecrit("Europe/Paris", "le week-end"))
        self.assertEqual(jours_ecrits("le week-end"), {"samedi", "dimanche"})
        self.assertEqual(jours_ecrits("les lundis et le mardi"), {"lundi", "mardi"})
        self.assertEqual(heures_ecrites("de 20 h à 7h30, puis minuit"), {"20:00", "07:30", "00:00"})
        self.assertTrue(quatre_yeux_ecrit("validation en quatre yeux"))
        self.assertTrue(quatre_yeux_ecrit("par deux personnes distinctes"))
        self.assertFalse(quatre_yeux_ecrit("une validation"))


class TestCibleEtCritere(Base):
    def test_acces_superflu(self):
        p = d_seuil()
        p["declencheur"]["acces"] = "action"   # attribuer_pret est déjà classé action
        r = self.compiler(CREDIT, ClientScripte(jeu(p), jeu(p)))
        self.assertEqual(r["etat"], c2.REFUSE)
        self.assertTrue(any("acces action ne restreint rien" in e for e in r["erreurs"]), r["erreurs"])

    def test_acces_qui_restreint_admis(self):
        from compilateur.verifications_v2 import verifier_directive
        d = {"declencheur": {"evenement": "appel_outil", "cible": {"serveur": "rh-synthetique", "acces": "action"}},
             "effets": [{"effet": "refuser"}], "fondements": []}
        self.assertEqual(verifier_directive({}, d, self.vocab, "x", ""), [])

    def test_transmis_doit_etre_ecrit(self):
        p = d_gels()
        p["noeuds"][1]["critere"] = "transmis"
        r = self.compiler(CREDIT, ClientScripte(jeu(p), jeu(p)))
        self.assertTrue(any("critere transmis" in e for e in r["erreurs"]), r["erreurs"])
        r = self.compiler(CREDIT, ClientScripte(jeu(p)), [
            {"question": "Critère ?", "reponse": "Un appel transmis suffit, quel qu'en soit le résultat."},
            {"question": "q", "reponse": "refuser", "champ": "si_indetermine", "origine": "compilateur"}])
        self.assertEqual(r["etat"], c2.JEU, r["erreurs"])


class TestQuestionsReposees(Base):
    def test_question_deja_repondue_ecartee(self):
        q = appel("poser_question", {"question": "Quel seuil ?", "sujet": "seuil"})
        r = self.compiler(CREDIT, ClientScripte(reponse(q)), [{"question": "Quel seuil ? ", "reponse": "500 000 euros"}])
        self.assertEqual(r["etat"], c2.ERREURS)
        self.assertIn("déjà répondu", r["erreurs"][0])

    def test_reponses_dedoublonnees_et_definitives(self):
        cl = ClientScripte(jeu(d_seuil()))
        rep = {"question": "Quel seuil ?", "reponse": "500 000 euros"}
        self.compiler(CREDIT, cl, [rep, dict(rep)])
        contenu = cl.recus[0][0][1]["content"]
        self.assertEqual(contenu.count("Quel seuil ?"), 1)
        self.assertIn("ne repose pas ces questions", contenu)

    def test_fin_en_erreur_signalee(self):
        r = self.compiler(CREDIT, ClientScripte({"choices": [{"finish_reason": "error", "message": {"content": ""}}]}))
        self.assertEqual(r["etat"], c2.ERREURS)
        self.assertIn("error", r["erreurs"][0])


class TestSourceDesignee(Base):
    def test_source_designee(self):
        from compilateur.verifications_v2 import source_designee
        self.assertTrue(source_designee("gels-avoirs", "le registre des gels d'avoirs"))
        self.assertTrue(source_designee("taux-officiel", "à partir du taux officiel"))
        self.assertTrue(source_designee("recherche-entreprises", "le serveur de recherche d'entreprises"))
        self.assertFalse(source_designee("banque-synthetique", "toute attribution de prêt de plus de 2 M€"))

    def test_condition_ajoutee_refusee(self):
        p = d_seuil()
        p["racine"] = "e"
        p["noeuds"] += [{"id": "e", "type": "et", "enfants": ["a", "b"]},
                        {"id": "b", "type": "a_consulte", "outil": "banque-synthetique/attribuer_pret", "critere": "donnees_recues"}]
        r = self.compiler(CREDIT, ClientScripte(jeu(p), jeu(p)))
        self.assertTrue(any("la source banque-synthetique n'est pas désignée" in e for e in r["erreurs"]), r["erreurs"])


class TestDesignationPersonne(Base):
    """« utilisateur », « user », « la personne », « l'agent humain » : le code
    demande s'il s'agit du demandeur ou de la personne concernée, quand la
    proposition porte une feuille demandeur ou sujet."""

    TEXTE = "Si l'utilisateur est membre du groupe prestataires-externes, tout appel à attribuer_pret est bloqué (RGPD, art. 32)."
    SI = {"question": "q", "reponse": "refuser", "champ": "si_indetermine", "origine": "compilateur"}

    def propo(self, feuille):
        demandeur = {"id": "a", "type": "demandeur", "groupe": "prestataires-externes"}
        sujet = {"id": "a", "type": "sujet", "groupe": "prestataires-externes", "champ": "/siren"}
        if feuille == "les deux":
            noeuds = [{"id": "r", "type": "et", "enfants": ["a", "b"]}, demandeur, dict(sujet, id="b")]
        else:
            noeuds = [demandeur if feuille == "demandeur" else sujet]
        return jeu({"extrait": "x", "fondements": [{"texte": "RGPD", "article": "32"}], "declencheur": dict(PRET),
                    "racine": noeuds[0]["id"], "noeuds": noeuds, "effets": [{"effet": "refuser"}]})

    def designation(self, valeur):
        return {"question": "q", "reponse": valeur, "champ": "designation_personne", "origine": "compilateur"}

    def test_question_posee_par_le_code(self):
        cl = ClientScripte(self.propo("demandeur"))
        r = self.compiler(self.TEXTE, cl)
        self.assertEqual(r["etat"], c2.QUESTIONS)
        q = r["questions"][0]
        self.assertEqual((q["champ"], q["options"], q["origine"]),
                         ("designation_personne", ["demandeur", "personne_concernee"], "compilateur"))
        self.assertIn("« utilisateur »", q["question"])
        self.assertIn("la personne qui lance les agents (demandeur) ou la personne concernée par l'action", q["question"])
        self.assertEqual(len(cl.recus), 1, "aucun appel Mistral de plus")

    def test_reponse_conforme_consignee(self):
        cl = ClientScripte(self.propo("demandeur"))
        r = self.compiler(self.TEXTE, cl, [self.designation("demandeur"), self.SI])
        self.assertEqual(r["etat"], c2.JEU, r["erreurs"])
        self.assertEqual([c["champ"] for c in r["consignation"]], ["designation_personne", "si_indetermine"])
        self.assertNotIn("designation_personne", json.dumps(cl.recus, ensure_ascii=False))
        r = self.compiler(self.TEXTE, ClientScripte(self.propo("sujet")), [self.designation("Personne concernée"), self.SI])
        self.assertEqual(r["etat"], c2.JEU, r["erreurs"])

    def test_reponse_sans_feuille_correspondante_refuse_sans_rappel(self):
        for feuille, valeur, motif in (("demandeur", "personne_concernee",
                                        "désigne la personne concernée par l'action, mais aucune condition du jeu ne "
                                        "porte sur une feuille sujet (seulement : directive D-001 (feuille demandeur))"),
                                       ("sujet", "demandeur",
                                        "désigne le demandeur (la personne qui lance les agents), mais aucune condition "
                                        "du jeu ne porte sur une feuille demandeur (seulement : directive D-001 (feuille sujet))")):
            cl = ClientScripte(self.propo(feuille))
            r = self.compiler(self.TEXTE, cl, [self.designation(valeur), self.SI])
            self.assertEqual(r["etat"], c2.REFUSE)
            self.assertIsNone(r["jeu"])
            self.assertEqual(len(cl.recus), 1, "pas de rappel à Mistral")
            self.assertEqual(len(r["erreurs"]), 1)
            self.assertIn(motif, r["erreurs"][0])
            self.assertIn("« utilisateur »", r["erreurs"][0])
            self.assertEqual(r["consignation"][0]["champ"], "designation_personne")

    def test_demandeur_et_sujet_dans_la_meme_regle(self):
        """Une règle peut viser le demandeur et la personne concernée : la
        réponse dit seulement à quelle feuille le mot correspond, et le jeu
        est accepté dans les deux sens."""
        for valeur in ("demandeur", "personne_concernee"):
            cl = ClientScripte(self.propo("les deux"))
            r = self.compiler(self.TEXTE, cl, [self.designation(valeur), self.SI])
            self.assertEqual(r["etat"], c2.JEU, (valeur, r["erreurs"]))
            self.assertEqual(r["consignation"][0], self.designation(valeur))
            self.assertEqual(sorted(r["jeu"]["directives"][0]["condition"]["et"][i].popitem()[0] for i in (0, 1)),
                             ["demandeur", "sujet"])

    def test_question_tient_avec_un_jeu_existant(self):
        with open(os.path.join(directive_go.RACINE, "demo", "directives", "demo.json"), encoding="utf-8") as f:
            demo = json.load(f)
        r = c2.compiler_jeu(self.TEXTE, "ignore", self.vocab, ClientScripte(self.propo("les deux")),
                            "juriste@cabinet.fr", "2026-10-04T00:00:00Z", existant=demo)
        self.assertEqual([q["champ"] for q in r["questions"]], ["designation_personne", "si_indetermine"])
        self.assertIn("D-009", r["questions"][1]["question"])
        r = c2.compiler_jeu(self.TEXTE, "ignore", self.vocab, ClientScripte(self.propo("sujet")),
                            "juriste@cabinet.fr", "2026-10-04T00:00:00Z", existant=demo,
                            reponses=[self.designation("demandeur"), self.SI])
        self.assertEqual(r["etat"], c2.REFUSE)
        self.assertIn("directive D-009 (feuille sujet)", r["erreurs"][0])
        r = c2.compiler_jeu(self.TEXTE, "ignore", self.vocab, ClientScripte(self.propo("demandeur")),
                            "juriste@cabinet.fr", "2026-10-04T00:00:00Z", existant=demo,
                            reponses=[self.designation("demandeur"), self.SI])
        self.assertEqual(r["etat"], c2.JEU, r["erreurs"])
        self.assertEqual((r["jeu"]["jeu"]["id"], r["jeu"]["jeu"]["version"], r["ajoutees"]), ("demo", 2, ["D-009"]))
        self.assertEqual(r["jeu"]["directives"][:8], demo["directives"])

    def test_reponse_hors_options_redemandee(self):
        r = self.compiler(self.TEXTE, ClientScripte(self.propo("demandeur")), [self.designation("les deux"), self.SI])
        self.assertEqual(r["etat"], c2.QUESTIONS)
        self.assertEqual([q["champ"] for q in r["questions"]], ["designation_personne"])

    def test_pas_de_question_sans_mot_ambigu_ou_sans_feuille(self):
        texte = self.TEXTE.replace("l'utilisateur", "le demandeur")
        r = self.compiler(texte, ClientScripte(self.propo("demandeur")))
        self.assertEqual([q["champ"] for q in r["questions"]], ["si_indetermine"])
        texte = ("L'utilisateur ne compte pas ici. Au-delà de 500 000 euros, l'attribution d'un prêt est bloquée "
                 "(Politique de crédit, art. 3.2).")
        r = self.compiler(texte, ClientScripte(jeu(d_seuil())))
        self.assertEqual([q["champ"] for q in r["questions"]], ["si_indetermine"])

    def test_mots_reconnus(self):
        for t, mot in (("Le user ne peut pas", "user"), ("la personne visée", "la personne"),
                       ("l’agent humain décide", "l’agent humain"), ("Les utilisatrices", "utilisatrices")):
            self.assertEqual(c2.mot_personne_ambigu(t), mot)
        self.assertIsNone(c2.mot_personne_ambigu("Aucun appel le week-end."))
