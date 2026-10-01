"""Ligne de commande de l'orchestrateur (lancée par scripts/lancer-demo.sh).

    python -m orchestrateur executer --demande "…" --demandeur ID --session DIR \\
        --approbations DIR --date AAAA-MM-JJ [--mode reel|rejeu] [--enregistrement F] \\
        [--scenario NOM] [--meta JSON] [--mcp-approve BIN --cles DIR] -- <commande du gate>
    python -m orchestrateur sous-agents [--registre F | --catalogue F] [--dossier D]
    python -m orchestrateur comparer <journal1.jsonl> <journal2.jsonl>

executer : lance le gate (commande après --, en stdio, sans MISTRAL_API_KEY
dans son environnement), charge les sous-agents devant les outils que le
gate expose, traite la demande, écrit etat.json, reponse.json et
orchestrateur.json (trace) dans le dossier de session.

- ``--mode reel`` : Mistral, budget ORCHESTRATEUR_MAX_APPELS (80) ; les
  réponses obtenues sont écrites, dans l'ordre, dans ``--enregistrement``
  (avec les validations constatées), à rejouer hors ligne ;
- ``--mode rejeu`` : les réponses de ``--enregistrement``, dans l'ordre,
  sans réseau ; les validations enregistrées sont redéposées en signant de
  nouveaux jetons avec ``--mcp-approve`` (mêmes valideurs, mêmes rôles).

Codes de sortie : 0 terminé, 2 bloqué, 1 erreur.
"""

import argparse
import datetime
import json
import os
import subprocess
import sys

from boucle import mcp
from compilateur import mistral

from . import journal_gate, modele, orchestrateur, session, sous_agents

ICI = os.path.dirname(os.path.abspath(__file__))
CATALOGUE = os.path.join(os.path.dirname(ICI), "compilateur", "exemples", "catalogue-demo.json")
REGISTRE = os.path.join(os.path.dirname(os.path.dirname(ICI)), "demo", "registre.json")
SECRETS_HORS_GATE = ("MISTRAL_API_KEY",)


def maintenant():
    return datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def env_gate():
    return {k: v for k, v in os.environ.items() if k not in SECRETS_HORS_GATE}


def valideur_rejeu(validations, approbations, mcp_approve, cles):
    """Rejeu : à chaque attente, dépose les jetons des valideurs enregistrés
    pour cette attente, dans l'ordre, signés à nouveau."""
    restantes = list(validations)

    def deposer(attente):
        if not restantes:
            return
        v = restantes.pop(0)
        dossier = approbations.dossier(attente["empreinte_transmise"])
        os.makedirs(dossier, exist_ok=True)
        for i, x in enumerate(v.get("valideurs") or []):
            r = subprocess.run([mcp_approve, "valider", "-cles", cles, "-valideur", x["identite"], "-role", x["role"],
                                "-empreinte", attente["empreinte_transmise"]], capture_output=True, text=True)
            if r.returncode != 0:
                raise orchestrateur.ErreurOrchestrateur("mcp-approve valider : " + r.stderr.strip())
            tmp = os.path.join(dossier, ".%d.tmp" % i)
            with open(tmp, "w", encoding="utf-8") as f:
                f.write(r.stdout.strip() + "\n")
            os.replace(tmp, os.path.join(dossier, "%02d-%s.jwt" % (i, x["identite"])))
    return deposer


def executer(a, sortie):
    if a.mode == "rejeu":
        with open(a.enregistrement, encoding="utf-8") as f:
            enr = json.load(f)
        m = modele.ModeleRejoue(enr["reponses"])
        date, demande, demandeur = enr["date"], enr["demande"], enr["demandeur"]
    else:
        m = modele.ModeleReel(cache=os.path.join(a.session, "mistral"))
        date, demande, demandeur, enr = a.date, a.demande, a.demandeur, None
    if not demande or not demandeur or not date:
        raise SystemExit("--demande, --demandeur et --date obligatoires (ou --mode rejeu avec un enregistrement)")
    dossier = session.DossierSession(a.session, demande, demandeur, maintenant())
    approbations = session.Approbations(a.approbations)
    pendant = None
    if enr is not None and enr.get("validations"):
        if not a.mcp_approve or not a.cles:
            raise SystemExit("rejeu avec validations : --mcp-approve et --cles obligatoires")
        pendant = valideur_rejeu(enr["validations"], approbations, a.mcp_approve, a.cles)
    trace_chemin = dossier.fichier("orchestrateur.json")
    resultat = None
    try:
        with mcp.ClientMCP(a.gate, env=env_gate(), nom="orchestrateur") as gate:
            exposes = {o["name"] for o in gate.lister_outils()}
            agents = sous_agents.charger(exposes, a.dossier_sous_agents)
            o = orchestrateur.Orchestrateur(
                gate, m, agents, dossier, approbations, date, demandeur,
                directives_attente=lambda e: journal_gate.directives_attente(dossier.fichier("journal.jsonl"), e),
                pendant_attente=pendant, horodater=maintenant)
            resultat = o.executer(demande)
    except (mistral.ErreurMistral, modele.ErreurRejeu, mcp.ErreurMCP, sous_agents.SousAgentInvalide,
            orchestrateur.ErreurOrchestrateur, ValueError) as e:
        texte = "Erreur de l'orchestrateur, la demande n'a pas abouti : %s" % e
        dossier.reponse(texte, "erreur", [])
        dossier.statut("erreur", fin=maintenant())
        resultat = {"statut": "erreur", "texte": texte, "directives_citees": [], "trace": {}}
    trace = dict(resultat.get("trace") or {}, appels_mistral=len(m.reponses), mode=a.mode)
    session.ecrire_atomique(trace_chemin, trace)
    if a.mode == "reel" and a.enregistrement:
        os.makedirs(os.path.dirname(os.path.abspath(a.enregistrement)), exist_ok=True)
        meta = json.loads(a.meta) if a.meta else {}
        session.ecrire_atomique(a.enregistrement, {
            "format": 1, "scenario": a.scenario, "demande": demande, "demandeur": demandeur, "date": date,
            "options": meta, "modele": mistral.MODELE, "session": os.path.basename(os.path.normpath(a.session)),
            "appels_mistral": len(m.reponses),
            "resultat": {"statut": resultat["statut"], "directives_citees": resultat["directives_citees"]},
            "validations": [{"valideurs": v["valideurs"], "accepte": v["accepte"]}
                            for v in trace.get("validations", [])],
            "reponses": m.reponses,
        })
    if a.mode == "rejeu" and isinstance(m, modele.ModeleRejoue) and m.restantes() and resultat["statut"] != "erreur":
        sortie.write("rejeu : %d réponse(s) enregistrée(s) non consommée(s)\n" % m.restantes())
    sortie.write(resultat["texte"] + "\n")
    return {"termine": 0, "bloque": 2}.get(resultat["statut"], 1)


def principal(argv=None, sortie=sys.stdout):
    ap = argparse.ArgumentParser(prog="orchestrateur", description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    sous = ap.add_subparsers(dest="commande", required=True)
    e = sous.add_parser("executer")
    e.add_argument("--demande")
    e.add_argument("--demandeur")
    e.add_argument("--date", help="date de la demande (AAAA-MM-JJ), transmise aux sous-agents")
    e.add_argument("--session", required=True)
    e.add_argument("--approbations", required=True)
    e.add_argument("--mode", choices=["reel", "rejeu"], default="reel")
    e.add_argument("--enregistrement")
    e.add_argument("--scenario")
    e.add_argument("--meta", help="options du passage (JSON), recopiées dans l'enregistrement")
    e.add_argument("--mcp-approve")
    e.add_argument("--cles")
    e.add_argument("--dossier-sous-agents", default=sous_agents.DOSSIER)
    e.add_argument("gate", nargs="+", help="commande du gate (stdio)")
    s = sous.add_parser("sous-agents")
    s.add_argument("--registre", help="registre du gate (défaut : demo/registre.json s'il existe)")
    s.add_argument("--catalogue", help="catalogue du compilateur (défaut sans registre : catalogue-demo.json)")
    s.add_argument("--dossier", default=sous_agents.DOSSIER)
    c = sous.add_parser("comparer")
    c.add_argument("journaux", nargs=2)
    a = ap.parse_args(argv)
    if a.commande == "executer":
        if a.mode == "rejeu" and not a.enregistrement:
            ap.error("--mode rejeu exige --enregistrement")
        return executer(a, sortie)
    if a.commande == "sous-agents":
        try:
            if a.catalogue:
                outils = sous_agents.outils_du_catalogue(a.catalogue)
            elif a.registre or os.path.exists(REGISTRE):
                outils = sous_agents.outils_du_registre(a.registre or REGISTRE)
            else:
                outils = sous_agents.outils_du_catalogue(CATALOGUE)
            agents = sous_agents.charger(outils, a.dossier)
        except sous_agents.SousAgentInvalide as err:
            sortie.write(str(err) + "\n")
            return 1
        for ag in agents.values():
            sortie.write("%s : %s\n  outils : %s\n" % (ag.nom, ag.role, ", ".join(ag.outils)))
        return 0
    ecarts = journal_gate.comparer(*a.journaux)
    for x in ecarts:
        sortie.write(x + "\n")
    sortie.write("aucun écart (à l'horodatage près)\n" if not ecarts else "%d écart(s)\n" % len(ecarts))
    return 0 if not ecarts else 1


if __name__ == "__main__":
    sys.exit(principal())
