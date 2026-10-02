#!/usr/bin/env python3
"""Éditions de fichiers du dépôt faites par scripts/nouveau-serveur.sh.

    outillage.py liste        <racine> <nom>
    outillage.py supplement   <racine> <nom>
    outillage.py etiquettes   <racine> <nom> <outils.json> <etiquettes.json>
    outillage.py controle     <outils.json> <etiquettes.json>      (ne modifie rien)

Chacune est idempotente : relancée, elle ne change rien. Les étiquettes vont
dans demo/serveurs-supplementaires.json (registredemo.FichierSupplementaires),
jamais dans demo/etiquettes.json : celui-ci et le vocabulaire versionné du
compilateur restent ceux de la démo figée.
"""
import json
import os
import re
import sys

ETIQUETTE = re.compile(r"^[a-z][a-z0-9_-]{0,63}$")
POINTEUR = re.compile(r"^(/[A-Za-z0-9_]+)+$")


def echec(msg):
    print("nouveau-serveur: " + msg, file=sys.stderr)
    sys.exit(1)


def ecrire(chemin, texte):
    tmp = chemin + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        f.write(texte)
    if os.path.exists(chemin):
        os.chmod(tmp, os.stat(chemin).st_mode)  # le bit d'exécution des scripts
    os.replace(tmp, chemin)


def liste(racine, nom):
    chemin = os.path.join(racine, "scripts", "construire-serveurs.sh")
    texte = open(chemin, encoding="utf-8").read()
    m = re.search(r"(serveurs=\(\n)((?:\t[a-z0-9-]+\n)+)(\))", texte)
    if not m:
        echec("liste « serveurs » introuvable dans scripts/construire-serveurs.sh")
    noms = m.group(2).split()
    if nom not in noms:
        noms.append(nom)
    corps = "".join("\t%s\n" % n for n in noms)
    ecrire(chemin, texte[: m.start(2)] + corps + texte[m.end(2):])


def charger_supplement(racine):
    chemin = os.path.join(racine, "demo", "serveurs-supplementaires.json")
    if os.path.exists(chemin):
        return chemin, json.load(open(chemin, encoding="utf-8"))
    return chemin, {"serveurs": []}


def supplement(racine, nom):
    chemin, doc = charger_supplement(racine)
    if nom not in doc["serveurs"]:
        doc["serveurs"].append(nom)
    ecrire(chemin, rendre(doc))


def inline(valeur):
    return json.dumps(valeur, ensure_ascii=False)


def rendre(doc):
    """JSON lisible : un serveur ou un outil par ligne, tableaux en ligne."""
    def bloc(val, niveau):
        marge = "  " * (niveau + 1)
        if isinstance(val, dict) and any(isinstance(v, dict) for v in val.values()) or (isinstance(val, dict) and niveau == 0):
            lignes = [marge + "%s: %s" % (inline(k), bloc(v, niveau + 1)) for k, v in val.items()]
            return "{\n" + ",\n".join(lignes) + "\n" + "  " * niveau + "}"
        if isinstance(val, dict):
            lignes = [marge + "%s: %s" % (inline(k), inline(v)) for k, v in val.items()]
            return "{\n" + ",\n".join(lignes) + "\n" + "  " * niveau + "}" if lignes else "{}"
        return inline(val)
    return bloc(doc, 0) + "\n"


def controle(chemin_outils, chemin_etiquettes):
    """Valide le fichier d'étiquettes ; rend (serveur, par_outil, personnes)."""
    outils = {o["nom"] for o in json.load(open(chemin_outils, encoding="utf-8"))["outils"]}
    try:
        e = json.load(open(chemin_etiquettes, encoding="utf-8"))
    except (OSError, ValueError) as ex:
        echec("étiquettes illisibles : %s" % ex)
    inconnus = set(e) - {"serveur", "outils", "personnes_concernees"}
    if inconnus:
        echec("étiquettes : clé(s) inconnue(s) %s (serveur, outils, personnes_concernees)" % sorted(inconnus))
    serveur = e.get("serveur", [])
    par_outil = e.get("outils", {})
    personnes = e.get("personnes_concernees", {})
    if not isinstance(serveur, list) or not isinstance(par_outil, dict) or not isinstance(personnes, dict):
        echec("étiquettes : « serveur » est une liste, « outils » et « personnes_concernees » des objets")
    for l in [serveur] + list(par_outil.values()):
        if not isinstance(l, list) or not all(isinstance(x, str) and ETIQUETTE.match(x) for x in l):
            echec("étiquettes : un nom d'étiquette est en minuscules, chiffres, « _ » ou « - » (%s)" % l)
    for o in list(par_outil) + list(personnes):
        if o not in outils:
            echec("étiquettes : l'outil %r n'est pas déclaré dans outils.json (outils : %s)" % (o, ", ".join(sorted(outils))))
    for o, p in personnes.items():
        if not isinstance(p, str) or not POINTEUR.match(p):
            echec("étiquettes : personnes_concernees.%s doit être un pointeur d'argument, par exemple /salarie_id" % o)
    return serveur, par_outil, personnes


def etiquettes(racine, nom, chemin_outils, chemin_etiquettes):
    serveur, par_outil, personnes = controle(chemin_outils, chemin_etiquettes)
    chemin, doc = charger_supplement(racine)
    et = doc.setdefault("etiquettes", {})
    et.setdefault("serveurs", {})
    et.setdefault("outils", {})
    if serveur:
        et["serveurs"][nom] = sorted(set(et["serveurs"].get(nom, [])) | set(serveur))
    for o, l in par_outil.items():
        cle = "%s/%s" % (nom, o)
        et["outils"][cle] = sorted(set(et["outils"].get(cle, [])) | set(l))
    if personnes:
        et.setdefault("personnes_concernees", {})
        for o, p in personnes.items():
            et["personnes_concernees"]["%s/%s" % (nom, o)] = p
    ecrire(chemin, rendre(doc))


if __name__ == "__main__":
    commande, args = sys.argv[1], sys.argv[2:]
    {"liste": liste, "supplement": supplement, "etiquettes": etiquettes, "controle": controle}[commande](*args)
