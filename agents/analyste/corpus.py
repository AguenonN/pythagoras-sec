"""Petit corpus local et versionné des fondements (``analyste/corpus/``).

Les articles déjà cités comme fondement dans ``demo/directives/demo.json``
et dans les briefs, en texte public, tirés **par les serveurs MCP du
projet** (``legifrance``, ``eurlex``) : en ``MIXTE`` si les identifiants
PISTE sont dans l'environnement, sinon en ``REJEU`` sur leurs
enregistrements. Aucun texte n'est écrit de mémoire : un article que ni la
source ni un enregistrement ne rend est consigné comme indisponible.

    cd agents
    python -m analyste.corpus          # (re)construit analyste/corpus/

Le corpus sert à **suggérer** des fondements au juriste ; il n'entre ni
dans l'engine ni dans une décision du gate.
"""

import hashlib
import json
import os
import sys

from boucle.mcp import ClientMCP

ICI = os.path.dirname(os.path.abspath(__file__))
RACINE = os.path.dirname(os.path.dirname(ICI))
DOSSIER = os.path.join(ICI, "corpus")
INDEX = os.path.join(DOSSIER, "index.json")
DATE_REFERENCE = "2026-10-02"

# Fondements cités (demo.json, briefs). « lien » : la page publique de
# l'article, que le juriste ouvre ; l'URL d'API consultée reste dans la
# source.
ARTICLES = [
    {"id": "CMF-L313-5-1", "code": "Code monétaire et financier", "article": "L313-5-1",
     "cite_par": ["D-002", "D-009"], "serveur": "legifrance", "outil": "lire_article",
     "arguments": {"identifiant": "LEGIARTI000027643291", "date_reference": DATE_REFERENCE},
     "lien": "https://www.legifrance.gouv.fr/codes/article_lc/LEGIARTI000027643291"},
    {"id": "CONSO-L314-6", "code": "Code de la consommation", "article": "L314-6",
     "cite_par": ["servers/taux-officiel (seuil de l'usure)"], "serveur": "legifrance", "outil": "lire_article",
     "arguments": {"identifiant": "LEGIARTI000032303335", "date_reference": DATE_REFERENCE},
     "lien": "https://www.legifrance.gouv.fr/codes/article_lc/LEGIARTI000032303335"},
    {"id": "RGPD-5", "code": "Règlement (UE) 2016/679 (RGPD)", "article": "5",
     "cite_par": ["D-004"], "serveur": "eurlex", "outil": "lire_article",
     "arguments": {"celex": "32016R0679", "article": "5"},
     "lien": "https://eur-lex.europa.eu/legal-content/FR/TXT/?uri=CELEX:32016R0679"},
    {"id": "RGPD-22", "code": "Règlement (UE) 2016/679 (RGPD)", "article": "22",
     "cite_par": ["docs/orchestration/03_session_agents_hors_ligne.md"], "serveur": "eurlex", "outil": "lire_article",
     "arguments": {"celex": "32016R0679", "article": "22"},
     "lien": "https://eur-lex.europa.eu/legal-content/FR/TXT/?uri=CELEX:32016R0679"},
    {"id": "RGPD-32", "code": "Règlement (UE) 2016/679 (RGPD)", "article": "32",
     "cite_par": ["D-008"], "serveur": "eurlex", "outil": "lire_article",
     "arguments": {"celex": "32016R0679", "article": "32"},
     "lien": "https://eur-lex.europa.eu/legal-content/FR/TXT/?uri=CELEX:32016R0679"},
    # Identifiants LEGIARTI retrouvés le 03/10 (session gel) par l'API officielle
    # Légifrance (consult/getArticleWithIdAndNum, version en vigueur) ; le texte
    # est lu par le serveur legifrance.
    {"id": "CMF-L562-4", "code": "Code monétaire et financier", "article": "L562-4", "cite_par": ["D-001"],
     "serveur": "legifrance", "outil": "lire_article", "arguments": {"identifiant": "LEGIARTI000045250683", "date_reference": DATE_REFERENCE},
     "lien": "https://www.legifrance.gouv.fr/codes/article_lc/LEGIARTI000045250683",
     "raison_si_absent": "identifiant LEGIARTI inconnu du dépôt et aucun enregistrement ; PISTE requis"},
    {"id": "CT-L1121-1", "code": "Code du travail", "article": "L1121-1", "cite_par": ["D-006"],
     "serveur": "legifrance", "outil": "lire_article", "arguments": {"identifiant": "LEGIARTI000006900785", "date_reference": DATE_REFERENCE},
     "lien": "https://www.legifrance.gouv.fr/codes/article_lc/LEGIARTI000006900785",
     "raison_si_absent": "identifiant LEGIARTI inconnu du dépôt et aucun enregistrement ; PISTE requis"},
    {"id": "CT-L2411-1", "code": "Code du travail", "article": "L2411-1", "cite_par": ["D-007"],
     "serveur": "legifrance", "outil": "lire_article", "arguments": {"identifiant": "LEGIARTI000035652370", "date_reference": DATE_REFERENCE},
     "lien": "https://www.legifrance.gouv.fr/codes/article_lc/LEGIARTI000035652370",
     "raison_si_absent": "identifiant LEGIARTI inconnu du dépôt et aucun enregistrement ; PISTE requis"},
]
# Non publics, donc hors corpus : « Politique de sécurité des sources de
# données » art. 3 (D-003), « Politique de crédit » art. 7.1 (D-005).


def _mode():
    if os.environ.get("PISTE_CLIENT_ID") and os.environ.get("PISTE_CLIENT_SECRET"):
        return "MIXTE"
    return "REJEU"


def _env(mode):
    env = {"PATH": os.environ.get("PATH", ""), "DONNEES_MODE": mode}
    for v in ("PISTE_CLIENT_ID", "PISTE_CLIENT_SECRET"):
        if os.environ.get(v):
            env[v] = os.environ[v]
    return env


def _texte(serveur, sc):
    if serveur == "legifrance":
        return sc.get("contenu", "")
    return sc.get("texte", "")


def lire(article, mode, racine=RACINE):
    """Rend (entrée du corpus, None) ou (None, raison)."""
    if article["arguments"] is None:
        return None, article["raison_si_absent"]
    binaire = os.path.join(racine, "bin", article["serveur"])
    if not os.path.exists(binaire):
        return None, "bin/%s absent (scripts/construire-serveurs.sh)" % article["serveur"]
    argv = [binaire, "-enregistrements", os.path.join(racine, "servers", article["serveur"], "enregistrements")]
    with ClientMCP(argv, env=_env(mode)) as c:
        res = c.appeler(article["outil"], article["arguments"])
    sc = (res or {}).get("structuredContent") or {}
    texte = _texte(article["serveur"], sc).strip()
    if (res or {}).get("isError") or not texte:
        return None, "le serveur %s n'a pas rendu le texte (mode %s)" % (article["serveur"], mode)
    src = sc.get("source") or {}
    entree = {k: article[k] for k in ("id", "code", "article", "cite_par", "lien")}
    entree.update({"texte": texte, "source": {
        "serveur": article["serveur"], "outil": article["outil"], "arguments": article["arguments"],
        "mode": mode, "origine": src.get("origine", ""), "date": src.get("date", ""),
        "url_consultee": src.get("url", ""), "sha256": src.get("sha256", "")}})
    if article["serveur"] == "legifrance":
        entree["en_vigueur"] = sc.get("en_vigueur")
    return entree, None


def empreinte(entrees):
    canon = json.dumps([{"id": e["id"], "texte": e["texte"]} for e in entrees], sort_keys=True,
                       ensure_ascii=False, separators=(",", ":"))
    return "sha256:" + hashlib.sha256(canon.encode("utf-8")).hexdigest()


def construire(dossier=DOSSIER, racine=RACINE):
    mode = _mode()
    entrees, indisponibles = [], []
    for a in ARTICLES:
        e, raison = lire(a, mode, racine)
        if e is None:
            indisponibles.append({"id": a["id"], "code": a["code"], "article": a["article"],
                                  "cite_par": a["cite_par"], "raison": raison})
            continue
        entrees.append(e)
    os.makedirs(dossier, exist_ok=True)
    for e in entrees:
        with open(os.path.join(dossier, e["id"] + ".json"), "w", encoding="utf-8") as f:
            json.dump(e, f, ensure_ascii=False, indent=1)
            f.write("\n")
    index = {"commentaire": "Corpus des fondements suggérés à l'Analyste. Suggestions seulement, à valider par le "
                            "juriste ; rien n'entre dans l'engine.",
             "date_reference": DATE_REFERENCE, "mode": mode, "articles": [e["id"] for e in entrees],
             "empreinte": empreinte(entrees), "indisponibles": indisponibles}
    with open(os.path.join(dossier, "index.json"), "w", encoding="utf-8") as f:
        json.dump(index, f, ensure_ascii=False, indent=1)
        f.write("\n")
    return index


def charger(dossier=DOSSIER):
    """Les articles du corpus, dans l'ordre de l'index ; l'empreinte est
    vérifiée (un corpus retouché à la main ne se charge pas)."""
    with open(os.path.join(dossier, "index.json"), encoding="utf-8") as f:
        index = json.load(f)
    entrees = []
    for i in index["articles"]:
        with open(os.path.join(dossier, i + ".json"), encoding="utf-8") as f:
            entrees.append(json.load(f))
    if empreinte(entrees) != index["empreinte"]:
        raise ValueError("corpus : empreinte différente de celle de l'index")
    return entrees, index


if __name__ == "__main__":
    json.dump(construire(), sys.stdout, ensure_ascii=False, indent=1)
    sys.stdout.write("\n")
