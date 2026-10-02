"""Lecture du journal du gate par l'orchestrateur : jamais pour décider,
seulement pour citer et comparer.

- ``directives_attente`` : les directives appliquées qui mettent un appel
  en attente (la réponse de l'engine ne nomme que la première) ;
- ``comparer`` : deux journaux d'une même demande (passage réel et rejeu
  hors ligne) sont-ils les mêmes, à l'horodatage près ? Sont ignorés ce
  qui change nécessairement d'un passage à l'autre : dates, session,
  chaînage (prev, mac), durées, empreintes des réponses des serveurs et
  provenance de leurs données (direct en réel, rejeu hors ligne),
  empreintes des jetons (signés à nouveau).

La chaîne elle-même se vérifie avec ``journal verifier`` et ``journal
rejouer`` (cmd/journal), qui ont la clé.
"""

import json

IGNORES = {"ts", "session", "prev", "mac", "duree_us", "duree_ms", "empreinte_reponse", "source", "date",
           "empreinte_jeton"}


def lire(chemin):
    with open(chemin, encoding="utf-8") as f:
        return [json.loads(l) for l in f if l.strip()]


def directives_attente(chemin, empreinte):
    try:
        entrees = lire(chemin)
    except (OSError, ValueError):
        return []
    for e in reversed(entrees):
        if e.get("type") != "appel" or (e.get("appel") or {}).get("empreinte_transmise") != empreinte:
            continue
        apps = (e.get("decision") or {}).get("applications") or []
        return [a["id"] for a in apps if a.get("appliquee") and a.get("effet") == "attendre_validation"]
    return []


def normaliser(x):
    if isinstance(x, dict):
        return {k: normaliser(v) for k, v in sorted(x.items()) if k not in IGNORES}
    if isinstance(x, list):
        return [normaliser(v) for v in x]
    return x


def comparer(chemin1, chemin2):
    a, b = lire(chemin1), lire(chemin2)
    ecarts = []
    if len(a) != len(b):
        ecarts.append("nombre d'entrées : %d contre %d" % (len(a), len(b)))
    for i, (x, y) in enumerate(zip(a, b)):
        nx, ny = normaliser(x), normaliser(y)
        if nx != ny:
            cles = sorted(k for k in set(nx) | set(ny) if nx.get(k) != ny.get(k))
            ecarts.append("entrée %d (%s / %s) : diffère sur %s" % (i + 1, x.get("type"), y.get("type"), ", ".join(cles)))
    return ecarts
