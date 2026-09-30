#!/usr/bin/env python3
"""Faux serveur MCP sur stdio pour les tests de la boucle.

Outils exposés sous leur nom agrégé serveur__outil. Les refus, attentes et
échecs ont la forme du contrat de réponse de l'engine (docs/contrat-engine.md
§6) : isError et structuredContent.statut.

- taux-officiel__lire_taux            lecture ; type « usure » répond, type
                                      « directeur » : source indisponible
- banque-synthetique__attribuer_pret  en attente de validation, sauf avec le
                                      jeton JETON_VALIDE dans _meta.approvalToken
- gels-avoirs__lever_gel              refusé par directive (statut refuse) ;
                                      avec l'entité ENTITE_JSON_RPC, par une
                                      erreur JSON-RPC -32001 (convention de
                                      cmd/mcp-proxy)
- referentiel-rh__modifier_poste      refusé par directive (statut refuse)
- rh-synthetique__lire_fiche          décline (isError sans structure)
- eurlex__chercher_texte              schéma avec mots-clés retirés (format, pattern, const…)
- legifrance__citer_article           schéma avec oneOf : doit être exclu

Chaque tools/call reçu (params complets, _meta compris) est ajouté en JSON
par ligne au fichier FAUX_MCP_JOURNAL s'il est défini. Au premier
tools/list, le serveur envoie une notification et une requête de sampling
que l'hôte doit refuser ; la réponse de l'hôte est journalisée aussi.
"""

import hashlib
import json
import os
import sys

JETON_VALIDE = "jeton-de-test-valide"
ENTITE_JSON_RPC = "via-json-rpc"

OUTILS = [
    {
        "name": "taux-officiel__lire_taux",
        "description": "Rend le taux de référence officiel en vigueur (taux d'usure ou taux directeur).",
        "inputSchema": {"type": "object", "properties": {
            "type": {"type": "string", "enum": ["usure", "directeur"], "description": "Taux demandé."}},
            "required": ["type"]},
    },
    {
        "name": "banque-synthetique__attribuer_pret",
        "description": "Attribue un prêt à une entreprise identifiée par son SIREN.",
        "inputSchema": {"type": "object", "properties": {
            "siren": {"type": "string", "description": "SIREN à 9 chiffres."},
            "montant": {"type": "number", "description": "Montant en euros."}},
            "required": ["siren", "montant"]},
    },
    {
        "name": "gels-avoirs__lever_gel",
        "description": "Lève le gel des avoirs d'une entité.",
        "inputSchema": {"type": "object", "properties": {
            "entite": {"type": "string", "description": "Nom de l'entité."}}, "required": ["entite"]},
    },
    {
        "name": "referentiel-rh__modifier_poste",
        "description": "Modifie l'intitulé du poste d'un salarié dans le référentiel RH.",
        "inputSchema": {"type": "object", "properties": {
            "matricule": {"type": "string"}, "poste": {"type": "string"}}, "required": ["matricule", "poste"]},
    },
    {
        "name": "rh-synthetique__lire_fiche",
        "description": "Lit la fiche d'un salarié.",
        "inputSchema": {"type": "object", "properties": {"matricule": {"type": "string"}}, "required": ["matricule"]},
    },
    {
        "name": "eurlex__chercher_texte",
        "title": "Recherche EUR-Lex",
        "description": "Cherche un texte européen par sa référence CELEX.",
        "inputSchema": {
            "$schema": "https://json-schema.org/draft/2020-12/schema",
            "type": "object",
            "additionalProperties": False,
            "properties": {
                "celex": {"type": "string", "pattern": "^[0-9]{5}[A-Z][0-9]{4}$", "description": "Numéro CELEX."},
                "langue": {"const": "fr"},
                "depuis": {"type": ["string", "null"], "format": "date"},
                "limite": {"type": "integer", "minimum": 1, "maximum": 20, "default": 5},
            },
            "required": ["celex"],
        },
    },
    {
        "name": "legifrance__citer_article",
        "description": "Cite un article de code.",
        "inputSchema": {"type": "object", "properties": {
            "reference": {"oneOf": [{"type": "string"}, {"type": "object", "properties": {"code": {"type": "string"}}}]}}},
    },
]


def empreinte(nom, arguments):
    canon = json.dumps({"outil": nom, "arguments": arguments}, sort_keys=True, ensure_ascii=False, separators=(",", ":"))
    return "sha256:" + hashlib.sha256(canon.encode("utf-8")).hexdigest()


def journaliser(entree):
    chemin = os.environ.get("FAUX_MCP_JOURNAL")
    if chemin:
        with open(chemin, "a", encoding="utf-8") as f:
            f.write(json.dumps(entree, ensure_ascii=False, sort_keys=True) + "\n")


def texte(t):
    return [{"type": "text", "text": t}]


def appeler(params):
    nom = params.get("name")
    args = params.get("arguments") or {}
    meta = params.get("_meta") or {}
    if nom == "taux-officiel__lire_taux":
        if args.get("type") == "directeur":
            return echec("taux-officiel", "lire_taux", "Source indisponible : le serveur taux-officiel n'a pas répondu.")
        taux = {"usure": 5.87}.get(args.get("type"))
        if taux is None:
            return {"content": texte("type de taux inconnu"), "isError": True}
        sc = {"type": args["type"], "taux": taux, "date": "2026-10-01"}
        return {"content": texte(json.dumps(sc, ensure_ascii=False)), "structuredContent": sc}
    if nom == "banque-synthetique__attribuer_pret":
        e = empreinte(nom, args)
        if meta.get("approvalToken") == JETON_VALIDE:
            sc = {"statut": "accorde", "reference": "PRET-0001", "siren": args.get("siren"), "montant": args.get("montant")}
            return {"content": texte("Prêt accordé, référence PRET-0001."), "structuredContent": sc}
        sc = {"statut": "en_attente", "empreinte_transmise": e, "directive": "D-001", "version": 1}
        return {"content": texte("Appel non transmis : en attente de validation humaine par D-001 v1 "
                                 "(provenance_session, RGPD art. 22)."), "structuredContent": sc, "isError": True}
    if nom == "gels-avoirs__lever_gel":
        motif = "refusé par D-003 v1 (interdiction, Règlement (CE) 2580/2001)"
        if args.get("entite") == ENTITE_JSON_RPC:
            raise Refus(motif, {"directive": "D-003 v1"})
        return refus("D-003", 1, {"texte": "Règlement (CE) 2580/2001"}, motif)
    if nom == "referentiel-rh__modifier_poste":
        return refus("D-010", 1, {"texte": "Code du travail", "article": "L1222-1"},
                     "refusé par D-010 v1 (approbation_humaine, Code du travail art. L1222-1) : modification unilatérale interdite")
    if nom == "rh-synthetique__lire_fiche":
        return {"content": texte("Salarié inconnu : %s" % args.get("matricule")), "isError": True}
    if nom == "eurlex__chercher_texte":
        return {"content": texte("32016R0679 : Règlement général sur la protection des données.")}
    raise Refus("outil inconnu : %s" % nom, None, code=-32602)


def refus(directive, version, fondement, motif):
    sc = {"statut": "refuse", "origine": "directive", "directive": directive, "version": version,
          "fondement": fondement, "motif": motif}
    return {"content": texte("Appel refusé par l'engine : %s." % motif), "structuredContent": sc, "isError": True}


def echec(serveur, outil, motif):
    sc = {"statut": "indisponible", "serveur": serveur, "outil": outil, "motif": motif}
    return {"content": texte(motif), "structuredContent": sc, "isError": True}


class Refus(Exception):
    def __init__(self, message, data, code=-32001):
        super().__init__(message)
        self.message, self.data, self.code = message, data, code


def main():
    sortie = sys.stdout
    premiere_liste = True

    def envoyer(msg):
        sortie.write(json.dumps(msg, ensure_ascii=False) + "\n")
        sortie.flush()

    for ligne in sys.stdin:
        ligne = ligne.strip()
        if not ligne:
            continue
        msg = json.loads(ligne)
        methode, ident = msg.get("method"), msg.get("id")
        if methode is None:  # réponse de l'hôte à notre requête de sampling
            journaliser({"reponse_hote": msg})
            continue
        if ident is None:
            continue  # notification
        if methode == "initialize":
            envoyer({"jsonrpc": "2.0", "id": ident, "result": {
                "protocolVersion": msg["params"].get("protocolVersion"),
                "capabilities": {"tools": {}}, "serverInfo": {"name": "faux-serveur", "version": "0"}}})
        elif methode == "tools/list":
            if premiere_liste:
                premiere_liste = False
                envoyer({"jsonrpc": "2.0", "method": "notifications/message", "params": {"level": "info", "data": "bonjour"}})
                envoyer({"jsonrpc": "2.0", "id": "s-1", "method": "sampling/createMessage",
                         "params": {"messages": [], "maxTokens": 10}})
            # Pagination : deux pages, pour exercer nextCursor.
            curseur = (msg.get("params") or {}).get("cursor")
            if curseur is None:
                envoyer({"jsonrpc": "2.0", "id": ident, "result": {"tools": OUTILS[:3], "nextCursor": "p2"}})
            else:
                envoyer({"jsonrpc": "2.0", "id": ident, "result": {"tools": OUTILS[3:]}})
        elif methode == "tools/call":
            journaliser({"tools/call": msg["params"]})
            try:
                envoyer({"jsonrpc": "2.0", "id": ident, "result": appeler(msg["params"])})
            except Refus as r:
                erreur = {"code": r.code, "message": r.message}
                if r.data:
                    erreur["data"] = r.data
                envoyer({"jsonrpc": "2.0", "id": ident, "error": erreur})
        else:
            envoyer({"jsonrpc": "2.0", "id": ident, "error": {"code": -32601, "message": "méthode inconnue"}})


if __name__ == "__main__":
    main()
