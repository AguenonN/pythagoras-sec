"""L'Analyste : propose au juriste des directives à partir des exécutions passées.

    cd agents
    python -m analyste constats  [--sessions DIR] [--jeu F] [--seuil-montant N]
    python -m analyste proposer  [--sessions DIR] [--jeu F] [--sortie DIR] [--mode rejeu|reel] [--seuil-montant N]
    python -m analyste mesurer   <id de proposition>

``constats`` : détection seule, par le code, sans modèle (JSON sur la sortie).
``proposer`` : détection, rédaction par Mistral de chaque constat non encore
proposé, écriture de ``demo/execution/propositions/<id>.json``. Jamais
d'écrasement, jamais de décision écrite.
``mesurer`` : avant décision ou compilation, les sessions concernées ;
après compilation par le juriste, le rejeu (``mesure``).

Mode ``rejeu`` (défaut) : réponses enregistrées seulement
(``analyste/enregistrements/``) ; ``reel`` : l'API sinon, 15 appels au plus.
"""

import argparse
import json
import os
import sys

from compilateur import mistral

from . import couverture, detecteurs, journaux, mesure, propositions, redaction

RACINE = detecteurs.RACINE
SESSIONS = os.path.join(RACINE, "demo", "execution", "sessions")
JEU = os.path.join(RACINE, "demo", "directives", "demo.json")
SORTIE = os.path.join(RACINE, "demo", "execution", "propositions")


def constats(sessions_dir, jeu, seuil=None, racine=RACINE):
    jeu_courant = couverture.lire_jeu(jeu)
    catalogue = detecteurs.lire_catalogue()
    sessions = journaux.lire_sessions(sessions_dir)
    occ, non_verifiables = detecteurs.detecter(sessions, catalogue, racine, seuil, jeu_courant)
    cs, couverts = propositions.regrouper(occ, catalogue, jeu_courant)
    for c in cs:
        c["contexte"] = propositions.contexte(c, sessions, jeu_courant, catalogue)
    return {"sessions_lues": len(sessions), "constats": cs, "couverts": couverts,
            "non_verifiables": non_verifiables}


def proposer(sessions_dir, jeu, sortie, client, seuil=None, racine=RACINE, seulement=None):
    """``seulement`` : identifiants des constats à rédiger (tous si vide)."""
    res = constats(sessions_dir, jeu, seuil, racine)
    ecrites, deja, erreurs = [], [], []
    for c in res["constats"]:
        if seulement and c["id"] not in seulement:
            continue
        if propositions.deja_proposee(sortie, c["id"]):
            deja.append(c["id"])
            continue
        try:
            r = redaction.rediger(c, client)
        except (redaction.RedactionInvalide, mistral.ErreurMistral) as e:
            erreurs.append({"id": c["id"], "erreur": "%s : %s" % (type(e).__name__, e)})
            continue
        p = propositions.proposition(c, r)
        if propositions.ecrire(sortie, p):
            ecrites.append(p)
        else:
            deja.append(c["id"])
    return {"sessions_lues": res["sessions_lues"], "ecrites": ecrites, "deja_proposees": deja,
            "couverts": res["couverts"], "non_verifiables": res["non_verifiables"], "erreurs": erreurs}


def principal(argv=None, sortie=sys.stdout):
    ap = argparse.ArgumentParser(prog="analyste", description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="commande", required=True)
    for nom in ("constats", "proposer"):
        p = sub.add_parser(nom)
        p.add_argument("--sessions", default=SESSIONS)
        p.add_argument("--jeu", default=JEU, help="jeu en vigueur (ce qui est déjà couvert)")
        p.add_argument("--seuil-montant", type=float, default=None)
        if nom == "proposer":
            p.add_argument("--sortie", default=SORTIE)
            p.add_argument("--mode", choices=["rejeu", "reel"], default=os.environ.get("ANALYSTE_MODE", "rejeu"))
            p.add_argument("--constat", action="append", default=[], help="ne rédiger que ce constat (répétable)")
    m = sub.add_parser("mesurer")
    m.add_argument("id")
    m.add_argument("--propositions", default=SORTIE)
    m.add_argument("--sessions", default=SESSIONS)
    a = ap.parse_args(argv)

    if a.commande == "constats":
        out = constats(a.sessions, a.jeu, a.seuil_montant)
        code = 0
    elif a.commande == "proposer":
        out = proposer(a.sessions, a.jeu, a.sortie, redaction.client(a.mode), a.seuil_montant,
                       seulement=a.constat)
        code = 1 if out["erreurs"] else 0
    else:
        out = mesure.mesurer(a.id, propositions=a.propositions, sessions=a.sessions)
        if out["etat"] == "mesuree":
            out["fichier"] = os.path.relpath(mesure.ecrire(out), RACINE)
        code = 0
    json.dump(out, sortie, ensure_ascii=False, indent=1)
    sortie.write("\n")
    return code


if __name__ == "__main__":
    sys.exit(principal())
