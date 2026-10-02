#!/usr/bin/env python3
"""Bouchon du service du compilateur, pour test/extension : AUCUN modèle n'est
appelé. Il rend, pour tout texte, la directive de testdata/directive-d009.json
ajoutée au jeu existant en brouillon (version n+1), au format du contrat des
démos (docs/orchestration/contrats-demo.md §4). Le vrai compilateur
(python -m compilateur.service) appelle Mistral : jamais dans un test."""
import json
import os
import sys

req = json.load(sys.stdin)
ici = os.path.dirname(os.path.abspath(__file__))
with open(os.path.join(ici, "directive-d009.json"), encoding="utf-8") as f:
    d = json.loads(f.read().replace("@DATE@", req["date_effet"]))
with open(req["jeu_existant"], encoding="utf-8") as f:
    jeu = json.load(f)
conv = "c-extension"
dossier = os.path.join("demo", "execution", "compilation", conv)
os.makedirs(dossier, exist_ok=True)
nouveau = {"jeu": {"id": jeu["jeu"]["id"], "version": jeu["jeu"]["version"] + 1,
                   "date_effet": req["date_effet"], "auteur": req["auteur"]},
           "directives": jeu["directives"] + [d]}
chemin = os.path.join(dossier, "demo.v%d.json" % nouveau["jeu"]["version"])
with open(chemin, "w", encoding="utf-8") as f:
    json.dump(nouveau, f, ensure_ascii=False, indent=2)
json.dump({"conversation": conv, "etat": "jeu", "questions": [], "non_observable": None,
           "relecture": "", "jeu_candidat": chemin.replace(os.sep, "/"), "erreurs": []}, sys.stdout)
