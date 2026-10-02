"""Le modèle vu par l'orchestrateur et ses sous-agents : appels réels
enregistrés dans l'ordre, ou rejeu de cet ordre sans réseau.

Le rejeu ne peut pas retrouver une réponse par l'empreinte de la requête
(comme le fait ``compilateur.mistral``) : le gate scelle chaque résultat
d'outil sous un identifiant tiré au hasard, et les dates d'obtention des
données changent d'un passage à l'autre. Il sert donc les réponses dans
l'ordre où elles ont été obtenues, en vérifiant que chacune est demandée
par le même agent ; le premier écart arrête le rejeu.

Un enregistrement ne contient que les réponses de Mistral (ni la clé, ni
les en-têtes, ni les requêtes) : ce sont les requêtes qui portent les
résultats d'outils, gardés dans le dossier de session, hors Git.
"""

import os

from compilateur import mistral

ICI = os.path.dirname(os.path.abspath(__file__))
ETAT = os.path.join(os.path.dirname(ICI), ".etat", "orchestrateur")
# Budget de la session « démo réelle » (nuit/brief_A_demo_reelle.md) :
# 80 appels réels, puis 30 de plus accordés par Manu le 02/10 (110 au total),
# compteur dans agents/.etat/orchestrateur.
MAX_APPELS = 110


class ErreurRejeu(Exception):
    pass


class _Vue:
    def __init__(self, parent, agent):
        self.parent = parent
        self.agent = agent

    def completer(self, messages, outils, choix_outil="auto", options=None):
        return self.parent.completer(self.agent, messages, outils, choix_outil, options)


class ModeleReel:
    """Appels réels (cache sur disque et budget de compilateur.mistral),
    chaque réponse étant aussi gardée dans l'ordre pour l'enregistrement."""

    def __init__(self, cache, etat=ETAT, max_appels=None, client=None):
        if max_appels is None:
            max_appels = int(os.environ.get("ORCHESTRATEUR_MAX_APPELS", str(MAX_APPELS)))
        self.client = client or mistral.Client(mode="reel", enregistrements=cache, etat=etat, max_appels=max_appels)
        self.reponses = []

    def pour(self, agent):
        return _Vue(self, agent)

    def completer(self, agent, messages, outils, choix_outil, options):
        r = self.client.completer(messages, outils, choix_outil=choix_outil, options=options)
        self.reponses.append({"agent": agent, "reponse": r})
        return r


class ModeleRejoue:
    def __init__(self, reponses):
        self.attendues = list(reponses)
        self.reponses = []

    def pour(self, agent):
        return _Vue(self, agent)

    def completer(self, agent, messages, outils, choix_outil, options):
        rang = len(self.reponses)
        if rang >= len(self.attendues):
            raise ErreurRejeu("rejeu : réponse %d demandée par %s, l'enregistrement s'arrête à %d"
                              % (rang + 1, agent, len(self.attendues)))
        e = self.attendues[rang]
        if e.get("agent") != agent:
            raise ErreurRejeu("rejeu : réponse %d enregistrée pour %s, demandée par %s"
                              % (rang + 1, e.get("agent"), agent))
        self.reponses.append(e)
        return e["reponse"]

    def restantes(self):
        return len(self.attendues) - len(self.reponses)
