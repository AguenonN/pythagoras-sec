"""Lecture seule des exécutions passées (contrats de la démo, §2).

Une session est un dossier ``demo/execution/sessions/<nom>/`` : ``journal.jsonl``
(journal chaîné du gate), ``coffre.jsonl`` (arguments d'origine, par
empreinte), ``etat.json``. On n'en retient que les entrées ``appel`` : une
par présentation d'un appel à l'engine, avec la décision, le résultat, la
source déclarée et l'éventuelle approbation.

Les arguments viennent du coffre et ne servent qu'aux détecteurs : ils ne
sont jamais transmis au modèle (``redaction.vue_modele``).

Rien n'est écrit : ce module ouvre les fichiers en lecture et ne vérifie
pas la chaîne HMAC (la clé du journal n'est pas nécessaire pour lire ; la
vérification reste celle de ``journal verifier`` et du rejeu, ``mesure``).
"""

import datetime
import json
import os


class Appel:
    """Une entrée ``appel`` du journal, avec ses arguments d'origine."""

    def __init__(self, entree, arguments):
        self.seq = entree.get("seq")
        self.ts = entree.get("ts", "")
        self.serveur = entree.get("serveur", "")
        self.outil = entree.get("outil", "")
        self.acces = entree.get("acces", "")
        self.etiquettes_cible = list(entree.get("etiquettes_cible") or [])
        decision = entree.get("decision") or {}
        self.effet = decision.get("effet", "")
        self.applications = list(decision.get("applications") or [])
        resultat = entree.get("resultat") or {}
        self.statut = resultat.get("statut", "")
        self.source = resultat.get("source") or {}
        self.approbation = entree.get("approbation")
        self.arguments = arguments if isinstance(arguments, dict) else {}
        self.sous_agent = entree.get("sous_agent_declare", "")

    @property
    def nom(self):
        return self.serveur + "/" + self.outil

    @property
    def donnees_recues(self):
        return self.statut == "ok"

    @property
    def transmis(self):
        """L'appel est parti vers le serveur (quel que soit le résultat)."""
        return self.statut not in ("", "non_transmis")

    @property
    def validee_par_humain(self):
        a = self.approbation or {}
        return a.get("statut") == "acceptee"

    def directives_appliquees(self):
        return sorted(a["id"] for a in self.applications if a.get("appliquee"))

    def date(self):
        return lire_date(self.ts)


class Session:
    def __init__(self, nom, dossier, appels, etat):
        self.nom = nom
        self.dossier = dossier
        self.appels = appels
        self.etat = etat

    def consultations_avant(self, seq):
        """Appels de lecture partis avant ``seq``, données reçues."""
        return [a for a in self.appels if a.seq is not None and a.seq < seq
                and a.acces != "action" and a.donnees_recues]

    def actions(self):
        return [a for a in self.appels if a.acces == "action"]


def lire_date(texte):
    if not texte:
        return None
    t = texte.replace("Z", "+00:00")
    # Python < 3.11 n'accepte pas plus de six décimales.
    if "." in t:
        tete, queue = t.split(".", 1)
        chiffres = ""
        while queue and queue[0].isdigit():
            chiffres, queue = chiffres + queue[0], queue[1:]
        t = tete + "." + chiffres[:6].ljust(6, "0") + queue
    try:
        d = datetime.datetime.fromisoformat(t)
    except ValueError:
        return None
    if d.tzinfo is None:
        return None
    return d.astimezone(datetime.timezone.utc)


def _lignes(chemin):
    try:
        with open(chemin, encoding="utf-8") as f:
            for ligne in f:
                ligne = ligne.strip()
                if not ligne:
                    continue
                try:
                    yield json.loads(ligne)
                except ValueError:
                    continue
    except FileNotFoundError:
        return


def lire_session(dossier):
    coffre = {}
    for e in _lignes(os.path.join(dossier, "coffre.jsonl")):
        if "empreinte" in e:
            coffre[e["empreinte"]] = e.get("arguments_origine")
    appels = []
    for e in _lignes(os.path.join(dossier, "journal.jsonl")):
        if e.get("type") != "appel":
            continue
        empreinte = (e.get("appel") or {}).get("empreinte_origine", "")
        appels.append(Appel(e, coffre.get(empreinte)))
    appels.sort(key=lambda a: (a.seq is None, a.seq or 0))
    try:
        with open(os.path.join(dossier, "etat.json"), encoding="utf-8") as f:
            etat = json.load(f)
    except (FileNotFoundError, ValueError):
        etat = {}
    return Session(os.path.basename(os.path.normpath(dossier)), dossier, appels, etat)


def lire_sessions(racine):
    """Toutes les sessions de ``racine`` qui ont un journal, par nom."""
    out = []
    try:
        noms = sorted(os.listdir(racine))
    except FileNotFoundError:
        return out
    for nom in noms:
        dossier = os.path.join(racine, nom)
        if os.path.isfile(os.path.join(dossier, "journal.jsonl")):
            out.append(lire_session(dossier))
    return out
