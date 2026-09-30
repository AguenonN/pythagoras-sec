"""Sondes : Mistral accepte-t-il un mot-clé JSON Schema dans ``parameters`` ?

    python -m boucle.sondes              # rejoue boucle/sondes.json, n'appelle rien
    python -m boucle.sondes --mode reel  # sonde ce qui n'a pas encore de verdict

Mistral ne documente pas le sous-ensemble de JSON Schema qu'il accepte. Pour
chaque construction que les vrais schémas épinglés emploient et que
``schemas`` retire ou réduit, une sonde envoie une requête minimale : une
seule fonction dont les paramètres portent la construction, rien d'autre.
Réponse 200 : acceptée ; erreur 4xx : refusée. Le verdict est consigné dans
``sondes.json`` (sans rien de la réponse ni de la clé) et n'est plus jamais
redemandé. Les appels réels comptent dans le budget de la boucle.
"""

import argparse
import json
import os
import sys
import tempfile

from compilateur import mistral

from .__main__ import ETAT, MAX_APPELS

ICI = os.path.dirname(os.path.abspath(__file__))
VERDICTS = os.path.join(ICI, "sondes.json")

# Construction sondée → paramètres de la fonction de sonde. Chacune reprend
# la forme exacte qu'elle a dans les schémas épinglés (demo/registre.json).
SONDES = {
    "additionalProperties:false": {
        "type": "object",
        "properties": {"siren": {"type": "string", "description": "SIREN"}},
        "required": ["siren"],
        "additionalProperties": False,
    },
    "type:[null,array]": {
        "type": "object",
        "properties": {"ids": {"type": ["null", "array"], "items": {"type": "string"}}},
        "required": ["ids"],
    },
}


def requete(nom):
    messages = [{"role": "system", "content": "Réponds en un mot, sans appeler d'outil."},
                {"role": "user", "content": "Bonjour."}]
    outils = [{"type": "function", "function": {
        "name": "sonde", "description": "Fonction de sonde, ne pas appeler.", "parameters": SONDES[nom]}}]
    return messages, outils


def lire_verdicts(chemin=VERDICTS):
    try:
        with open(chemin, encoding="utf-8") as f:
            return json.load(f)
    except FileNotFoundError:
        return {}


def sonder(client, verdicts):
    """Complète ``verdicts`` pour les sondes qui n'en ont pas. Rend le
    nombre de sondes jouées."""
    jouees = 0
    for nom in SONDES:
        if nom in verdicts:
            continue
        messages, outils = requete(nom)
        try:
            client.completer(messages, outils, choix_outil="auto", options={"parallel_tool_calls": False})
            verdicts[nom] = {"accepte": True, "detail": "réponse 200"}
        except mistral.BudgetEpuise:
            raise
        except mistral.EnregistrementAbsent:
            continue
        except mistral.ErreurMistral as e:
            # Seul un 4xx est un refus du schéma ; une panne (5xx, réseau,
            # 429 répétés) ne donne pas de verdict. Le message porte le statut
            # HTTP et le début du corps d'erreur, jamais la clé ni les en-têtes.
            if not str(e).startswith("HTTP 4"):
                continue
            verdicts[nom] = {"accepte": False, "detail": str(e)[:300]}
        jouees += 1
    return jouees


def principal(argv=None, sortie=sys.stdout):
    ap = argparse.ArgumentParser(prog="boucle.sondes", description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--mode", choices=["rejeu", "reel"], default="rejeu")
    a = ap.parse_args(argv)
    verdicts = lire_verdicts()
    if a.mode == "reel":
        # Budget et compteur de la boucle ; réponses jetées (seul le verdict
        # compte), jamais mêlées aux enregistrements des scénarios.
        with tempfile.TemporaryDirectory() as tmp:
            sonder(mistral.Client(mode="reel", enregistrements=tmp, etat=ETAT, max_appels=MAX_APPELS), verdicts)
        with open(VERDICTS, "w", encoding="utf-8") as f:
            json.dump(verdicts, f, ensure_ascii=False, indent=1, sort_keys=True)
            f.write("\n")
    for nom in SONDES:
        v = verdicts.get(nom)
        sortie.write("%s : %s\n" % (nom, "sans verdict" if v is None else
                                    ("accepté" if v["accepte"] else "refusé (%s)" % v["detail"])))
    return 0


if __name__ == "__main__":
    sys.exit(principal())
