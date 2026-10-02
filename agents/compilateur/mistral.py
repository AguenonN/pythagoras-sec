"""Client Mistral minimal : function calling, cache sur disque, budget, débit.

Chaque requête est identifiée par l'empreinte de son corps canonique. Une
réponse obtenue est enregistrée dans ``enregistrements/`` et ne sera plus
jamais redemandée : c'est à la fois le cache (courtoisie envers l'API) et le
jeu de réponses que les tests rejouent.

Deux modes :

- ``rejeu`` : uniquement les enregistrements ; une requête inconnue lève
  ``EnregistrementAbsent``. C'est le mode des tests : jamais d'appel réel.
- ``reel`` : les enregistrements d'abord, l'API sinon, dans la limite du
  budget (``MISTRAL_MAX_APPELS``, 40 par défaut) tenu dans ``.etat/``.

Aucune dépendance tierce : urllib. La clé est lue dans l'environnement et
n'apparaît jamais dans un message, une exception ou un fichier.
"""

import hashlib
import json
import os
import time
import urllib.error
import urllib.request

URL = "https://api.mistral.ai/v1/chat/completions"
MODELE = "mistral-small-latest"   # défaut du client, partagé avec la boucle et l'orchestrateur
MODELES = ("mistral-small-latest", "mistral-medium-latest")
# Modèle par défaut du compilateur de directives, retenu sur mesures
# (week-end et base interne, RAPPORT_B.md) : small inversait la condition du
# week-end et inventait des nœuds ; medium a rendu les deux jeux justes du
# premier coup.
MODELE_COMPILATEUR = "mistral-medium-latest"

ICI = os.path.dirname(os.path.abspath(__file__))
ENREGISTREMENTS = os.path.join(ICI, "enregistrements")
ETAT = os.path.join(os.path.dirname(ICI), ".etat")

INTERVALLE_MIN = 1.1  # secondes entre deux appels réels : au plus 1 par seconde
TENTATIVES = 3


class ErreurMistral(Exception):
    """L'API n'a pas rendu de réponse exploitable."""


class EnregistrementAbsent(ErreurMistral):
    """Mode rejeu : aucune réponse enregistrée pour cette requête."""


class BudgetEpuise(ErreurMistral):
    """Le nombre maximal d'appels réels est atteint."""


def empreinte(corps):
    canon = json.dumps(corps, sort_keys=True, ensure_ascii=False, separators=(",", ":"))
    return hashlib.sha256(canon.encode("utf-8")).hexdigest()


class Client:
    def __init__(self, mode="rejeu", enregistrements=ENREGISTREMENTS, etat=ETAT,
                 max_appels=None, transport=None, dormir=time.sleep, modele=MODELE):
        if mode not in ("rejeu", "reel"):
            raise ValueError("mode inconnu : %r" % mode)
        if modele not in MODELES:
            raise ValueError("modèle inconnu : %r (%s)" % (modele, ", ".join(MODELES)))
        self.mode = mode
        self.modele = modele
        self.enregistrements = enregistrements
        self.etat = etat
        if max_appels is None:
            max_appels = int(os.environ.get("MISTRAL_MAX_APPELS", "40"))
        self.max_appels = max_appels
        self.transport = transport or _poster
        self.dormir = dormir
        self._dernier = 0.0

    def completer(self, messages, outils, choix_outil="any", options=None):
        """``options`` complète le corps (parallel_tool_calls…) ; absentes, la
        requête et donc son empreinte sont celles d'avant leur ajout."""
        corps = {
            "model": self.modele,
            "temperature": 0,
            "random_seed": 0,
            "messages": messages,
            "tools": outils,
            "tool_choice": choix_outil,
        }
        for cle, valeur in (options or {}).items():
            if cle in corps:
                raise ValueError("option %r fixée par le client" % cle)
            corps[cle] = valeur
        cle = empreinte(corps)
        chemin = os.path.join(self.enregistrements, cle + ".json")
        if os.path.exists(chemin):
            with open(chemin, encoding="utf-8") as f:
                return json.load(f)["reponse"]
        if self.mode == "rejeu":
            raise EnregistrementAbsent("aucune réponse enregistrée pour la requête " + cle[:12])
        reponse = self._appeler(corps)
        os.makedirs(self.enregistrements, exist_ok=True)
        with open(chemin, "w", encoding="utf-8") as f:
            json.dump({"requete": corps, "reponse": reponse}, f, ensure_ascii=False, indent=1, sort_keys=True)
            f.write("\n")
        return reponse

    # -- appels réels -------------------------------------------------------

    def _budget(self):
        chemin = os.path.join(self.etat, "budget.json")
        try:
            with open(chemin, encoding="utf-8") as f:
                return json.load(f)
        except FileNotFoundError:
            return {"appels": 0, "dernier": 0.0}

    def _noter(self, budget):
        os.makedirs(self.etat, exist_ok=True)
        with open(os.path.join(self.etat, "budget.json"), "w", encoding="utf-8") as f:
            json.dump(budget, f)

    def _journal(self, corps, issue):
        """Une ligne par appel réel (``.etat/appels.jsonl``) : modèle,
        empreinte de la requête, issue. Ni la clé ni le contenu."""
        os.makedirs(self.etat, exist_ok=True)
        with open(os.path.join(self.etat, "appels.jsonl"), "a", encoding="utf-8") as f:
            f.write(json.dumps({"date": time.strftime("%Y-%m-%dT%H:%M:%S"), "modele": corps.get("model"),
                                "requete": empreinte(corps)[:12], "issue": issue[:200]}, ensure_ascii=False) + "\n")

    def _appeler(self, corps):
        cle = os.environ.get("MISTRAL_API_KEY", "")
        if not cle:
            raise ErreurMistral("MISTRAL_API_KEY absente de l'environnement")
        derniere_erreur = None
        for tentative in range(TENTATIVES):
            budget = self._budget()
            if budget["appels"] >= self.max_appels:
                raise BudgetEpuise("budget de %d appels réels atteint" % self.max_appels)
            attente = INTERVALLE_MIN - (time.time() - max(self._dernier, budget.get("dernier", 0.0)))
            if attente > 0:
                self.dormir(attente)
            budget["appels"] += 1
            budget["dernier"] = self._dernier = time.time()
            self._noter(budget)
            try:
                reponse = self.transport(corps, cle)
                if _fin_en_erreur(reponse):
                    # Génération interrompue côté Mistral : ce n'est pas une
                    # réponse, elle n'est ni rendue ni enregistrée.
                    raise ErreurTransitoire("génération interrompue (finish_reason: error)")
                self._journal(corps, "ok")
                return reponse
            except ErreurTransitoire as e:
                self._journal(corps, str(e))
                derniere_erreur = e
                self.dormir(2 ** (tentative + 1))
            except ErreurMistral as e:
                self._journal(corps, str(e))
                raise
        raise ErreurMistral("échec après %d tentatives : %s" % (TENTATIVES, derniere_erreur))


def _fin_en_erreur(reponse):
    try:
        return reponse["choices"][0].get("finish_reason") == "error"
    except (KeyError, IndexError, TypeError, AttributeError):
        return False


class ErreurTransitoire(ErreurMistral):
    """Erreur qui justifie une nouvelle tentative (429, 5xx, réseau)."""


def _poster(corps, cle):
    requete = urllib.request.Request(
        URL,
        data=json.dumps(corps).encode("utf-8"),
        headers={
            "Authorization": "Bearer " + cle,
            "Content-Type": "application/json",
            "Accept": "application/json",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(requete, timeout=60) as r:
            return json.loads(r.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        # Le corps d'erreur de l'API ne contient pas la clé ; on le tronque
        # tout de même, et on ne recopie jamais les en-têtes.
        detail = e.read().decode("utf-8", "replace")[:500]
        if e.code == 429 or e.code >= 500:
            raise ErreurTransitoire("HTTP %d : %s" % (e.code, detail)) from None
        raise ErreurMistral("HTTP %d : %s" % (e.code, detail)) from None
    except (urllib.error.URLError, TimeoutError) as e:
        raise ErreurTransitoire("réseau : %s" % getattr(e, "reason", e)) from None
