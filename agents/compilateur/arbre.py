"""L'arbre de condition, à plat pour Mistral, reconstruit par le code.

Un schéma JSON récursif n'est pas forcément compris par Mistral. L'outil
``proposer_jeu`` décrit donc la condition comme une **liste de nœuds
identifiés**, chaque nœud citant ses enfants par identifiant, et la racine
par son identifiant. Le code reconstruit l'arbre (forme de
docs/schema-directive-v2.md §5) et refuse, avant même le chargeur Go :

- identifiant absent ou en double, enfant ou racine inconnus ;
- cycle, nœud cité par deux parents, nœud non rattaché à la racine ;
- mauvais nombre d'enfants pour le type, enfant d'une feuille ;
- paramètre qui n'appartient pas au type, paramètre manquant ;
- profondeur au-delà de 6, plus de 64 nœuds.

Un nœud à plat ::

    {"id": "n1", "type": "et", "enfants": ["n2", "n3"]}
    {"id": "n2", "type": "argument", "champ": "/montant", "op": ">", "valeur": 500000}
    {"id": "n3", "type": "non", "enfants": ["n4"]}
    {"id": "n4", "type": "a_consulte", "serveur": "gels-avoirs", "critere": "donnees_recues"}
"""

PROFONDEUR_MAX = 6
NOEUDS_MAX = 64

COMPOSES = {"et": (1, None), "ou": (1, None), "non": (1, 1), "existe": (1, 1), "tous": (1, 1)}

# Paramètres admis par type ; ceux entre parenthèses sont facultatifs.
PARAMETRES = {
    "et": set(), "ou": set(), "non": set(),
    "existe": {"champ"}, "tous": {"champ"},
    "argument": {"champ", "op", "(valeur)"},
    "a_consulte": {"(serveur)", "(outil)", "(etiquette)", "critere"},
    "appelant": {"(egal)", "(prefixe)"},
    "sous_agent_declare": {"(egal)", "(dans)"},
    "demandeur": {"(groupe)", "(identites)"},
    "attestation": {"verifiee", "(mode)"},
    "temps": {"(jours)", "(heure_de)", "(heure_a)", "(date_op)", "(date_valeur)", "fuseau"},
    "etiquette_cible": {"etiquette"},
    "metadonnee_reponse": {"(serveur)", "(etiquette)", "meta", "op", "valeur"},
    "sujet": {"groupe", "(champ)"},
}
TYPES = sorted(PARAMETRES)
CLES_STRUCTURE = {"id", "type", "enfants"}


def _pose(v):
    """Une valeur vide (None, "", [], {}) compte comme absente : Mistral
    remplit parfois les paramètres qui ne servent pas."""
    return v not in (None, "", [], {})


def _parametres(n):
    return {k: v for k, v in n.items() if k not in CLES_STRUCTURE and _pose(v)}


def _feuille(t, p):
    """Rend le nœud Go d'une feuille à partir de ses paramètres, déjà
    contrôlés (présence, admission)."""
    if t == "argument":
        f = {"champ": p.get("champ", ""), "op": p["op"]}
        if "valeur" in p and p["op"] not in ("present", "absent"):
            f["valeur"] = p["valeur"]
        return {"argument": f}
    if t == "a_consulte":
        f = {k: p[k] for k in ("serveur", "outil", "etiquette") if k in p}
        f["critere"] = p["critere"]
        return {"session.a_consulte": f}
    if t in ("appelant", "sous_agent_declare"):
        return {t: {k: p[k] for k in ("egal", "prefixe", "dans") if k in p}}
    if t == "demandeur":
        if "groupe" in p:
            return {"demandeur": {"groupe": p["groupe"]}}
        ids = p.get("identites")
        if isinstance(ids, list) and len(ids) == 1:
            return {"demandeur": {"identite": {"egal": ids[0]}}}
        return {"demandeur": {"identite": {"dans": ids}}}
    if t == "attestation":
        f = {"verifiee": p["verifiee"]}
        if "mode" in p:
            f["mode"] = p["mode"]
        return {"attestation": f}
    if t == "temps":
        f = {}
        if "jours" in p:
            f["jour_semaine"] = p["jours"]
        if "heure_de" in p or "heure_a" in p:
            f["heure"] = {"de": p.get("heure_de", ""), "a": p.get("heure_a", "")}
        if "date_op" in p or "date_valeur" in p:
            f["date"] = {"op": p.get("date_op", ""), "valeur": p.get("date_valeur", "")}
        f["fuseau"] = p["fuseau"]
        return {"temps": f}
    if t == "etiquette_cible":
        return {"etiquette_cible": {"contient": p["etiquette"]}}
    if t == "metadonnee_reponse":
        f = {k: p[k] for k in ("serveur", "etiquette") if k in p}
        f.update(champ=p["meta"], op=p["op"], valeur=p["valeur"])
        return {"metadonnee_reponse": f}
    if t == "sujet":
        f = {"groupe": p["groupe"]}
        if "champ" in p:
            f["champ"] = p["champ"]
        return {"sujet": f}
    raise AssertionError(t)


def reconstruire(noeuds, racine, prefixe=""):
    """Rend (arbre, erreurs). ``arbre`` est None dès qu'il y a une erreur :
    un arbre à moitié reconstruit ne doit jamais atteindre le chargeur.

    ``racine`` absente (None, "") : pas de condition, et la liste de nœuds
    doit être vide."""
    erreurs = []

    def err(msg):
        erreurs.append(prefixe + msg)

    noeuds = noeuds or []
    if not isinstance(noeuds, list):
        err("noeuds doit être une liste")
        return None, erreurs
    if not _pose(racine):
        if noeuds:
            err("%d nœud(s) sans racine : donner l'identifiant du nœud racine" % len(noeuds))
        return None, erreurs
    racine = str(racine)

    index = {}
    for i, n in enumerate(noeuds):
        if not isinstance(n, dict):
            err("nœud %d : objet attendu" % (i + 1))
            continue
        nid = n.get("id")
        if not _pose(nid):
            err("nœud %d : id manquant" % (i + 1))
            continue
        nid = str(nid)
        if nid in index:
            err("nœud %s : identifiant en double" % nid)
            continue
        index[nid] = n
    if len(noeuds) > NOEUDS_MAX:
        err("%d nœuds (au plus %d)" % (len(noeuds), NOEUDS_MAX))
    if not index:
        err("racine %s sans aucun nœud : sans condition, racine vaut \"\" ; sinon décris les nœuds de la condition" % racine)
    elif racine not in index:
        err("racine %s : aucun nœud ne porte cet identifiant" % racine)
    if erreurs:
        return None, erreurs

    parent = {}
    for nid, n in index.items():
        enfants = n.get("enfants") or []
        if not isinstance(enfants, list):
            err("nœud %s : enfants doit être une liste d'identifiants" % nid)
            continue
        for e in enfants:
            e = str(e)
            if e not in index:
                err("nœud %s : enfant %s inconnu" % (nid, e))
            elif e == nid:
                err("nœud %s : se cite lui-même (cycle)" % nid)
            elif e in parent:
                err("nœud %s : déjà enfant de %s ; un nœud n'a qu'un parent (dupliquer le nœud sous un autre identifiant)" % (e, parent[e]))
            else:
                parent[e] = nid
    if racine in parent:
        err("racine %s : citée comme enfant de %s (cycle)" % (racine, parent[racine]))
    if erreurs:
        return None, erreurs

    vus = set()

    def construire(nid, profondeur):
        vus.add(nid)
        n = index[nid]
        t = n.get("type")
        q = "nœud %s (%s) : " % (nid, t)
        if profondeur > PROFONDEUR_MAX:
            err(q + "profondeur %d (au plus %d)" % (profondeur, PROFONDEUR_MAX))
            return None
        if t not in PARAMETRES:
            err("nœud %s : type %r inconnu (%s)" % (nid, t, ", ".join(TYPES)))
            return None
        p = _parametres(n)
        admis = {x.strip("()") for x in PARAMETRES[t]}
        for k in sorted(p):
            if k not in admis:
                err(q + "le paramètre %s n'est pas admis pour ce type" % k)
        for x in sorted(PARAMETRES[t]):
            if not x.startswith("(") and x not in p and not (x == "champ" and "champ" in n):
                err(q + "paramètre %s manquant" % x)
        enfants = [str(e) for e in (n.get("enfants") or [])]
        if t in COMPOSES:
            mini, maxi = COMPOSES[t]
            if len(enfants) < mini or (maxi is not None and len(enfants) > maxi):
                attendu = "exactement %d" % mini if mini == maxi else "au moins %d" % mini
                err(q + "%s enfant(s) attendu(s), %d donné(s)" % (attendu, len(enfants)))
                return None
            sous = [construire(e, profondeur + 1) for e in enfants]
            if any(s is None for s in sous):
                return None
            if t in ("et", "ou"):
                return {t: sous}
            if t == "non":
                return {"non": sous[0]}
            return {t: {"champ": n.get("champ") if n.get("champ") is not None else "", "condition": sous[0]}}
        if enfants:
            err(q + "une feuille n'a pas d'enfant")
            return None
        if erreurs:
            return None
        return _feuille(t, p)

    arbre = construire(racine, 1)
    orphelins = sorted(set(index) - vus)
    if orphelins:
        err("nœud(s) %s non rattaché(s) à la racine %s" % (", ".join(orphelins), racine))
    if erreurs:
        return None, erreurs
    return arbre, []


def parcourir(arbre, visiter, dans_quantif=False):
    """Appelle ``visiter(cle, valeur, dans_quantif)`` sur chaque nœud Go de
    l'arbre, en profondeur, dans l'ordre."""
    if not isinstance(arbre, dict) or len(arbre) != 1:
        return
    (cle, v), = arbre.items()
    visiter(cle, v, dans_quantif)
    if cle in ("et", "ou"):
        for e in v:
            parcourir(e, visiter, dans_quantif)
    elif cle == "non":
        parcourir(v, visiter, dans_quantif)
    elif cle in ("existe", "tous"):
        parcourir(v.get("condition"), visiter, True)
