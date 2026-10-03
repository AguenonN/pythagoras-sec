"""Une directive couvre-t-elle déjà un risque ?

Deux questions, toutes deux par lecture des directives (schéma v2), sans
évaluer de condition :

- pendant la session : l'une des directives **appliquées** à l'appel
  (journal) porte-t-elle sur l'élément du risque (``references``) ? Le jeu
  lu est celui que la session a chargé (``jeu.json`` du dossier), à défaut
  le jeu courant ;
- aujourd'hui : une directive ``active`` ou ``observation`` du jeu courant
  vise-t-elle l'action et porte-t-elle sur cet élément ? Si oui, l'Analyste
  ne propose rien : le juriste a déjà tranché ce risque.
"""

import json
import os

EVALUEES = ("active", "observation")


def lire_jeu(chemin):
    try:
        with open(chemin, encoding="utf-8") as f:
            return json.load(f)
    except (FileNotFoundError, ValueError):
        return None


def references(noeud):
    """Ce sur quoi porte une condition : ``outil:s/o``, ``serveur:s``,
    ``etiquette:e``, ``champ:/ptr``, ``groupe:g``."""
    out = set()
    if isinstance(noeud, list):
        for n in noeud:
            out |= references(n)
        return out
    if not isinstance(noeud, dict):
        return out
    for cle, val in noeud.items():
        if cle in ("session.a_consulte", "metadonnee_reponse") and isinstance(val, dict):
            for k in ("outil", "serveur", "etiquette"):
                if k in val:
                    out.add("%s:%s" % (k, val[k]))
        elif cle == "argument" and isinstance(val, dict):
            out.add("champ:%s" % val.get("champ", ""))
        elif cle == "sujet" and isinstance(val, dict):
            out.add("groupe:%s" % val.get("groupe", ""))
        elif cle == "etiquette_cible" and isinstance(val, dict):
            out.add("etiquette:%s" % val.get("contient", ""))
        else:
            out |= references(val)
    return out


def vise(directive, serveur, outil, etiquettes=(), acces=None):
    """Le déclencheur de la directive correspond-il à cet appel ?"""
    d = directive.get("declencheur") or {}
    if d.get("evenement") != "appel_outil":
        return False
    c = d.get("cible") or {}
    if c.get("serveur", "*") not in ("*", serveur):
        return False
    if c.get("outil", "*") not in ("*", outil):
        return False
    if "etiquette" in c and c["etiquette"] not in etiquettes:
        return False
    if "acces" in c and acces is not None and c["acces"] != acces:
        return False
    return True


def porte_sur(directive, elements):
    return bool(references(directive.get("condition")) & set(elements))


def directives_par_id(jeu):
    return {d.get("id"): d for d in (jeu or {}).get("directives", [])}


def jeu_de_session(session, jeu_courant):
    return lire_jeu(os.path.join(session.dossier, "jeu.json")) or jeu_courant


def couvert_pendant(session, appel, elements, jeu_courant):
    """Une directive appliquée à l'appel portait sur l'un des éléments."""
    par_id = directives_par_id(jeu_de_session(session, jeu_courant))
    return [i for i in appel.directives_appliquees() if i in par_id and porte_sur(par_id[i], elements)]


def couvert_aujourdhui(jeu_courant, serveur, outil, elements, etiquettes=(), acces=None):
    """Directives évaluées du jeu courant qui visent l'action et portent
    sur l'un des éléments."""
    out = []
    for d in (jeu_courant or {}).get("directives", []):
        if d.get("statut") in EVALUEES and vise(d, serveur, outil, etiquettes, acces) and porte_sur(d, elements):
            out.append(d.get("id"))
    return out


def seuils(noeud, champ, sous_non=False):
    """Valeurs des feuilles ``argument`` sur ``champ`` avec ``>`` ou ``>=``,
    hors négation : les montants à partir desquels la directive retient."""
    out = []
    if isinstance(noeud, list):
        for n in noeud:
            out += seuils(n, champ, sous_non)
        return out
    if not isinstance(noeud, dict):
        return out
    for cle, val in noeud.items():
        if cle == "argument" and isinstance(val, dict):
            if not sous_non and val.get("champ") == champ and val.get("op") in (">", ">=") \
                    and isinstance(val.get("valeur"), (int, float)) and not isinstance(val.get("valeur"), bool):
                out.append(val["valeur"])
        elif cle == "non":
            out += seuils(val, champ, not sous_non)
        else:
            out += seuils(val, champ, sous_non)
    return out
