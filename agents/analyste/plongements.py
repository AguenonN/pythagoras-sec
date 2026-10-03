"""Embeddings Mistral (``mistral-embed``), enregistrés comme les autres
réponses : un fichier par texte, ``enregistrements/plongements/<empreinte>.json``,
où l'empreinte est celle du modèle et du texte. En ``rejeu`` (défaut, et
toujours dans les tests), seuls les enregistrements servent ; un texte
inconnu lève ``EnregistrementAbsent``. En ``reel``, les textes manquants
sont demandés en un seul appel par lot, dans le budget de la suite.

Les embeddings servent à **proposer** (sessions similaires, fondements
suggérés), jamais à décider : rien de ce module n'entre dans l'engine.
"""

import hashlib
import json
import math
import os
import urllib.error
import urllib.request

from compilateur import mistral

from .redaction import ETAT_SUITE, MAX_APPELS_SUITE  # budget de la suite, embeddings compris

ICI = os.path.dirname(os.path.abspath(__file__))
ENREGISTREMENTS = os.path.join(ICI, "enregistrements", "plongements")
MODELE = "mistral-embed"
URL = "https://api.mistral.ai/v1/embeddings"
LOT = 32  # textes par appel


def empreinte(texte, modele=MODELE):
    return hashlib.sha256((modele + "\n" + texte).encode("utf-8")).hexdigest()


def _poster(corps, cle):
    requete = urllib.request.Request(URL, data=json.dumps(corps).encode("utf-8"), method="POST", headers={
        "Authorization": "Bearer " + cle, "Content-Type": "application/json", "Accept": "application/json"})
    try:
        with urllib.request.urlopen(requete, timeout=60) as r:
            return json.loads(r.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        detail = e.read().decode("utf-8", "replace")[:500]
        if e.code == 429 or e.code >= 500:
            raise mistral.ErreurTransitoire("HTTP %d : %s" % (e.code, detail)) from None
        raise mistral.ErreurMistral("HTTP %d : %s" % (e.code, detail)) from None
    except (urllib.error.URLError, TimeoutError) as e:
        raise mistral.ErreurTransitoire("réseau : %s" % getattr(e, "reason", e)) from None


class Plongeur:
    def __init__(self, mode="rejeu", enregistrements=ENREGISTREMENTS, etat=ETAT_SUITE,
                 max_appels=MAX_APPELS_SUITE, transport=None):
        if mode not in ("rejeu", "reel"):
            raise ValueError("mode inconnu : %r" % mode)
        self.mode = mode
        self.enregistrements = enregistrements
        # Le client du compilateur tient le budget, le débit et les
        # tentatives ; seul le transport change (point d'accès embeddings).
        self._client = mistral.Client(mode="reel", enregistrements=enregistrements, etat=etat,
                                      max_appels=max_appels, transport=transport or _poster)
        self.appels = 0

    def _chemin(self, texte):
        return os.path.join(self.enregistrements, empreinte(texte) + ".json")

    def _lire(self, texte):
        try:
            with open(self._chemin(texte), encoding="utf-8") as f:
                return json.load(f)["vecteur"]
        except FileNotFoundError:
            return None

    def vecteurs(self, textes):
        """Un vecteur par texte, dans l'ordre."""
        out = {t: self._lire(t) for t in textes}
        manquants = sorted({t for t, v in out.items() if v is None})
        if manquants and self.mode == "rejeu":
            raise mistral.EnregistrementAbsent("%d texte(s) sans embedding enregistré (%s…)"
                                               % (len(manquants), empreinte(manquants[0])[:12]))
        for i in range(0, len(manquants), LOT):
            lot = manquants[i:i + LOT]
            reponse = self._client._appeler({"model": MODELE, "input": lot})
            self.appels += 1
            donnees = sorted(reponse.get("data") or [], key=lambda d: d.get("index", 0))
            if len(donnees) != len(lot):
                raise mistral.ErreurMistral("embeddings : %d vecteur(s) pour %d texte(s)" % (len(donnees), len(lot)))
            os.makedirs(self.enregistrements, exist_ok=True)
            for texte, d in zip(lot, donnees):
                with open(self._chemin(texte), "w", encoding="utf-8") as f:
                    json.dump({"modele": MODELE, "empreinte_texte": empreinte(texte), "extrait": texte[:120],
                               "vecteur": d["embedding"]}, f)
                    f.write("\n")
                out[texte] = d["embedding"]
        return [out[t] for t in textes]


def cosinus(a, b):
    na = math.sqrt(sum(x * x for x in a))
    nb = math.sqrt(sum(x * x for x in b))
    if not na or not nb:
        return 0.0
    return sum(x * y for x, y in zip(a, b)) / (na * nb)
