"""Compilation d'une directive : Mistral propose, le code vérifie, le juriste
décide.

    texte du juriste (+ réponses) ──► Mistral ──► poser_question ──► questions
                                           └────► proposer_directive
                                                     │
                     vérifications (rien d'inventé) + directive verifier (Go)
                                                     │ refus : une correction
                                                     ▼
                                   directive relire (Go) ──► relecture

Ce que Mistral ne fixe jamais : le statut (toujours brouillon), texte_fr
(le texte du juriste tel quel), l'identifiant, la version, la date d'effet
(donnée par le juriste) et si_indetermine.

si_indetermine : pour toute directive à condition, le compilateur (le code,
pas Mistral) pose une question fermée au juriste. « ignorer » n'est retenu
que sur cette réponse, qui est consignée dans le résultat (``consignation``)
et nomme l'auteur dans si_indetermine_valide_par. Les réponses aux questions
du code ne sont jamais envoyées à Mistral.
"""

import datetime
import json
import re

from . import directive_go, outils, verifications

QUESTIONS = "questions"
DIRECTIVE = "directive"
ERREURS = "erreurs"  # aucune proposition exploitable
REFUSE = "refuse"    # un jeu a été construit et refusé par les vérifications

ORIGINE_CODE = "compilateur"
QUESTION_INDETERMINE = {
    "question": ("Si une condition de la directive ne peut pas être évaluée (argument absent ou de type "
                 "inattendu), faut-il refuser l'appel (choix par défaut) ou ignorer la directive pour cet "
                 "appel ? Répondez « refuser » ou « ignorer »."),
    "champ": "si_indetermine",
    "options": ["refuser", "ignorer"],
    "origine": ORIGINE_CODE,
}


def _du_code(r):
    return r.get("origine") == ORIGINE_CODE


def reponse_indetermine(reponses):
    """La réponse consignée à la question du code, normalisée, ou None si
    elle manque ou n'est pas l'une des options."""
    for r in reversed(reponses):
        if _du_code(r) and r.get("champ") == "si_indetermine":
            choix = verifications.sans_accents(r.get("reponse", "")).strip().lower().rstrip(".")
            return (choix, r) if choix in QUESTION_INDETERMINE["options"] else None
    return None


def message_utilisateur(texte, catalogue, reponses):
    parts = [
        "Catalogue des serveurs et outils disponibles :",
        catalogue.pour_le_modele(),
        "",
        "Texte de la directive, écrit par le juriste :",
        "<<<",
        texte,
        ">>>",
    ]
    reponses = [r for r in reponses if not _du_code(r)]
    if reponses:
        parts += ["", "Réponses du juriste à tes questions :"]
        for r in reponses:
            parts += ["- Question : " + r["question"], "  Réponse : " + r["reponse"]]
    return "\n".join(parts)


def _date(iso, cle):
    try:
        datetime.date.fromisoformat(iso)
    except (TypeError, ValueError):
        return None, "%s : %r n'est pas une date AAAA-MM-JJ" % (cle, iso)
    return iso + "T00:00:00Z", None


_ARTICLE = re.compile(r"^\s*(?:l['’])?art(?:icle|\.)?\s*([A-Za-z]?\d[\w.\-§]*)\s+(?:du |de la |des |de l['’])?(\S.*)$", re.IGNORECASE)
_PREFIXE_ARTICLE = re.compile(r"^\s*art(?:icle|\.)?\s*", re.IGNORECASE)


def fondement(f):
    """Range « Art. 22 RGPD » en texte RGPD, article 22 : même contenu,
    autrement découpé ; rien n'est ajouté."""
    texte = (f.get("texte") or "").strip()
    article = (f.get("article") or "").strip()
    m = _ARTICLE.match(texte)
    if m and not article:
        article, texte = m.group(1).strip(), m.group(2).strip()
    article = _PREFIXE_ARTICLE.sub("", article)
    out = {"texte": texte}
    if article:
        out["article"] = article
    return out


def construire_jeu(p, texte, jeu_id, auteur, date_effet, id_directive, ignorer_valide_par=None):
    """``ignorer_valide_par`` n'est passé que par ``compiler``, sur la réponse
    consignée du juriste à la question du code."""
    """Assemble le jeu d'une directive à partir de la proposition brute.

    Rend (jeu, erreurs) ; les erreurs sont celles qu'on ne peut pas même
    confier au chargeur (date illisible)."""
    erreurs = []
    d = {
        "id": id_directive,
        "version": 1,
        "statut": "brouillon",
        "date_effet": date_effet,
    }
    if p.get("revision_avant"):
        rev, err = _date(p["revision_avant"], "revision_avant")
        if err:
            erreurs.append(err)
        else:
            d["revision_avant"] = rev
    d["texte_fr"] = texte
    d["fondement"] = fondement(p.get("fondement") or {})
    d["verrou"] = p.get("verrou", "")
    cible = p.get("cible") or {}
    d["cible"] = {"serveur": cible.get("serveur", ""), "outil": cible.get("outil", "")}
    if p.get("perimetre"):
        d["perimetre"] = {"appelants": list(p["perimetre"])}
    if p.get("condition"):
        conds = []
        for c in p["condition"]:
            cond = {"champ": c.get("champ", ""), "op": c.get("op", "")}
            if c.get("valeur") is not None and c.get("op") not in ("present", "absent"):
                cond["valeur"] = c["valeur"]
            conds.append(cond)
        d["condition"] = conds
        if ignorer_valide_par:
            d["si_indetermine"] = "ignorer"
            d["si_indetermine_valide_par"] = ignorer_valide_par
        else:
            d["si_indetermine"] = "refuser"
    if p.get("provenance"):
        pr = {k: p["provenance"][k] for k in ("serveurs", "outils", "critere") if p["provenance"].get(k)}
        d["provenance"] = pr
    if p.get("champs"):
        d["champs"] = {"autorises": list(p["champs"].get("autorises") or []),
                       "mode": p["champs"].get("mode", "")}
    d["effet"] = p.get("effet", "")
    jeu = {
        "jeu": {"id": jeu_id, "version": 1, "date_effet": date_effet, "auteur": auteur},
        "directives": [d],
    }
    return jeu, erreurs


def _appels(reponse):
    try:
        message = reponse["choices"][0]["message"]
    except (KeyError, IndexError, TypeError):
        return None, []
    appels = []
    for tc in message.get("tool_calls") or []:
        fn = tc.get("function", {})
        args = fn.get("arguments", "{}")
        if isinstance(args, str):
            try:
                args = json.loads(args)
            except ValueError:
                args = {"_illisible": args}
        appels.append((tc.get("id", ""), fn.get("name", ""), args))
    return message, appels


def _questions(appels):
    out = []
    for _, nom, args in appels:
        if nom == "poser_question":
            out.append({
                "question": args.get("question", ""),
                "champ": args.get("champ", "autre"),
                "options": list(args.get("options") or []),
            })
    return out


def _refuser(resultat):
    """Marque le jeu refusé : état refuse, et le jeu quitte « jeu » pour
    « jeu_refuse », afin qu'aucun appelant ne le reprenne par mégarde."""
    resultat.update(etat=REFUSE, jeu_refuse=resultat["jeu"], jeu=None, relecture=None)
    return resultat


def _conclure(resultat, p, texte, jeu_id, auteur, date_effet, id_directive, reponses):
    """Proposition valide : pose la question du code sur si_indetermine si
    la directive a une condition, puis rend la directive et sa relecture."""
    if resultat["jeu"]["directives"][0].get("condition"):
        choix = reponse_indetermine(reponses)
        if choix is None:
            resultat.update(etat=QUESTIONS, questions=[dict(QUESTION_INDETERMINE)], jeu=None)
            return resultat
        valeur, consignee = choix
        resultat["consignation"] = [consignee]
        if valeur == "ignorer":
            jeu, _ = construire_jeu(p, texte, jeu_id, auteur, date_effet, id_directive, ignorer_valide_par=auteur)
            ok, err_go, _ = directive_go.verifier(jeu)
            if not ok:
                resultat.update(erreurs=err_go, jeu=jeu)
                return _refuser(resultat)
            resultat["jeu"] = jeu
    resultat.update(etat=DIRECTIVE, erreurs=[], relecture=directive_go.relire(resultat["jeu"]))
    return resultat


def compiler(texte, jeu_id, catalogue, client, auteur, date_effet,
             reponses=(), id_directive="D-001"):
    """Rend un dict : etat (questions | directive | refuse | erreurs) et le détail.

    ``date_effet`` est un horodatage RFC 3339 fixé par le juriste.
    ``reponses`` : [{"question", "reponse", "champ"?, "options"?, "origine"?}]."""
    reponses = list(reponses)
    source = "\n".join([texte] + [r["reponse"] for r in reponses if not _du_code(r)])
    resultat = {"texte": texte, "reponses": reponses, "questions": [], "erreurs": [],
                "jeu": None, "relecture": None, "tentatives": 0, "consignation": []}
    messages = [
        {"role": "system", "content": outils.CONSIGNE},
        {"role": "user", "content": message_utilisateur(texte, catalogue, reponses)},
    ]
    offerts = outils.outils(catalogue)

    for tentative in (1, 2):
        resultat["tentatives"] = tentative
        reponse = client.completer(messages, offerts)
        message, appels = _appels(reponse)
        questions = _questions(appels)
        if questions:
            # Une question l'emporte toujours sur une proposition simultanée :
            # le doute se lève chez le juriste, pas dans le code.
            resultat.update(etat=QUESTIONS, questions=questions)
            return resultat
        propositions = [(i, a) for i, n, a in appels if n == "proposer_directive"]
        if not propositions:
            resultat.update(etat=ERREURS, erreurs=["Mistral n'a appelé aucun des deux outils"])
            return resultat

        erreurs = []
        if len(propositions) > 1:
            erreurs.append("une seule directive par proposition (%d reçues)" % len(propositions))
        p = propositions[0][1]
        jeu, err_jeu = construire_jeu(p, texte, jeu_id, auteur, date_effet, id_directive)
        erreurs += err_jeu
        erreurs += verifications.verifier_proposition(p, source, catalogue)
        ok, err_go, _ = directive_go.verifier(jeu)
        erreurs += err_go
        resultat["jeu"] = jeu
        if not erreurs and ok:
            return _conclure(resultat, p, texte, jeu_id, auteur, date_effet, id_directive, reponses)
        resultat["erreurs"] = erreurs
        if tentative == 2:
            break
        # Une seule correction : les erreurs retournent à Mistral, telles quelles.
        messages.append({"role": "assistant", "content": message.get("content") or "",
                         "tool_calls": message["tool_calls"]})
        for i, nom, _ in appels:
            contenu = ("Refusé par le vérificateur :\n- " + "\n- ".join(erreurs) +
                       "\nCorrige la directive, ou pose une question si une information manque."
                       if nom == "proposer_directive" else "Ignoré.")
            messages.append({"role": "tool", "tool_call_id": i, "name": nom, "content": contenu})

    return _refuser(resultat)
