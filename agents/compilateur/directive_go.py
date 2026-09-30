"""Appel des sous-commandes Go ``directive verifier`` et ``directive relire``.

Le binaire est construit depuis le dépôt dans ``agents/.bin`` (ignoré par
git), une fois par processus ; ``DIRECTIVE_BIN`` permet d'en fournir un.
"""

import json
import os
import subprocess

ICI = os.path.dirname(os.path.abspath(__file__))
RACINE = os.path.dirname(os.path.dirname(ICI))
_binaire = None


class ErreurOutil(Exception):
    pass


def binaire():
    global _binaire
    if _binaire:
        return _binaire
    chemin = os.environ.get("DIRECTIVE_BIN")
    if not chemin:
        chemin = os.path.join(RACINE, "agents", ".bin", "directive")
        r = subprocess.run(["go", "build", "-o", chemin, "./cmd/directive"],
                           cwd=RACINE, capture_output=True, text=True)
        if r.returncode != 0:
            raise ErreurOutil("construction de cmd/directive impossible :\n" + r.stderr)
    _binaire = chemin
    return chemin


def _lancer(sous_commande, jeu, options=()):
    donnees = json.dumps(jeu, ensure_ascii=False).encode("utf-8")
    return subprocess.run([binaire(), sous_commande, *options], input=donnees, capture_output=True)


def verifier(jeu, exiger_brouillon=True):
    """Rend (ok, erreurs, empreinte) selon le chargeur strict. Par défaut,
    toute directive hors brouillon est aussi refusée : le compilateur ne
    produit jamais une directive appliquée."""
    r = _lancer("verifier", jeu, ["--exiger-brouillon"] if exiger_brouillon else [])
    if r.returncode not in (0, 1):
        raise ErreurOutil("directive verifier : " + r.stderr.decode("utf-8", "replace"))
    rapport = json.loads(r.stdout.decode("utf-8"))
    return rapport["ok"], rapport.get("erreurs", []), rapport.get("empreinte", "")


def relire(jeu):
    r = _lancer("relire", jeu)
    if r.returncode != 0:
        raise ErreurOutil("directive relire : " + r.stderr.decode("utf-8", "replace"))
    return r.stdout.decode("utf-8")
