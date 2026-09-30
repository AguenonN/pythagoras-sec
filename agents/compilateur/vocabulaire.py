"""Vocabulaire fermé du compilateur v2 : tout nom qu'une directive peut citer.

Trois sources, aucune tenue en double :

- le catalogue généré depuis le registre de démo (serveurs, outils, accès,
  arguments), ``exemples/catalogue-demo.json`` ;
- un complément pour ce que le registre et l'annuaire ne portent pas encore
  (étiquettes, rôles, groupes), ``exemples/vocabulaire-demo.json`` ;
- les fuseaux admis par le chargeur Go (``directive fuseaux``).

Chaque liste devient une énumération du function calling : Mistral ne peut
pas écrire un nom qui n'y est pas. Qu'un nom soit dans la liste ne suffit
pas : il faut encore que le juriste l'ait écrit ou choisi (verifications_v2).

Format du complément ::

    {"etiquettes": [{"nom": "donnees_client", "description": "..."}],
     "roles":      [{"nom": "juridique", "description": "..."}],
     "groupes":    [{"nom": "salaries-proteges", "description": "..."}]}
"""

import json

from . import directive_go
from .catalogue import Catalogue, CatalogueInvalide

JOURS = ["lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi", "dimanche"]


class Vocabulaire:
    def __init__(self, catalogue, complement, fuseaux):
        if not isinstance(catalogue, Catalogue):
            catalogue = Catalogue(catalogue)
        self.catalogue = catalogue
        self.complement = complement
        self.etiquettes = self._noms(complement, "etiquettes")
        self.roles = self._noms(complement, "roles")
        self.groupes = self._noms(complement, "groupes")
        self.fuseaux = sorted(fuseaux)
        if not self.fuseaux:
            raise CatalogueInvalide("aucun fuseau")

    @staticmethod
    def _noms(complement, cle):
        noms = []
        for e in complement.get(cle, []):
            nom = e.get("nom") if isinstance(e, dict) else None
            if not isinstance(nom, str) or not nom:
                raise CatalogueInvalide("%s : entrée sans nom" % cle)
            if nom in noms:
                raise CatalogueInvalide("%s : %r en double" % (cle, nom))
            noms.append(nom)
        return sorted(noms)

    @classmethod
    def lire(cls, chemin_catalogue, chemin_complement, fuseaux=None):
        with open(chemin_complement, encoding="utf-8") as f:
            complement = json.load(f)
        if fuseaux is None:
            fuseaux = directive_go.fuseaux()
        return cls(Catalogue.lire(chemin_catalogue), complement, fuseaux)

    # -- accès au catalogue ---------------------------------------------------

    def acces(self, serveur, outil):
        return self.catalogue.serveurs.get(serveur, {}).get(outil, {}).get("acces")

    def outils_vises(self, cible):
        """Les couples (serveur, outil) du catalogue que vise une cible de
        déclencheur ; l'étiquette n'est pas connue du catalogue et ne
        restreint rien ici."""
        serveur = cible.get("serveur") or "*"
        outil = cible.get("outil") or "*"
        acces = cible.get("acces")
        out = []
        for s in sorted(self.catalogue.serveurs):
            if serveur not in ("*", s):
                continue
            for o in sorted(self.catalogue.serveurs[s]):
                if outil not in ("*", o):
                    continue
                if acces and self.acces(s, o) not in (None, acces):
                    continue
                out.append((s, o))
        return out

    def arguments_de(self, serveur, outil):
        return set(self.catalogue.serveurs.get(serveur, {}).get(outil, {}).get("arguments", []))

    def pour_le_modele(self):
        """Ce qu'on montre à Mistral : le catalogue, puis le complément."""
        complement = {k: self.complement.get(k, []) for k in ("etiquettes", "roles", "groupes")}
        return (self.catalogue.pour_le_modele() + "\n\nÉtiquettes, rôles et groupes connus :\n" +
                json.dumps(complement, ensure_ascii=False, indent=1, sort_keys=True) +
                "\n\nFuseaux admis : " + ", ".join(self.fuseaux))
