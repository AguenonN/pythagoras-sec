"""Critère de réussite du brief E : le texte proposé par l'Analyste, passé au
compilateur comme le juriste le reprendrait tel quel, donne D-009.

    cd agents
    python -m analyste.critere [--mode rejeu|reel] [exemples/critere-d009.json]

Le parcours (``exemples/critere-d009.json``) donne la proposition reprise
et les réponses du juriste aux questions du compilateur. Le service du
compilateur (contrats §4) est appelé tel quel, avec un client Mistral dont
les réponses sont enregistrées dans ``analyste/enregistrements/compilateur/``
(et comptées dans le budget de l'Analyste) : rejouable hors ligne. Les
conversations vivent dans un dossier temporaire.

D-009 (contrats §1) : sur ``banque-synthetique/attribuer_pret``, si la
session a consulté ``banque-synthetique/get_taux_interne``, attendre la
validation d'un ``analyste-risques``. ``conforme`` rend les écarts.
"""

import argparse
import json
import os
import sys
import tempfile

from compilateur import mistral, service

from . import couverture, redaction

ICI = os.path.dirname(os.path.abspath(__file__))
PARCOURS = os.path.join(ICI, "exemples", "critere-d009.json")
ENREGISTREMENTS = os.path.join(ICI, "enregistrements", "compilateur")
INTERNE = "banque-synthetique/get_taux_interne"


def conforme(directive):
    """Écarts entre une directive compilée et D-009 ; vide si conforme."""
    ecarts = []
    cible = (directive.get("declencheur") or {}).get("cible") or {}
    if (directive.get("declencheur") or {}).get("evenement") != "appel_outil" or \
            cible.get("serveur") != "banque-synthetique" or cible.get("outil") != "attribuer_pret":
        ecarts.append("déclencheur : attendu appel_outil sur banque-synthetique/attribuer_pret, obtenu %s"
                      % json.dumps(directive.get("declencheur"), ensure_ascii=False))
    if "outil:" + INTERNE not in couverture.references(directive.get("condition")):
        ecarts.append("condition : ne porte pas sur la consultation de %s" % INTERNE)
    if _sous_non(directive.get("condition"), "outil:" + INTERNE):
        ecarts.append("condition : la consultation de %s est niée" % INTERNE)
    effets = directive.get("effets") or []
    attente = [e for e in effets if e.get("effet") == "attendre_validation"]
    if not attente or attente[0].get("par_role") != "analyste-risques":
        ecarts.append("effets : attendu attendre_validation par analyste-risques, obtenu %s"
                      % json.dumps(effets, ensure_ascii=False))
    if any(e.get("effet") == "refuser" for e in effets):
        ecarts.append("effets : refuser en plus de l'attente")
    return ecarts


def _sous_non(noeud, ref, nie=False):
    if isinstance(noeud, list):
        return any(_sous_non(n, ref, nie) for n in noeud)
    if not isinstance(noeud, dict):
        return False
    for cle, val in noeud.items():
        if cle == "session.a_consulte" and isinstance(val, dict) and "outil:" + val.get("outil", "") == ref:
            return nie
        if _sous_non(val, ref, not nie if cle == "non" else nie):
            return True
    return False


def fabrique(mode):
    return lambda modele: mistral.Client(mode=mode, modele=modele, enregistrements=ENREGISTREMENTS,
                                         etat=redaction.ETAT, max_appels=redaction.MAX_APPELS)


def lire_parcours(chemin):
    with open(chemin, encoding="utf-8") as f:
        p = json.load(f)
    with open(os.path.join(os.path.dirname(chemin), p["proposition"]), encoding="utf-8") as f:
        prop = json.load(f)
    requete = {"texte": prop["texte_propose"], "fondement": prop["fondement_propose"], "auteur": p["auteur"],
               "jeu_existant": p["jeu_existant"], "date_effet": p["date_effet"]}
    return prop, requete, p.get("reponses") or {}


def derouler(chemin=PARCOURS, mode="rejeu"):
    prop, requete, reponses = lire_parcours(chemin)
    with tempfile.TemporaryDirectory() as dossier:
        svc = service.Service(mode=mode, dossier=dossier, client=fabrique(mode))
        sorties = service.parcours(svc, requete, reponses)
        fin = sorties[-1]
        resultat = {"proposition": prop["id"], "texte": requete["texte"], "tours": len(sorties),
                    "etat": fin["etat"], "questions_sans_reponse": [q["texte"] for q in fin.get("questions", [])],
                    "erreurs": fin.get("erreurs", []), "relecture": fin.get("relecture", "")}
        if fin["etat"] == "jeu" and fin.get("jeu_candidat"):
            with open(service.chemin_absolu(fin["jeu_candidat"]), encoding="utf-8") as f:
                jeu = json.load(f)
            with open(service.chemin_absolu(requete["jeu_existant"]), encoding="utf-8") as f:
                pris = {d["id"] for d in json.load(f)["directives"]}
            ajoutees = [d for d in jeu["directives"] if d["id"] not in pris]
            resultat["ajoutees"] = ajoutees
            resultat["ecarts"] = [] if len(ajoutees) == 1 else ["%d directive(s) ajoutée(s), 1 attendue" % len(ajoutees)]
            if ajoutees:
                resultat["ecarts"] += conforme(ajoutees[0])
            resultat["conforme_d009"] = not resultat["ecarts"]
        else:
            resultat["conforme_d009"] = False
    return resultat


def principal(argv=None):
    ap = argparse.ArgumentParser(prog="analyste.critere", description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("parcours", nargs="?", default=PARCOURS)
    ap.add_argument("--mode", choices=["rejeu", "reel"], default="rejeu")
    a = ap.parse_args(argv)
    r = derouler(a.parcours, a.mode)
    json.dump(r, sys.stdout, ensure_ascii=False, indent=1)
    sys.stdout.write("\n")
    return 0 if r["conforme_d009"] else 1


if __name__ == "__main__":
    sys.exit(principal())
