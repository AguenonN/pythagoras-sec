"""Mesure d'une proposition : combien d'appels passés auraient changé de
décision si la directive avait existé.

- Sans décision du juriste, ou tant que son texte n'est pas compilé : pas de
  mesure, seulement les sessions concernées (celles de la proposition).
- Rejetée : rien à mesurer.
- Reprise (modifiée ou telle quelle) et compilée : le jeu candidat de la
  conversation du compilateur (``demo/execution/compilation/<conv>/``,
  dernière version) est rejoué sur le journal de chaque session concernée,
  et sur celui de toute autre session du dossier (les appels passés, pas
  seulement ceux du constat), par ``journal rejouer`` (chaîne vérifiée, clé ``MCP_GATE_CLE_JOURNAL``
  lue dans l'environnement). Dans une **copie temporaire** du candidat, les
  directives ajoutées passent de ``brouillon`` à ``active`` avec une date
  d'effet antérieure aux journaux : c'est la question « si elle avait
  existé ». Le candidat lui-même n'est jamais modifié, rien n'est activé.

Le résultat est écrit dans ``demo/execution/analyste/mesures/<id>.json``,
jamais dans le dossier des propositions (la console lirait tout ``.json``
de ce dossier comme une proposition).
"""

import datetime
import glob
import json
import os
import re
import subprocess
import tempfile

ICI = os.path.dirname(os.path.abspath(__file__))
RACINE = os.path.dirname(os.path.dirname(ICI))
DATE_SI_AVAIT_EXISTE = "2000-01-01T00:00:00Z"


def _rel(chemin, racine):
    r = os.path.relpath(chemin, racine)
    return chemin if r.startswith("..") else r


def _lire(chemin):
    try:
        with open(chemin, encoding="utf-8") as f:
            return json.load(f)
    except FileNotFoundError:
        return None


def _lister(sessions):
    try:
        return [n for n in os.listdir(sessions) if os.path.isfile(os.path.join(sessions, n, "journal.jsonl"))]
    except FileNotFoundError:
        return []


def jeu_candidat(dossier_compilation, conversation):
    """Dernière version du jeu candidat d'une conversation, ou None."""
    if not conversation or "/" in conversation or conversation.startswith("."):
        return None
    meilleurs = []
    for p in glob.glob(os.path.join(dossier_compilation, conversation, "*.v*.json")):
        m = re.search(r"\.v(\d+)\.json$", p)
        if m:
            meilleurs.append((int(m.group(1)), p))
    return max(meilleurs)[1] if meilleurs else None


def jeu_si_avait_existe(candidat, base):
    """Copie du candidat où les directives absentes de ``base`` sont
    actives depuis toujours. Rend (jeu, ids ajoutés)."""
    pris = {d.get("id") for d in (base or {}).get("directives", [])}
    jeu = json.loads(json.dumps(candidat))
    for k in ("empreinte", "signature"):
        jeu["jeu"].pop(k, None)
    ajoutees = []
    for d in jeu["directives"]:
        if d.get("id") in pris:
            continue
        ajoutees.append(d["id"])
        d["statut"] = "active"
        d["date_effet"] = DATE_SI_AVAIT_EXISTE
        d.pop("revision_avant", None)
    return jeu, ajoutees


RE_LIGNE = re.compile(r"^(\S+)\s+(\d+)\s+(\S+)\s+(\S+)\s+(\S+)\s+←\s*$")


def lire_avant_apres(sortie):
    """Les lignes « ← » du tableau Avant / après de ``journal rejouer``."""
    out, dans = [], False
    for ligne in sortie.splitlines():
        if ligne.startswith("Avant / après"):
            dans = True
            continue
        if dans and ligne.startswith("Transitions"):
            break
        m = RE_LIGNE.match(ligne.rstrip()) if dans else None
        if m:
            out.append({"seq": int(m.group(2)), "outil": m.group(3), "avant": m.group(4), "apres": m.group(5)})
    return out


def rejouer(binaire, session_dir, jeu_avant, jeu_apres, racine=RACINE):
    args = [binaire, "rejouer", os.path.join(session_dir, "journal.jsonl"), "-directives", jeu_avant, jeu_apres]
    coffre = os.path.join(session_dir, "coffre.jsonl")
    if os.path.exists(coffre):
        args += ["-coffre", coffre]
    annuaire = os.path.join(racine, "demo", "annuaire.json")
    autorite = os.path.join(racine, "demo", "execution", "cles", "autorite.pub")
    if os.path.exists(annuaire) and os.path.exists(autorite):
        args += ["-annuaire", annuaire, "-annuaire-autorite", autorite]
    r = subprocess.run(args, capture_output=True, text=True, timeout=120)
    return r.returncode, r.stdout, r.stderr


def mesurer(id_, racine=RACINE, propositions=None, sessions=None, compilation=None, binaire=None, jeu_courant=None):
    propositions = propositions or os.path.join(racine, "demo", "execution", "propositions")
    sessions = sessions or os.path.join(racine, "demo", "execution", "sessions")
    compilation = compilation or os.path.join(racine, "demo", "execution", "compilation")
    binaire = binaire or os.path.join(racine, "bin", "journal")
    jeu_courant = jeu_courant or os.path.join(racine, "demo", "directives", "demo.json")
    prop = _lire(os.path.join(propositions, id_ + ".json"))
    if prop is None:
        raise FileNotFoundError("proposition %s introuvable" % id_)
    concernees = prop.get("sessions", [])
    base = {"proposition": id_, "date": datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0)
            .isoformat().replace("+00:00", "Z"), "sessions_concernees": concernees}
    dec = _lire(os.path.join(propositions, id_ + ".decision.json"))
    if dec is None:
        return dict(base, etat="avant_decision",
                    message="Pas encore de décision du juriste : sessions concernées seulement.")
    if dec.get("decision") == "rejetee":
        return dict(base, etat="rejetee", message="Proposition rejetée par le juriste : rien à mesurer.")
    chemin = jeu_candidat(compilation, dec.get("conversation_compilateur", ""))
    if chemin is None:
        return dict(base, etat="avant_compilation",
                    message="Texte repris mais pas encore compilé en jeu candidat : sessions concernées seulement.")
    if not os.environ.get("MCP_GATE_CLE_JOURNAL"):
        return dict(base, etat="cle_absente", jeu_candidat=_rel(chemin, racine),
                    message="MCP_GATE_CLE_JOURNAL absente : le rejeu vérifie la chaîne, il ne se fait pas sans clé.")
    jeu, ajoutees = jeu_si_avait_existe(_lire(chemin), _lire(jeu_courant))
    changes, rejouees, erreurs = [], [], []
    with tempfile.TemporaryDirectory() as tmp:
        apres = os.path.join(tmp, "si-avait-existe.json")
        with open(apres, "w", encoding="utf-8") as f:
            json.dump(jeu, f, ensure_ascii=False)
        noms = sorted(set(concernees) | {n for n in _lister(sessions)})
        for nom in noms:
            d = os.path.join(sessions, nom)
            if not os.path.isfile(os.path.join(d, "journal.jsonl")):
                erreurs.append("%s : journal absent" % nom)
                continue
            avant = os.path.join(d, "jeu.json")
            avant = avant if os.path.exists(avant) else jeu_courant
            code, out, err = rejouer(binaire, d, avant, apres, racine)
            if code != 0 and "Avant / après" not in out:
                erreurs.append("%s : rejeu impossible (%s)" % (nom, (err.strip().splitlines() or ["code %d" % code])[-1]))
                continue
            rejouees.append(nom)
            for ligne in lire_avant_apres(out):
                changes.append(dict(ligne, session=nom))
    return dict(base, etat="mesuree", decision=dec.get("decision"), jeu_candidat=_rel(chemin, racine),
                directives_ajoutees=ajoutees, sessions_rejouees=rejouees, appels_changes=len(changes),
                changements=changes, erreurs=erreurs)


def ecrire(resultat, racine=RACINE):
    dossier = os.path.join(racine, "demo", "execution", "analyste", "mesures")
    os.makedirs(dossier, exist_ok=True)
    chemin = os.path.join(dossier, resultat["proposition"] + ".json")
    tmp = chemin + ".part"
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(resultat, f, ensure_ascii=False, indent=1)
        f.write("\n")
    os.replace(tmp, chemin)
    return chemin

