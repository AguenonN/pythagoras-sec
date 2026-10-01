"""Scénarios enregistrés du compilateur v2 : un texte de règle de la démo, les
réponses du juriste, et le déroulé jusqu'au jeu en brouillon.

Un scénario (``exemples/scenarios-v2/<nom>.json``) ::

    {"texte": "...", "jeu": "weekend", "auteur": "juriste@cabinet.fr",
     "date_effet": "2026-10-05T00:00:00Z",
     "reponses": [{"question": "<question exacte de Mistral>", "reponse": "..."},
                  {"champ": "si_indetermine", "reponse": "refuser"}]}

Les réponses aux questions de Mistral sont retrouvées par le texte exact de
la question ; celles aux questions du code, par leur ``champ``. ``derouler``
répond tour après tour, dans l'ordre des questions posées, et s'arrête sur
une question sans réponse : c'est ainsi que les réponses ont été écrites, en
mode réel, puis rejouées par les tests sans aucun appel.

    python -m compilateur.scenarios_v2 weekend [--mode reel]
"""

import argparse
import json
import os
import sys

from . import compilateur_v2 as c2
from . import mistral, vocabulaire

ICI = os.path.dirname(os.path.abspath(__file__))
SCENARIOS = os.path.join(ICI, "exemples", "scenarios-v2")
CATALOGUE = os.path.join(ICI, "exemples", "catalogue-demo.json")
COMPLEMENT = os.path.join(ICI, "exemples", "vocabulaire-demo.json")
TOURS_MAX = 5


def lire(nom):
    with open(os.path.join(SCENARIOS, nom + ".json"), encoding="utf-8") as f:
        return json.load(f)


def noms():
    return sorted(f[:-5] for f in os.listdir(SCENARIOS) if f.endswith(".json"))


def vocab_demo():
    return vocabulaire.Vocabulaire.lire(CATALOGUE, COMPLEMENT)


def _reponse(scenario, q):
    for r in scenario.get("reponses", []):
        if q.get("origine") == c2.ORIGINE_CODE:
            if r.get("champ") == q["champ"]:
                return r["reponse"]
        elif (r.get("question") or "").strip() == q["question"].strip():
            return r["reponse"]
    return None


def derouler(scenario, client, vocab=None):
    """Rend (résultat final, tours) ; chaque tour garde l'état et les
    questions posées. S'arrête au jeu, au refus, ou sur une question sans
    réponse dans le scénario (état questions ou non_observable)."""
    vocab = vocab or vocab_demo()
    reponses, tours = [], []
    for _ in range(TOURS_MAX):
        r = c2.compiler_jeu(scenario["texte"], scenario["jeu"], vocab, client, scenario["auteur"],
                            scenario["date_effet"], reponses=reponses, prefixe_id=scenario.get("prefixe_id", "D"))
        tours.append({"etat": r["etat"], "questions": r["questions"], "non_observables": r["non_observables"]})
        if r["etat"] not in (c2.QUESTIONS, c2.NON_OBSERVABLE):
            return r, tours
        nouvelles = []
        for q in r["questions"]:
            rep = _reponse(scenario, q)
            if rep is None:
                return r, tours
            entree = {"question": q["question"], "reponse": rep}
            if q.get("origine"):
                entree.update(champ=q["champ"], options=q["options"], origine=q["origine"])
            if entree not in reponses:
                nouvelles.append(entree)
        if not nouvelles:
            return r, tours
        reponses += nouvelles
    return r, tours


def principal(argv=None, sortie=sys.stdout):
    ap = argparse.ArgumentParser(prog="compilateur.scenarios_v2")
    ap.add_argument("nom", choices=noms())
    ap.add_argument("--mode", choices=["rejeu", "reel"], default="rejeu")
    ap.add_argument("--etat", help="répertoire du compteur d'appels réels")
    a = ap.parse_args(argv)
    options = {"etat": a.etat} if a.etat else {}
    r, tours = derouler(lire(a.nom), mistral.Client(mode=a.mode, **options))
    for i, t in enumerate(tours, 1):
        sortie.write("Tour %d : %s\n" % (i, t["etat"]))
        for n in t["non_observables"]:
            sortie.write("  non observable : %s — %s\n" % (n["extrait"], n["raison"]))
        for q in t["questions"]:
            sortie.write("  ? [%s] %s %s\n" % (q.get("champ") or q.get("sujet"), q["question"],
                                               json.dumps(q.get("options") or [], ensure_ascii=False)))
    if r["etat"] == c2.JEU:
        sortie.write("\n" + json.dumps(r["jeu"], ensure_ascii=False, indent=2) + "\n\n" + r["relecture"])
    elif r["etat"] in (c2.REFUSE, c2.ERREURS):
        sortie.write("Erreurs :\n" + "".join("- %s\n" % e for e in r["erreurs"]))
        if r.get("jeu_refuse"):
            sortie.write(json.dumps(r["jeu_refuse"], ensure_ascii=False, indent=2) + "\n")
    return 0 if r["etat"] == c2.JEU else 1


if __name__ == "__main__":
    sys.exit(principal())
