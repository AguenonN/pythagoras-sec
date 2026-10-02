#!/usr/bin/env python3
"""Simulateur du service du compilateur (contrats de la démo, §4).

Lit une requête JSON sur l'entrée standard, écrit une réponse JSON sur la
sortie standard. Aucun modèle n'est appelé : les réponses sont déterministes,
d'après quelques mots du texte. L'état de la conversation vit dans
demo/execution/compilation/<id>/ (relatif au répertoire courant).
"""
import hashlib
import json
import os
import sys

req = json.load(sys.stdin)
texte = (req.get("texte") or "").strip()
conv = req.get("conversation")
base = os.path.join("demo", "execution", "compilation")


def sortie(d):
    d.setdefault("questions", [])
    d.setdefault("non_observable", None)
    d.setdefault("relecture", "")
    d.setdefault("jeu_candidat", None)
    d.setdefault("erreurs", [])
    json.dump(d, sys.stdout)
    sys.exit(0)


if conv:
    etat_f = os.path.join(base, conv, "etat.json")
    with open(etat_f) as f:
        etat = json.load(f)
    # Comme le service réel : une conversation porte un seul texte ; une
    # valeur donnée et différente ouvre une autre conversation.
    for cle in ("texte", "fondement", "auteur", "jeu_existant", "date_effet"):
        v = req.get(cle)
        if v is not None and (v.strip() if isinstance(v, str) else v) != etat[cle]:
            sortie({"conversation": conv, "etat": "erreurs",
                    "erreurs": ["%s différent de celui de la conversation %s : ouvrir une nouvelle conversation"
                                % (cle, conv)]})
    texte = etat["texte"]
    req.update({cle: etat[cle] for cle in ("fondement", "auteur", "jeu_existant", "date_effet")})
else:
    conv = "c-" + hashlib.sha1(texte.encode()).hexdigest()[:8]
    os.makedirs(os.path.join(base, conv), exist_ok=True)
    with open(os.path.join(base, conv, "etat.json"), "w") as f:
        json.dump({"texte": texte, "fondement": (req.get("fondement") or "").strip(), "auteur": req["auteur"],
                   "jeu_existant": req["jeu_existant"], "date_effet": req["date_effet"]}, f)

t = texte.lower()
if "discrimin" in t:
    sortie({"conversation": conv, "etat": "non_observable",
            "non_observable": {"extrait": texte, "raison": "critère non observable dans un appel d'outil"}})
if "inconnu" in t:
    sortie({"conversation": conv, "etat": "refuse", "erreurs": ["Le texte ne se rattache à aucun outil connu."]})

reponses = {r["question"]: r["reponse"] for r in req.get("reponses") or []}
if "week-end" in t and "q-fuseau" not in reponses:
    sortie({"conversation": conv, "etat": "questions",
            "questions": [{"id": "q-fuseau", "texte": "Quel fuseau horaire pour le week-end ?",
                           "options": ["Europe/Paris", "UTC"], "origine": "code"},
                          {"id": "q-acces", "texte": "Quels appels visez-vous : toutes les actions des agents ?",
                           "options": ["oui", "non"], "origine": "mistral"}]})

with open(req["jeu_existant"]) as f:
    jeu = json.load(f)
n = max(int(d["id"].split("-")[1]) for d in jeu["directives"]) + 1
ident = "D-%03d" % n
fond = [{"texte": req.get("fondement") or "à valider par le juriste"}]
d = {"id": ident, "version": 1, "statut": "brouillon", "date_effet": req["date_effet"], "texte_fr": texte,
     "fondements": fond}
if "week-end" in t:
    fuseau = reponses.get("q-fuseau", "Europe/Paris")
    d["declencheur"] = {"evenement": "appel_outil", "cible": {"serveur": "*", "outil": "*", "acces": "action"}}
    d["condition"] = {"temps": {"jour_semaine": ["samedi", "dimanche"], "fuseau": fuseau}}
    d["effets"] = [{"effet": "refuser"}]
else:  # base interne des taux
    d["declencheur"] = {"evenement": "appel_outil", "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"}}
    d["condition"] = {"session.a_consulte": {"outil": "banque-synthetique/get_taux_interne", "critere": "donnees_recues"}}
    d["effets"] = [{"effet": "attendre_validation", "par_role": "analyste-risques"}]
nouveau = {"jeu": {"id": jeu["jeu"]["id"], "version": jeu["jeu"]["version"] + 1, "date_effet": req["date_effet"],
                   "auteur": req["auteur"]},
           "directives": jeu["directives"] + [d]}
chemin = os.path.join(base, conv, "demo.v%d.json" % nouveau["jeu"]["version"])
with open(chemin, "w") as f:
    json.dump(nouveau, f, ensure_ascii=False, indent=2)
sortie({"conversation": conv, "etat": "jeu", "jeu_candidat": chemin.replace(os.sep, "/"),
        "relecture": "Directive %s, version 1." % ident})
