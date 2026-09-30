"""Catalogue des serveurs et outils disponibles, fourni par l'appelant.

C'est la seule source de noms que le compilateur accepte : une directive qui
cite un serveur, un outil, un champ ou un appelant absent du catalogue est
refusée avant même le chargeur strict.

Format ::

    {"serveurs": [{"nom": "banque-synthetique", "description": "...",
                   "outils": [{"nom": "attribuer_pret", "description": "...",
                               "arguments": ["/montant", "/siren"]}]}],
     "appelants": ["spiffe://..."]}          # facultatif

Le catalogue de démonstration, ``exemples/catalogue-demo.json``, est généré
depuis le registre de démo (``go run ./cmd/registre-demo -catalogue ...``) ;
ses outils portent en plus ``acces`` (read ou action), et le document une
clé ``origine``. Le compilateur ignore les deux.
"""

import json


class CatalogueInvalide(Exception):
    pass


class Catalogue:
    def __init__(self, donnees):
        self.donnees = donnees
        self.serveurs = {}
        for s in donnees.get("serveurs", []):
            nom = s.get("nom")
            if not isinstance(nom, str) or not nom:
                raise CatalogueInvalide("serveur sans nom")
            if nom in self.serveurs:
                raise CatalogueInvalide("serveur %r en double" % nom)
            self.serveurs[nom] = {o["nom"]: o for o in s.get("outils", [])}
        if not self.serveurs:
            raise CatalogueInvalide("aucun serveur")
        self.appelants = list(donnees.get("appelants", []))

    @classmethod
    def lire(cls, chemin):
        with open(chemin, encoding="utf-8") as f:
            return cls(json.load(f))

    def noms_serveurs(self):
        return sorted(self.serveurs)

    def noms_outils(self):
        return sorted({o for outils in self.serveurs.values() for o in outils})

    def outils_qualifies(self):
        return sorted(s + "/" + o for s, outils in self.serveurs.items() for o in outils)

    def arguments(self):
        """Tous les pointeurs d'arguments déclarés ; vide si aucun ne l'est."""
        return sorted({a for outils in self.serveurs.values()
                       for o in outils.values() for a in o.get("arguments", [])})

    def arguments_de(self, serveur, outil):
        """Pointeurs admis pour une cible, étoiles comprises."""
        out = set()
        for s, outils in self.serveurs.items():
            if serveur not in ("*", s):
                continue
            for o, desc in outils.items():
                if outil in ("*", o):
                    out.update(desc.get("arguments", []))
        return out

    def cible_existe(self, serveur, outil):
        if serveur == "*":
            return outil == "*" or outil in self.noms_outils()
        if serveur not in self.serveurs:
            return False
        return outil == "*" or outil in self.serveurs[serveur]

    def pour_le_modele(self):
        """Le catalogue tel qu'on le montre à Mistral, sans rien d'autre."""
        return json.dumps(self.donnees, ensure_ascii=False, indent=1, sort_keys=True)
