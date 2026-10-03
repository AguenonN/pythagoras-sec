"""Les réponses enregistrées de la rédaction sont indexées par la requête,
qui contient la liste des rôles de l'annuaire. L'annuaire de démo s'est
enrichi (secteurs) : ces tests figent l'annuaire d'avant, copié dans
``donnees/``. À la réenregistrement des réponses (session de gel), retirer
``figer`` des modules de test."""

import os
from unittest import mock

from analyste import redaction

ANNUAIRE_FIGE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "donnees", "annuaire-source.json")


def figer(cls_test=None):
    original = redaction.roles
    p = mock.patch.object(redaction, "roles", lambda chemin=ANNUAIRE_FIGE: original(chemin))
    p.start()
    return p


def derouler_fige():
    """Parcours D-009 sur le jeu, le catalogue et le vocabulaire d'avant les
    secteurs, ceux de la compilation enregistrée (écart pour le gel)."""
    from analyste import critere
    d = os.path.join(os.path.dirname(ANNUAIRE_FIGE))
    return critere.derouler(jeu_existant=os.path.join(d, "demo-avant-secteurs.json"),
                            catalogue=os.path.join(d, "catalogue-demo-avant-secteurs.json"),
                            vocabulaire=os.path.join(d, "vocabulaire-demo-avant-secteurs.json"))
