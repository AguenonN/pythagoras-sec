"""Sous-agents déclaratifs : un fichier ``sous_agents/<nom>.json`` par
sous-agent, aucune ligne de Python pour en ajouter un.

    {
      "nom": "conformite",                      # = nom du fichier
      "role": "contrôle l'entreprise et le registre des gels",   # facultatif
      "consigne": "Tu es le sous-agent conformité…",
      "outils": ["recherche-entreprises__rechercher_entreprise", "gels-avoirs__*"],
      "schema_sortie": {"type": "object", "properties": {…}, "required": […]}
    }

Le chargeur refuse, avec un message par erreur et le nom du fichier : un
champ inconnu ou manquant, un nom qui n'est pas celui du fichier, un outil
absent du registre (``serveur__outil`` exact, ou ``serveur__*`` qui doit
désigner au moins un outil), un schéma de sortie hors du sous-ensemble
admis (schema_sortie.py). Le rôle, absent, est la première phrase de la
consigne : c'est lui que l'orchestrateur lit pour choisir à qui déléguer.
"""

import glob
import json
import os
import re

from . import schema_sortie

ICI = os.path.dirname(os.path.abspath(__file__))
DOSSIER = os.path.join(ICI, "sous_agents")
CHAMPS = {"nom", "role", "consigne", "outils", "schema_sortie"}
OBLIGATOIRES = ("nom", "consigne", "outils", "schema_sortie")
NOM = re.compile(r"^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$")


class SousAgentInvalide(Exception):
    pass


class SousAgent:
    def __init__(self, nom, role, consigne, outils, schema, fichier):
        self.nom = nom
        self.role = role
        self.consigne = consigne
        self.outils = outils
        self.schema = schema
        self.fichier = fichier

    def __repr__(self):
        return "SousAgent(%s)" % self.nom


def premiere_phrase(texte):
    t = " ".join(texte.split())
    m = re.search(r"[.!?](\s|$)", t)
    return t[: m.start() + 1] if m else t


def resoudre_outils(demandes, disponibles):
    """Rend (outils exacts triés, erreurs)."""
    out, erreurs = set(), []
    for o in demandes:
        if not isinstance(o, str) or "__" not in o:
            erreurs.append("outil %r : nom attendu serveur__outil" % (o,))
            continue
        if o.endswith("__*"):
            serveur = o[:-1]
            trouves = {d for d in disponibles if d.startswith(serveur)}
            if not trouves:
                erreurs.append("outil %s : aucun outil de ce serveur au registre" % o)
            out |= trouves
        elif o in disponibles:
            out.add(o)
        else:
            erreurs.append("outil %s : absent du registre" % o)
    return sorted(out), erreurs


def lire(chemin, disponibles):
    nom_fichier = os.path.splitext(os.path.basename(chemin))[0]
    try:
        with open(chemin, encoding="utf-8") as f:
            d = json.load(f)
    except (OSError, ValueError) as e:
        raise SousAgentInvalide("%s : illisible (%s)" % (os.path.basename(chemin), e)) from None
    erreurs = []
    if not isinstance(d, dict):
        raise SousAgentInvalide("%s : un objet JSON est attendu" % os.path.basename(chemin))
    for c in sorted(set(d) - CHAMPS):
        erreurs.append("champ inconnu « %s » (admis : %s)" % (c, ", ".join(sorted(CHAMPS))))
    for c in OBLIGATOIRES:
        if c not in d:
            erreurs.append("champ obligatoire « %s » absent" % c)
    nom = d.get("nom")
    if "nom" in d and (not isinstance(nom, str) or not NOM.match(nom)):
        erreurs.append("nom %r : minuscules, chiffres et tirets" % (nom,))
    elif "nom" in d and nom != nom_fichier:
        erreurs.append("nom « %s » différent du nom du fichier « %s »" % (nom, nom_fichier))
    consigne = d.get("consigne")
    if "consigne" in d and (not isinstance(consigne, str) or not consigne.strip()):
        erreurs.append("consigne : texte non vide attendu")
    role = d.get("role")
    if role is not None and (not isinstance(role, str) or not role.strip()):
        erreurs.append("role : texte non vide attendu")
    outils = []
    if "outils" in d:
        if not isinstance(d["outils"], list) or not d["outils"]:
            erreurs.append("outils : liste non vide attendue")
        else:
            outils, e = resoudre_outils(d["outils"], disponibles)
            erreurs += e
    if "schema_sortie" in d:
        try:
            schema_sortie.verifier_schema(d["schema_sortie"])
        except schema_sortie.SchemaInvalide as e:
            erreurs += str(e).split("\n")
    if erreurs:
        raise SousAgentInvalide("\n".join("%s : %s" % (os.path.basename(chemin), x) for x in erreurs))
    return SousAgent(nom, role.strip() if role else premiere_phrase(consigne), consigne.strip(), outils,
                     d["schema_sortie"], chemin)


def charger(disponibles, dossier=DOSSIER):
    """Charge tous les sous-agents du dossier, dans l'ordre des noms. Toutes
    les erreurs sont rendues en bloc."""
    disponibles = set(disponibles)
    agents, erreurs = {}, []
    for chemin in sorted(glob.glob(os.path.join(dossier, "*.json"))):
        try:
            a = lire(chemin, disponibles)
        except SousAgentInvalide as e:
            erreurs.append(str(e))
            continue
        agents[a.nom] = a
    if erreurs:
        raise SousAgentInvalide("\n".join(erreurs))
    if not agents:
        raise SousAgentInvalide("aucun sous-agent dans %s" % dossier)
    return agents


def outils_du_catalogue(chemin):
    """Les noms exposés (serveur__outil) d'un catalogue généré par
    cmd/registre-demo : le registre hors ligne, pour vérifier les
    sous-agents sans lancer le gate."""
    with open(chemin, encoding="utf-8") as f:
        c = json.load(f)
    return {"%s__%s" % (s["nom"], o["nom"]) for s in c["serveurs"] for o in s["outils"]}
