"""Orchestrateur Mistral : une demande, une session MCP du gate, des
sous-agents déclaratifs.

    demande ──► orchestrateur (chat.complete, outil « deleguer »)
                    │  un sous-agent à la fois, tâche + résultats déjà obtenus
                    ▼
                sous-agent (boucle.Boucle : ses outils seulement, _meta.sous_agent
                posé par le code) ──► gate (même session MCP pour tous)
                    │
                    └── sortie JSON validée par le code (une relance au plus)

Règles tenues par le code, jamais par le modèle :

- ``_meta.sous_agent`` est le nom du sous-agent qui appelle, posé par la
  boucle ; le modèle ne le choisit pas (il choisit seulement à qui
  déléguer, parmi les sous-agents chargés).
- Un sous-agent ne voit que ses outils ; un outil absent du registre
  empêche le chargement.
- Le statut d'un appel vient du seul contrat de réponse de l'engine
  (``boucle.classer`` : ``structuredContent`` lu sur un résultat
  ``isError`` uniquement). Le ``structuredContent`` d'un serveur n'est
  jamais cru.
- Refus de l'engine : la demande s'arrête, et le texte rendu au demandeur
  est la décision de l'engine (``boucle.message_blocage``), pas ce que le
  modèle en dirait.
- Attente : l'état passe ``en_attente``, l'orchestrateur reste vivant sur
  la même session et attend les jetons (session.Approbations), puis
  reprend l'appel. Sans validation dans le délai : ``bloque``, motif
  « validation non reçue », texte tiré de la décision de l'engine.
- La réponse finale du modèle est suivie du relevé des décisions de
  l'engine, écrit par le code.
"""

import json

from boucle import boucle as bcl

from . import schema_sortie
from .session import lire_claims

TOURS_ORCHESTRATEUR = 8
DELEGATIONS_MAX = 6
TOURS_SOUS_AGENT = 5
OPTIONS = {"parallel_tool_calls": False}

CONSIGNE_ORCHESTRATEUR = """Tu es l'orchestrateur d'une équipe d'agents de la BANQUE SYNTHÉTIQUE (banque fictive de démonstration). Tu traites la demande d'un collaborateur en la découpant en tâches, que tu délègues une par une à tes sous-agents avec l'outil deleguer. Tu n'as pas d'autre outil : tu n'appelles jamais toi-même les outils des sous-agents.

Sous-agents disponibles :
{roles}

Règles :
- Délègue une seule tâche à la fois, au sous-agent dont c'est le rôle, avec une tâche précise qui reprend les identifiants, montants et éléments utiles. Chaque sous-agent reçoit aussi automatiquement les résultats déjà obtenus par les autres.
- Ne délègue que ce qui est nécessaire pour répondre à la demande ; ne délègue pas deux fois la même tâche.
- Un appel peut être refusé par une directive ou mis en attente d'une validation humaine : n'essaie jamais de le contourner.
- Quand la demande est traitée, réponds au demandeur en français, en quelques phrases, sans appel d'outil, en t'en tenant strictement aux résultats obtenus. N'affirme jamais qu'une action a été effectuée si un sous-agent ne l'a pas confirmée.

Demandeur : {demandeur}. Date de la demande : {date}."""

FIN_SOUS_AGENT = """Date de la demande (date de la décision) : {date}.

Quand ta tâche est terminée, réponds sans appel d'outil par un unique objet JSON, sans aucun texte autour, qui donne les valeurs des champs décrits par ce schéma JSON (ne recopie pas le schéma : remplis-le) :
{schema}"""


class ErreurOrchestrateur(Exception):
    pass


def outil_deleguer(agents):
    return {"type": "function", "function": {
        "name": "deleguer",
        "description": "Confie une tâche à un sous-agent et rend son résultat.",
        "parameters": {"type": "object", "properties": {
            "sous_agent": {"type": "string", "enum": sorted(agents), "description": "nom du sous-agent"},
            "tache": {"type": "string", "description": "tâche précise, avec les identifiants et montants utiles"},
        }, "required": ["sous_agent", "tache"]},
    }}


def _id_directive(nom):
    """« D-002 v1 » → « D-002 » ; « le registre demo v2 », « l'engine » → ''."""
    if not nom or nom.startswith("le registre") or nom.startswith("l'engine") or nom.startswith("une directive"):
        return ""
    return nom.split(" ")[0]


class Orchestrateur:
    """``serveur`` : client MCP du gate, déjà démarré (une session pour
    toute la demande). ``modele`` : ModeleReel ou ModeleRejoue.
    ``agents`` : sous-agents chargés (sous_agents.charger). ``dossier`` :
    session.DossierSession. ``approbations`` : session.Approbations.
    ``directives_attente`` : facultatif, rend les directives d'une attente
    d'après le journal du gate (empreinte → liste d'identifiants).
    ``pendant_attente`` : facultatif, appelé une fois à chaque entrée en
    attente avec l'attente (rejeu : dépôt des validations enregistrées)."""

    def __init__(self, serveur, modele, agents, dossier, approbations, date, demandeur,
                 directives_attente=None, pendant_attente=None, horodater=None):
        self.serveur = serveur
        self.modele = modele
        self.agents = agents
        self.dossier = dossier
        self.approbations = approbations
        self.date = date
        self.demandeur = demandeur
        self.directives_attente = directives_attente
        self.pendant_attente = pendant_attente
        self.horodater = horodater or (lambda: None)
        self.outils_mcp = serveur.lister_outils()
        self.trace = {"delegations": [], "validations": []}
        self.resultats = []  # (sous-agent, sortie validée)
        self.decisions = []  # relevé des appels, pour la réponse finale
        self.cites = []  # directives d'attente lues au journal du gate
        self.par_empreinte = {}  # empreinte transmise → directives qui la retiennent
        self.attentes_vues = set()

    # -- consignes ----------------------------------------------------------

    def consigne(self):
        roles = "\n".join("- %s : %s" % (a.nom, a.role) for a in self.agents.values())
        return CONSIGNE_ORCHESTRATEUR.format(roles=roles, demandeur=self.demandeur, date=self.date)

    def consigne_sous_agent(self, agent):
        return agent.consigne + "\n\n" + FIN_SOUS_AGENT.format(
            date=self.date, schema=json.dumps(agent.schema, ensure_ascii=False))

    def tache_complete(self, tache):
        if not self.resultats:
            return tache
        deja = "\n".join("- %s : %s" % (nom, json.dumps(s, ensure_ascii=False)) for nom, s in self.resultats)
        return tache + "\n\nRésultats déjà obtenus par les autres sous-agents :\n" + deja

    # -- point d'entrée -----------------------------------------------------

    def executer(self, demande):
        messages = [{"role": "system", "content": self.consigne()}, {"role": "user", "content": demande}]
        outils = [outil_deleguer(self.agents)]
        vue = self.modele.pour("orchestrateur")
        delegations = 0
        for _ in range(TOURS_ORCHESTRATEUR):
            r = vue.completer(messages, outils, choix_outil="auto", options=OPTIONS)
            try:
                message = r["choices"][0]["message"]
            except (KeyError, IndexError, TypeError):
                return self._terminer("erreur", "Réponse du modèle illisible : aucune réponse n'est donnée.")
            appels = message.get("tool_calls") or []
            if not appels:
                return self._terminer("termine", (message.get("content") or "").strip())
            premier = appels[0]
            if not premier.get("id"):
                return self._terminer("erreur", "Appel d'outil sans identifiant : aucune action effectuée.")
            messages.append({"role": "assistant", "content": message.get("content") or "", "tool_calls": appels})
            fn = premier.get("function") or {}
            contenu, fin = self._deleguer(fn, delegations)
            delegations += 1
            if fin is not None:
                return fin
            messages.append({"role": "tool", "tool_call_id": premier["id"], "name": fn.get("name", ""),
                             "content": contenu})
            for autre in appels[1:]:
                messages.append({"role": "tool", "tool_call_id": autre.get("id", ""),
                                 "name": (autre.get("function") or {}).get("name", ""),
                                 "content": "Non exécuté : une seule délégation par tour."})
        return self._terminer("erreur", "Nombre maximal de tours de l'orchestrateur atteint (%d) : la demande "
                                        "n'a pas abouti." % TOURS_ORCHESTRATEUR)

    # -- délégation ---------------------------------------------------------

    def _deleguer(self, fn, rang):
        """Rend (contenu pour le modèle, None) pour continuer, ou (None, fin)."""
        if fn.get("name") != "deleguer":
            return "Outil inconnu : %s. Seul deleguer existe." % fn.get("name"), None
        try:
            args = fn.get("arguments")
            args = json.loads(args) if isinstance(args, str) else (args or {})
        except ValueError:
            return "Arguments illisibles (JSON invalide).", None
        nom, tache = args.get("sous_agent"), args.get("tache")
        if nom not in self.agents:
            return "Sous-agent inconnu : %r. Sous-agents : %s." % (nom, ", ".join(sorted(self.agents))), None
        if not isinstance(tache, str) or not tache.strip():
            return "Tâche vide : précise la tâche du sous-agent.", None
        if rang >= DELEGATIONS_MAX:
            return "Nombre maximal de délégations atteint : réponds au demandeur avec les résultats obtenus.", None
        agent = self.agents[nom]
        d = {"sous_agent": nom, "tache": tache, "appels": [], "sortie": None, "etat": None}
        self.trace["delegations"].append(d)
        b = bcl.Boucle(self.serveur, self.modele.pour(nom), nom, tours_max=TOURS_SOUS_AGENT,
                       consigne=self.consigne_sous_agent(agent), outils=agent.outils, outils_mcp=self.outils_mcp,
                       arret_sur_refus=True)
        r = b.executer(self.tache_complete(tache))
        relance = False
        while True:
            if r["etat"] == bcl.EN_ATTENTE:
                r = self._attendre_et_reprendre(b, r, d)
                if r.get("_fin_demande"):
                    self._noter(d, r)
                    return None, r["_fin_demande"]
                continue
            if r["etat"] == bcl.BLOQUE:
                self._noter(d, r)
                d["etat"] = "bloque"
                refus = r.get("refus") or {}
                ids = [i for i in [_id_directive(refus.get("directive", ""))] if i]
                return None, self._terminer("bloque", r["reponse"], directives=ids)
            if r["etat"] != bcl.TERMINE:
                self._noter(d, r)
                d["etat"] = "erreur"
                return "Le sous-agent %s n'a pas abouti : %s" % (nom, r["reponse"]), None
            sortie, erreur = schema_sortie.lire_sortie(r["reponse"])
            ecarts = [erreur] if erreur else schema_sortie.valider(sortie, agent.schema)
            if not ecarts:
                self._noter(d, r)
                d["etat"] = "termine"
                d["sortie"] = sortie
                self.resultats.append((nom, sortie))
                return json.dumps(sortie, ensure_ascii=False), None
            if relance:
                self._noter(d, r)
                d["etat"] = "sortie_invalide"
                d["ecarts"] = ecarts
                return ("Sortie du sous-agent %s rejetée (non conforme à son schéma, après une relance) : %s"
                        % (nom, "; ".join(ecarts))), None
            relance = True
            d["relance"] = ecarts
            r = b.relancer(r, "Ta réponse finale n'est pas conforme au schéma demandé (%s). Réponds uniquement "
                              "par l'objet JSON conforme, sans appel d'outil." % "; ".join(ecarts))

    def _noter(self, d, r):
        vus = len(d["appels"])
        for j in r.get("journal", [])[vus:]:
            e = {"outil": j["outil"], "nature": j["nature"], "avec_jeton": j.get("avec_jeton", False)}
            for k in ("directive", "motif", "empreinte"):
                if j.get(k):
                    e[k] = j[k]
            if (j.get("attente") or {}).get("motif_jeton"):
                e["motif_jeton"] = j["attente"]["motif_jeton"]
            d["appels"].append(e)
            self.decisions.append(dict(e, sous_agent=d["sous_agent"]))

    # -- attente ------------------------------------------------------------

    def _attente(self, r):
        dernier = r["journal"][-1] if r.get("journal") else {}
        a = dernier.get("attente") or {}
        empreinte = r.get("empreinte")
        directives = None
        if self.directives_attente and empreinte:
            directives = self.directives_attente(empreinte)
        if not directives:
            directives = [i for i in [_id_directive(r.get("directive", ""))] if i]
        for i in directives:
            if i not in self.cites:
                self.cites.append(i)
        if empreinte:
            self.par_empreinte[empreinte] = directives
        attente = {"empreinte_transmise": empreinte, "roles": a.get("roles") or [],
                   "quatre_yeux": bool(a.get("quatre_yeux")), "directives": directives}
        if a.get("motif_jeton"):
            attente["motif_jeton"] = a["motif_jeton"]
        return attente

    def _attendre_et_reprendre(self, b, r, d):
        """Attend les jetons, reprend ; rend le résultat de la boucle, ou
        un résultat portant ``_fin_demande`` (validation non reçue)."""
        attente = self._attente(r)
        if not attente["empreinte_transmise"]:
            return dict(r, _fin_demande=self._terminer(
                "erreur", "L'engine a mis l'appel en attente sans empreinte à valider : aucune action effectuée.",
                directives=attente["directives"]))
        if attente["empreinte_transmise"] not in self.attentes_vues:
            self.attentes_vues.add(attente["empreinte_transmise"])
            self.approbations.ecarter_anciens(attente["empreinte_transmise"])
        self.dossier.statut("en_attente", attente)
        fichiers = self.approbations.attendre(
            attente["empreinte_transmise"], len(attente["roles"]),
            pendant_attente=(lambda: self.pendant_attente(attente)) if self.pendant_attente else None)
        if fichiers is None:
            texte = ("Validation non reçue dans le délai : l'appel %s reste en attente de validation humaine "
                     "(%s), aucune action effectuée.\n%s" % (r["outil"], ", ".join(attente["directives"]) or "engine",
                                                             r["reponse"]))
            self._noter(d, r)
            d["etat"] = "bloque"
            return dict(r, _fin_demande=self._terminer("bloque", texte, directives=attente["directives"],
                                                       motif="validation non reçue"))
        jetons = self.approbations.lire(fichiers)
        valideurs = [lire_claims(j) for j in jetons]
        self.dossier.statut("en_cours")
        suite = b.reprendre(r, jetons if len(jetons) > 1 else jetons[0])
        accepte = not (suite["etat"] == bcl.EN_ATTENTE and suite.get("empreinte") == attente["empreinte_transmise"])
        if not accepte:
            # Reprise refusée par le gate (motif_jeton) : jetons mis de côté,
            # nouvelle attente d'un dépôt complet.
            self.approbations.mettre_de_cote(fichiers)
        self.trace["validations"].append({"empreinte_transmise": attente["empreinte_transmise"],
                                          "valideurs": valideurs, "accepte": accepte})
        return suite

    # -- fin ----------------------------------------------------------------

    def releve(self):
        lignes = []
        for e in self.decisions:
            retenu = ", ".join(self.par_empreinte.get(e.get("empreinte")) or []) or e.get("directive", "")
            nature = {"ok": "transmis, données reçues", "refus": "refusé par %s" % e.get("directive", ""),
                      "attente": "mis en attente de validation (%s)" % retenu,
                      "indisponible": "source indisponible", "erreur": "erreur ou refus de l'outil"
                      }.get(e["nature"], e["nature"])
            if e.get("avec_jeton") and e["nature"] == "ok":
                nature = "validé, puis transmis"
            if e.get("avec_jeton") and e["nature"] == "attente":
                nature = "validation refusée par l'engine (%s), toujours en attente (%s)" % (
                    e.get("motif_jeton", "motif non précisé"), retenu)
            lignes.append("- %s · %s : %s" % (e["sous_agent"], e["outil"], nature))
        return lignes

    def directives_citees(self):
        ids = []
        for e in self.decisions:
            if e["nature"] in ("refus", "attente"):
                i = _id_directive(e.get("directive", ""))
                if i and i not in ids:
                    ids.append(i)
        for v in self.cites:
            if v not in ids:
                ids.append(v)
        return ids

    def _terminer(self, statut, texte, directives=None, motif=None):
        for i in directives or []:
            if i not in self.cites:
                self.cites.append(i)
        releve = self.releve()
        if releve:
            texte = (texte or "(aucune réponse du modèle)") + "\n\nDécisions de l'engine :\n" + "\n".join(releve)
        citees = self.directives_citees()
        self.dossier.reponse(texte, statut, citees)
        attente = None
        if motif:
            attente = dict(self.dossier.etat.get("attente") or {}, motif=motif)
        self.dossier.statut(statut, attente, fin=self.horodater())
        self.trace["statut"] = statut
        return {"statut": statut, "texte": texte, "directives_citees": citees, "trace": self.trace}
