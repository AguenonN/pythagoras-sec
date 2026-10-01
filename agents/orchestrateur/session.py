"""Dossier d'une demande (contrats-demo.md §2) et attente des jetons de
validation (§3).

    demo/execution/sessions/<AAAAMMJJ-HHMMSS>-<court>/
        journal.jsonl, coffre.jsonl, notifications.jsonl   écrits par le gate
        etat.json      réécrit atomiquement à chaque changement
        reponse.json   à la fin
    demo/execution/approbations/<empreinte_transmise>/*.jwt  un fichier par valideur
"""

import base64
import glob
import json
import os
import re
import time

STATUTS = ("en_cours", "en_attente", "termine", "bloque", "erreur")
DELAI_VALIDATION = 600  # secondes : 10 minutes au plus (§3)
INTERVALLE = 1.0
MAX_JETONS = 8  # au plus 8 jetons par reprise (contrat de l'engine §6)


def ecrire_atomique(chemin, donnees):
    tmp = "%s.%d.tmp" % (chemin, os.getpid())
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(donnees, f, ensure_ascii=False, indent=1)
        f.write("\n")
    os.replace(tmp, chemin)


def nom_court(texte):
    t = re.sub(r"[^a-z0-9]+", "-", (texte or "demande").lower()).strip("-")
    return (t or "demande")[:24].strip("-")


class DossierSession:
    def __init__(self, chemin, demande, demandeur, debut):
        self.chemin = chemin
        os.makedirs(chemin, exist_ok=True)
        self.etat = {"statut": "en_cours", "demande": demande, "demandeur": demandeur,
                     "debut": debut, "fin": None, "attente": None}
        self._ecrire_etat()

    def fichier(self, nom):
        return os.path.join(self.chemin, nom)

    def _ecrire_etat(self):
        ecrire_atomique(self.fichier("etat.json"), self.etat)

    def statut(self, statut, attente=None, fin=None):
        if statut not in STATUTS:
            raise ValueError("statut inconnu : %s" % statut)
        self.etat["statut"] = statut
        self.etat["attente"] = attente
        if fin is not None:
            self.etat["fin"] = fin
        self._ecrire_etat()

    def reponse(self, texte, statut, directives):
        ecrire_atomique(self.fichier("reponse.json"),
                        {"texte": texte, "statut": statut, "directives_citees": directives})


def lire_claims(jeton):
    """Contenu d'un jeton d'approbation, lu sans vérification : seulement
    pour consigner qui a validé, au titre de quel rôle. Le gate, lui,
    vérifie."""
    try:
        charge = jeton.split(".")[1]
        charge += "=" * (-len(charge) % 4)
        c = json.loads(base64.urlsafe_b64decode(charge.encode("ascii")))
        return {"identite": c.get("sub") or c.get("iss") or "", "role": c.get("role") or ""}
    except (IndexError, ValueError, UnicodeDecodeError, AttributeError):
        return {"identite": "", "role": ""}


class Approbations:
    """Les jetons déposés pour une empreinte. Un jeu de jetons essayé et
    refusé par le gate est mis de côté (suffixe .refuse) : la reprise
    suivante attend un nouveau dépôt complet."""

    def __init__(self, racine, horloge=time.monotonic, dormir=time.sleep, delai=DELAI_VALIDATION,
                 intervalle=INTERVALLE):
        self.racine = racine
        self.horloge = horloge
        self.dormir = dormir
        self.delai = delai
        self.intervalle = intervalle

    def dossier(self, empreinte):
        return os.path.join(self.racine, empreinte)

    def ecarter_anciens(self, empreinte):
        """À la première attente d'un appel dans une demande : les jetons
        déjà déposés pour la même empreinte viennent d'une demande
        précédente (même appel, mêmes arguments). Ils sont mis de côté
        (suffixe .ancien), avant que l'attente ne soit annoncée."""
        for f in self.deposes(empreinte):
            os.replace(f, f + ".ancien")

    def deposes(self, empreinte):
        return sorted(glob.glob(os.path.join(self.dossier(empreinte), "*.jwt")))

    def attendre(self, empreinte, requis, pendant_attente=None):
        """Attend au moins ``requis`` jetons (un par rôle), en relisant le
        dossier toutes les ``intervalle`` secondes, ``delai`` au plus. Rend
        la liste des chemins, ou None si le délai est écoulé."""
        os.makedirs(self.dossier(empreinte), exist_ok=True)
        fin = self.horloge() + self.delai
        premier = True
        while True:
            fichiers = self.deposes(empreinte)
            if len(fichiers) >= max(requis, 1):
                return fichiers[:MAX_JETONS]
            if premier and pendant_attente:
                pendant_attente()
            premier = False
            if self.horloge() >= fin:
                return None
            self.dormir(self.intervalle)

    @staticmethod
    def lire(fichiers):
        out = []
        for f in fichiers:
            with open(f, encoding="utf-8") as h:
                out.append(h.read().strip())
        return out

    @staticmethod
    def mettre_de_cote(fichiers):
        for f in fichiers:
            if os.path.exists(f):
                os.replace(f, f + ".refuse")
