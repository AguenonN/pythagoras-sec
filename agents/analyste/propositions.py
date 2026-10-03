"""Des occurrences aux propositions (contrats de la démo, §6).

Un constat regroupe les occurrences d'un même risque sur une même action
(type, outil de l'action). Son ``detail`` est un texte français écrit par
le code, pour le juriste (la console le lit comme une chaîne) ; les
occurrences structurées et sourcées l'accompagnent dans
``constat.occurrences`` (champ en plus du contrat, ignoré par la console).

L'identifiant d'une proposition est stable (type et action) : une
proposition déjà écrite n'est jamais réécrite, qu'elle ait été décidée ou
non, et un risque que le juriste a rejeté n'est pas reproposé en silence.
L'Analyste n'écrit jamais ``<id>.decision.json``.
"""

import datetime
import json
import os
import tempfile

from . import couverture, detecteurs as det

ENTETES = {
    det.SOURCE_INTERNE_PERIMEE: "source interne périmée",
    det.ACTION_SANS_CONTROLE: "action sans contrôle humain",
    det.DONNEES_PERSONNELLES_EXTERNE: "données personnelles vers l'extérieur",
    det.ENTREPRISE_NON_RETROUVEE: "entreprise non retrouvée au répertoire SIRENE",
    det.CONTOURNEMENT_RETRAIT: "contournement d'un retrait de champs",
}

COURTS = {
    det.SOURCE_INTERNE_PERIMEE: "source-interne-perimee",
    det.ACTION_SANS_CONTROLE: "action-sans-controle",
    det.DONNEES_PERSONNELLES_EXTERNE: "donnees-personnelles-externe",
    det.ENTREPRISE_NON_RETROUVEE: "entreprise-non-retrouvee",
    det.CONTOURNEMENT_RETRAIT: "contournement-retrait",
}


LIBELLES = {t: e[0].upper() + e[1:] for t, e in ENTETES.items()}


def trimestre_fr(t):
    """« 2026-Q3 » → « T3 2026 » : ce que la console affiche est en français."""
    try:
        annee, q = t.split("-Q")
        return "T%s %s" % (q, annee)
    except (ValueError, AttributeError):
        return t


def identifiant(type_, action):
    return "A-%s-%s" % (COURTS[type_], action.replace("/", "."))


def _montant(v):
    if isinstance(v, float) and v.is_integer():
        v = int(v)
    if isinstance(v, int):
        return "{:,}".format(v).replace(",", " ") + " €"
    return str(v) + " €"


def _decision_fr(o):
    d = o["decision"]
    eff = {"autoriser": "autorisée", "attendre_validation": "mise en attente de validation",
           "refuser": "refusée"}.get(d["effet"], d["effet"])
    s = eff
    if d["directives_appliquees"]:
        s += " (" + ", ".join(d["directives_appliquees"]) + ")"
    if d["validation_humaine"] == "acceptee":
        s += ", validée par un humain"
    s += ", transmise" if o["transmise"] else ", non transmise"
    return s


def _ligne(type_, o):
    tete = "%s, entrée %s (%s%s) : " % (o["session"], o["seq_action"], o["date_action"][:10],
                                       ", dossier " + o["valeurs_coffre"]["dossier"]
                                       if o["valeurs_coffre"].get("dossier") else "")
    if type_ == det.SOURCE_INTERNE_PERIMEE:
        ci = o["consultation_interne"]
        if o["source_officielle_consultee"]:
            off = o["consultations_officielles"][-1]
            ref = "source officielle consultée à l'entrée %s (%s, %s)" % (off["seq"], off["outil"],
                                                                         trimestre_fr(off["trimestre"]))
        else:
            ref = "taux officiel non consulté, trimestre en vigueur %s" % trimestre_fr(o["trimestre_reference"])
        return tete + "base interne consultée à l'entrée %s (%s) ; %s ; action %s." % (
            ci["seq"], trimestre_fr(ci["trimestre"]), ref, _decision_fr(o))
    if type_ == det.ACTION_SANS_CONTROLE:
        return tete + "montant %s, au moins égal au seuil de %s ; action %s, sans directive appliquée." % (
            _montant(o["valeurs_coffre"]["montant"]), _montant(o["seuil"]), _decision_fr(o))
    if type_ == det.DONNEES_PERSONNELLES_EXTERNE:
        cs = ", ".join("entrée %s (%s)" % (c["seq"], c["outil"]) for c in o["consultations_personnelles"])
        return tete + "données personnelles consultées : %s ; action vers une destination externe %s." % (
            cs, _decision_fr(o))
    if type_ == det.ENTREPRISE_NON_RETROUVEE:
        siren = o["valeurs_coffre"]["siren"]
        if o["motif"] == "introuvable":
            rs = ", ".join("entrée %s, aucun résultat (réponse du %s)" % (r["seq"], r["date_source"][:10])
                           for r in o["recherches"])
            quoi = "SIREN %s recherché au répertoire : %s" % (siren, rs)
        else:
            quoi = "SIREN %s jamais recherché au répertoire dans la session" % siren
        return tete + "%s ; action %s." % (quoi, _decision_fr(o))
    if type_ == det.CONTOURNEMENT_RETRAIT:
        cav = o["retrait"]
        cs = ", ".join(str(c["seq"]) for c in o["contournements"])
        return tete + ("champs retirés de %s à l'entrée %s par %s (%s) ; puis %s, qui porte aussi "
                       "l'étiquette %s, appelé sans retrait, données reçues, aux entrées %s.") % (
            cav["outil"], cav["seq"], ", ".join(cav["directives"]), ", ".join(cav["champs_retires"]),
            o["action"], ", ".join(o["contournements"][0]["etiquettes_communes"]), cs)
    raise ValueError(type_)


def _resume(type_, action, occ, catalogue):
    n = len(occ)
    if type_ == det.SOURCE_INTERNE_PERIMEE:
        ci = occ[0]["consultation_interne"]
        sans = sum(1 for o in occ if not o["source_officielle_consultee"])
        return ("%d action(s) %s transmise(s) dans une session qui a consulté la base interne des taux (%s), "
                "dont le trimestre (%s, lu dans %s) est antérieur au trimestre de référence ; %d sans aucune "
                "consultation du taux officiel. Aucune directive ne porte sur la consultation de la base interne."
                % (n, action, ci["outil"], trimestre_fr(ci["trimestre"]), ci["origine_trimestre"], sans))
    if type_ == det.ACTION_SANS_CONTROLE:
        return ("%d action(s) %s transmise(s) sans validation humaine ni directive appliquée, pour un montant "
                "au moins égal à %s." % (n, action, _montant(occ[0]["seuil"])))
    if type_ == det.DONNEES_PERSONNELLES_EXTERNE:
        return ("%d action(s) %s vers une destination étiquetée externe, après consultation de données "
                "personnelles dans la même session, sans directive qui la retienne." % (n, action))
    if type_ == det.ENTREPRISE_NON_RETROUVEE:
        outil = catalogue["repertoire_entreprises"]["outil"]
        return ("%d action(s) %s présentée(s) pour une entreprise que la session n'a pas retrouvée au répertoire "
                "SIRENE (%s) ; aucune directive ne porte sur ce contrôle." % (n, action, outil))
    if type_ == det.CONTOURNEMENT_RETRAIT:
        cav = occ[0]["retrait"]
        return ("Dans %d session(s), des champs ont été retirés de %s (%s), puis %s, qui porte la même donnée "
                "(étiquette %s), l'a rendue sans retrait : le retrait a pu être contourné."
                % (n, cav["outil"], ", ".join(cav["directives"]), action,
                   ", ".join(occ[0]["contournements"][0]["etiquettes_communes"])))
    raise ValueError(type_)


def elements_risque(type_, occ, catalogue):
    """Ce sur quoi une directive qui couvre ce risque porterait."""
    if type_ == det.SOURCE_INTERNE_PERIMEE:
        return ["outil:" + catalogue["taux"]["interne"]["outil"]]
    if type_ == det.ACTION_SANS_CONTROLE:
        return ["champ:" + catalogue["montant"]["argument"]]
    if type_ == det.DONNEES_PERSONNELLES_EXTERNE:
        return ["etiquette:" + catalogue["etiquettes"]["donnees_personnelles"]]
    if type_ == det.ENTREPRISE_NON_RETROUVEE:
        return det.elements_repertoire(catalogue)
    if type_ == det.CONTOURNEMENT_RETRAIT:
        cav = occ[0]["retrait"]
        return ["outil:" + cav["outil"]] + ["etiquette:" + e for e in cav["etiquettes_propres"]]
    raise ValueError(type_)


def _couvert_aujourdhui(type_, action, occ, catalogue, jeu_courant):
    serveur, outil = action.split("/", 1)
    ids = couverture.couvert_aujourdhui(jeu_courant, serveur, outil, elements_risque(type_, occ, catalogue))
    if type_ == det.CONTOURNEMENT_RETRAIT:
        # Une directive qui retire des champs de l'outil du contournement, ou le refuse,
        # le ferme, quelle que soit sa condition.
        for d in (jeu_courant or {}).get("directives", []):
            if d.get("statut") in couverture.EVALUEES and couverture.vise(d, serveur, outil) and \
                    any(e.get("effet") in ("caviarder", "refuser") for e in d.get("effets") or []) and \
                    d.get("id") not in ids:
                ids.append(d.get("id"))
        return ids
    if type_ != det.ACTION_SANS_CONTROLE:
        return ids
    # Un seuil de montant ne couvre que les montants au-delà : il faut qu'il
    # retienne le plus petit montant relevé.
    plus_petit = min(o["valeurs_coffre"]["montant"] for o in occ)
    par_id = couverture.directives_par_id(jeu_courant)
    return [i for i in ids if any(s <= plus_petit for s in couverture.seuils(par_id[i].get("condition"),
                                                                                catalogue["montant"]["argument"]))]


def regrouper(occurrences, catalogue, jeu_courant):
    """Constats par (type, action). Rend (constats, couverts) : un constat
    déjà couvert par le jeu courant n'est pas proposé."""
    constats, couverts = [], []
    for type_ in det.TYPES:
        par_action = {}
        for o in occurrences.get(type_, []):
            par_action.setdefault(o["action"], []).append(o)
        for action in sorted(par_action):
            occ = par_action[action]
            sessions = sorted({o["session"] for o in occ})
            c = {
                "id": identifiant(type_, action),
                "type": type_,
                "action": action,
                "sessions": sessions,
                "detail": "Constat : %s. %s\n%s" % (ENTETES[type_], _resume(type_, action, occ, catalogue),
                                                    "\n".join("- " + _ligne(type_, o) for o in occ)),
                "occurrences": occ,
            }
            ids = _couvert_aujourdhui(type_, action, occ, catalogue, jeu_courant)
            if ids:
                couverts.append({"id": c["id"], "directives": ids})
                continue
            constats.append(c)
    return constats, couverts


FAITS_COMMUNS = {
    det.SOURCE_INTERNE_PERIMEE: "dans toutes les occurrences, la session a consulté {interne} avant l'action",
    det.ACTION_SANS_CONTROLE: "dans toutes les occurrences, l'argument {montant} de l'action est au moins égal au "
                              "seuil de {seuil}",
    det.DONNEES_PERSONNELLES_EXTERNE: "dans toutes les occurrences, la session a consulté un serveur étiqueté "
                                      "{perso} avant une action vers un serveur étiqueté {externe}",
    det.ENTREPRISE_NON_RETROUVEE: "dans toutes les occurrences, l'entreprise désignée par l'action n'a pas été "
                                  "retrouvée par {repertoire} dans la session",
    det.CONTOURNEMENT_RETRAIT: "dans toutes les occurrences, la session a consulté {caviarde} avant l'appel à "
                                  "l'outil visé, qui porte l'étiquette {etiquettes} et dont rien n'a été retiré",
}

LIMITES = {
    det.ENTREPRISE_NON_RETROUVEE: "le gate ne lit pas le contenu des réponses : il sait si {repertoire} a été "
                                  "consulté dans la session (avec données reçues ou non), pas si la recherche a "
                                  "trouvé l'entreprise ; le constat vient de la relecture des réponses datées du "
                                  "répertoire, que l'engine ne fait pas",
}


def _etiquettes_serveur(constat, sessions):
    """Étiquettes de la cible de l'action, telles que le journal les a
    consignées dans les sessions du constat."""
    out = set()
    for s in sessions:
        if s.nom in constat["sessions"]:
            for a in s.appels:
                if a.nom == constat["action"]:
                    out |= set(a.etiquettes_cible)
    return out


def _effet_fr(e):
    """Un effet de directive dit en français, pour le contexte du modèle."""
    if e.get("effet") == "caviarder":
        return "retirer de l'appel les champs autres que " + ", ".join(e.get("champs_autorises") or [])
    if e.get("effet") == "attendre_validation":
        return "attendre la validation humaine" + (" du rôle " + e["par_role"] if e.get("par_role") else "")
    if e.get("effet") == "refuser":
        return "refuser l'appel"
    if e.get("effet") == "notifier":
        return "notifier le rôle " + e.get("destinataire_role", "")
    return e.get("effet", "")


def contexte(constat, sessions, jeu_courant, catalogue):
    """Ce que le modèle peut savoir en plus du constat, tiré du journal et
    du jeu (jamais du coffre) : les rôles qui ont validé cette action dans
    les sessions lues, les fondements des directives qui la visent, le fait
    observable commun aux occurrences, et ce que l'engine ne peut pas voir."""
    serveur, outil = constat["action"].split("/", 1)
    roles = set()
    for s in sessions:
        for a in s.appels:
            if a.nom == constat["action"] and a.validee_par_humain:
                roles |= set((a.approbation or {}).get("roles") or [])
    directives = []
    for d in (jeu_courant or {}).get("directives", []):
        if d.get("statut") in couverture.EVALUEES and couverture.vise(d, serveur, outil):
            directives.append({"id": d.get("id"), "texte": d.get("texte_fr", ""), "fondements": [
                f.get("texte", "") + (", article " + f["article"] if f.get("article") else "")
                for f in d.get("fondements") or []]})
    valeurs = {"interne": catalogue["taux"]["interne"]["outil"], "montant": catalogue["montant"]["argument"],
               "perso": catalogue["etiquettes"]["donnees_personnelles"],
               "externe": catalogue["etiquettes"]["externe"],
               "repertoire": catalogue["repertoire_entreprises"]["outil"],
               "seuil": _montant(constat["occurrences"][0].get("seuil", 0)),
               "caviarde": (constat["occurrences"][0].get("retrait") or {}).get("outil", ""),
               "etiquettes": ", ".join((constat["occurrences"][0].get("contournements") or [{}])[0]
                                       .get("etiquettes_communes", []))}
    out = {"roles_ayant_valide_cette_action": sorted(roles),
           "directives_qui_visent_cette_action": directives,
           "fait_observable_commun": FAITS_COMMUNS[constat["type"]].format(**valeurs)}
    if constat["type"] == det.CONTOURNEMENT_RETRAIT:
        par_id = couverture.directives_par_id(jeu_courant)
        for s_ in sessions:
            if s_.nom in constat["sessions"]:
                par_id.update(couverture.directives_par_id(couverture.jeu_de_session(s_, jeu_courant)))
        ids = sorted({i for o in constat["occurrences"] for i in o["retrait"]["directives"]})
        out["directives_contournees"] = [{"id": i, "texte": par_id[i].get("texte_fr", ""),
                                          "cible": par_id[i].get("declencheur", {}).get("cible"),
                                          "effets": [_effet_fr(e) for e in par_id[i].get("effets") or []],
                                          "fondements": [f.get("texte", "") + (", article " + f["article"]
                                                                               if f.get("article") else "")
                                                         for f in par_id[i].get("fondements") or []]}
                                         for i in ids if i in par_id]
    if not out["roles_ayant_valide_cette_action"]:
        # Aucun rôle n'a encore validé cette action : les rôles de validation
        # des directives qui visent le même serveur (fait du jeu).
        roles_serveur = set()
        for d in (jeu_courant or {}).get("directives", []):
            cible = (d.get("declencheur") or {}).get("cible") or {}
            if d.get("statut") in couverture.EVALUEES and cible.get("serveur") == serveur:
                roles_serveur |= {e["par_role"] for e in d.get("effets") or [] if e.get("par_role")}
        for d in (jeu_courant or {}).get("directives", []):
            # D-006/D-007 visent une étiquette, pas un serveur : celles du
            # serveur de l'action comptent aussi.
            cible = (d.get("declencheur") or {}).get("cible") or {}
            if d.get("statut") in couverture.EVALUEES and cible.get("etiquette") in _etiquettes_serveur(
                    constat, sessions):
                roles_serveur |= {e["par_role"] for e in d.get("effets") or [] if e.get("par_role")}
        out["roles_des_directives_du_meme_serveur"] = sorted(roles_serveur)
    if constat["type"] in LIMITES:
        out["limite_d_observation"] = LIMITES[constat["type"]].format(**valeurs)
    return out


def proposition(constat, redaction, date=None, extras=None):
    """``extras`` : champs facultatifs (``sessions_similaires``,
    ``fondements_suggeres``), ignorés par la console tant qu'elle ne les
    affiche pas."""
    date = date or datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0)
    return {
        "id": constat["id"],
        "date": date.isoformat().replace("+00:00", "Z"),
        # « type » est affiché par la console : libellé français ; « code »
        # garde le type machine.
        "constat": {"type": LIBELLES[constat["type"]], "code": constat["type"], "detail": constat["detail"],
                    "occurrences": constat["occurrences"]},
        "sessions": constat["sessions"],
        "texte_propose": redaction["texte_propose"],
        "fondement_propose": redaction["fondement_propose"],
        "explication": redaction["explication"],
        **(extras or {}),
    }


def deja_proposee(dossier, id_):
    return os.path.exists(os.path.join(dossier, id_ + ".json")) or \
        os.path.exists(os.path.join(dossier, id_ + ".decision.json"))


def ecrire(dossier, prop):
    """Écriture atomique de ``<id>.json`` ; jamais par-dessus une
    proposition existante (rend False)."""
    os.makedirs(dossier, exist_ok=True)
    if deja_proposee(dossier, prop["id"]):
        return False
    fd, tmp = tempfile.mkstemp(dir=dossier, prefix=".tmp-", suffix=".part")
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            json.dump(prop, f, ensure_ascii=False, indent=1)
            f.write("\n")
        # link échoue si la cible existe : pas d'écrasement, même en course.
        os.link(tmp, os.path.join(dossier, prop["id"] + ".json"))
    except FileExistsError:
        return False
    finally:
        os.unlink(tmp)
    return True
