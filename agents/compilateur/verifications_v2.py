"""Ce que le code vérifie d'un jeu v2 proposé, avant le chargeur strict : que
Mistral n'a rien inventé.

Le chargeur Go garantit la forme ; il ne sait pas si un montant, un fuseau,
une étiquette ou un rôle vient du juriste. Chaque valeur est comparée au
texte du juriste et à ses réponses (``source``), chaque nom au vocabulaire.
Les énumérations du function calling bornent déjà les noms ; ce module
vérifie en plus qu'un nom fermé (étiquette, rôle, groupe, fuseau) a été
écrit ou choisi par le juriste, et qu'une condition sur un argument porte
sur un argument de chaque outil visé (sinon elle serait indéterminée, donc
refusée, sur les autres).
"""

import re

from . import arbre as arbre_mod
from .verifications import _normaliser, dates, sans_accents, valeur_ecrite

JOURS = ["lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi", "dimanche"]
_WEEKEND = re.compile(r"\bweek[\s-]?ends?\b")
_HEURE = re.compile(r"\b(\d{1,2})\s*(?:h|heures?|:)\s*(\d{2})?(?![\d])")
_QUATRE_YEUX = re.compile(r"\b(?:quatre|4)[\s-]yeux\b|\bdeux (?:personnes|validateurs|signataires)\b|\bdouble validation\b")


def _nom(s):
    """Forme de comparaison souple : sans accents, minuscules, ponctuation,
    tirets, soulignés et barres lus comme des espaces."""
    return " ".join(re.sub(r"[^a-z0-9]+", " ", sans_accents(s).lower()).split())


def nom_ecrit(nom, source):
    """Le nom figure-t-il dans la source, mot pour mot, à la ponctuation près ?"""
    return bool(_nom(nom)) and (" " + _nom(nom) + " ") in (" " + _nom(source) + " ")


def _jetons(s):
    return re.findall(r"[a-z0-9][a-z0-9_-]*[a-z0-9]|[a-z0-9]", sans_accents(s).lower())


def nom_ferme_ecrit(nom, source):
    """Un nom fermé (étiquette, rôle, groupe) est écrit si le juriste l'a écrit
    sous sa forme exacte, tirets et soulignés compris : « données client »
    ne vaut pas donnees_client, qu'il faut choisir dans une réponse."""
    cible = _jetons(nom)
    if not cible:
        return False
    jetons = _jetons(source)
    n = len(cible)
    return any(jetons[i:i + n] == cible for i in range(len(jetons) - n + 1))


def fuseau_ecrit(fuseau, source):
    """Un fuseau est écrit si son nom IANA l'est, ou sa ville (« heure de
    Paris » justifie Europe/Paris)."""
    if nom_ecrit(fuseau, source):
        return True
    ville = fuseau.rsplit("/", 1)[-1]
    return ville != fuseau and nom_ecrit(ville, source)


def jours_ecrits(source):
    t = _nom(source)
    out = {j for j in JOURS if re.search(r"\b" + j + r"s?\b", t)}
    if _WEEKEND.search(sans_accents(source).lower()):
        out |= {"samedi", "dimanche"}
    return out


def heures_ecrites(source):
    t = sans_accents(source).lower()
    out = set()
    for h, m in _HEURE.findall(t):
        if int(h) <= 24 and (not m or int(m) < 60):
            out.add("%02d:%s" % (int(h) % 24, m or "00"))
    if re.search(r"\bminuit\b", t):
        out.add("00:00")
    if re.search(r"\bmidi\b", t):
        out.add("12:00")
    return out


def article_ecrit_apres(texte, source):
    """L'article que le juriste écrit juste après le nom du texte (« Code du
    travail, art. L1121-1 »), ou None."""
    m = re.search(re.escape(texte) + r"\s*,?\s*(?:l['’])?art(?:icle|\.)?\s*([A-Za-z]?\d[\w.\-]*\w)", source,
                  re.IGNORECASE)
    return m.group(1) if m else None


def _racine(mot):
    return mot[:-1] if len(mot) > 3 and mot.endswith(("s", "x")) else mot


def source_designee(serveur, source):
    """Le juriste a-t-il désigné ce serveur ? Chaque mot de son nom figure
    dans la source, au pluriel près (« registre des gels d'avoirs » désigne
    gels-avoirs, « taux officiel » taux-officiel)."""
    mots = {_racine(m) for m in _nom(source).split()}
    return all(_racine(m) in mots for m in _nom(serveur).split())


def transmis_ecrit(source):
    """Le juriste a-t-il dit qu'un appel parti suffit, quel qu'en soit le
    résultat ? Sinon « consulté » veut dire « données reçues », le plus fermé."""
    t = sans_accents(source).lower()
    return bool(re.search(r"\btransmis\b|quel(?:le)?s? qu.en soit le resultat|quel que soit (?:le|son) resultat|meme sans (?:reponse|resultat)", t))


def quatre_yeux_ecrit(source):
    return bool(_QUATRE_YEUX.search(sans_accents(source).lower()))


class _Verif:
    def __init__(self, vocab, source, prefixe):
        self.vocab = vocab
        self.source = source
        self.prefixe = prefixe
        self.erreurs = []

    def err(self, msg, *args):
        self.erreurs.append(self.prefixe + (msg % args if args else msg))

    def nom_ferme(self, genre, nom, connus):
        if nom not in connus:
            self.err("%s %r inconnu (%s)", genre, nom, ", ".join(connus) or "aucun")
        elif not nom_ferme_ecrit(nom, self.source):
            self.err("%s %r n'est ni écrit ni choisi par le juriste ; pose une question en proposant les %ss connus",
                     genre, nom, genre)

    def valeur(self, ou, v):
        if not valeur_ecrite(v, self.source):
            self.err("%s : la valeur %r n'est pas écrite par le juriste ; pose une question", ou, v)


def verifier_directive(p, d, vocab, source, prefixe):
    """``p`` : la directive proposée par Mistral (brute) ; ``d`` : la
    directive v2 construite. Rend la liste des erreurs."""
    v = _Verif(vocab, source, prefixe)
    cat = vocab.catalogue
    cible = (d.get("declencheur") or {}).get("cible") or {}
    serveur, outil = cible.get("serveur", "*"), cible.get("outil", "*")
    if serveur != "*" and serveur not in cat.serveurs:
        v.err("declencheur : serveur %s absent du catalogue", serveur)
    elif not cat.cible_existe(serveur, outil):
        v.err("declencheur : outil %s absent du serveur %s", outil, serveur)
    if cible.get("etiquette"):
        v.nom_ferme("etiquette", cible["etiquette"], vocab.etiquettes)
    vises = vocab.outils_vises(cible)
    if cible.get("acces"):
        sans = vocab.outils_vises({k: x for k, x in cible.items() if k != "acces"})
        if sans and sans == vises:
            v.err("declencheur : acces %s ne restreint rien (tous les outils visés sont déjà classés %s) ; "
                  "le retirer : une donnée du registre absente rendrait la cible indéterminée", cible["acces"], cible["acces"])
    if not vises and not cible.get("etiquette"):
        v.err("declencheur : la cible ne vise aucun outil du catalogue")

    def pointeur_racine(ou, champ):
        """Un pointeur hors quantificateur doit exister pour chaque outil visé."""
        if cible.get("etiquette") and serveur == "*":
            v.err("%s : une condition sur l'argument %s exige une cible dont les outils sont connus "
                  "(serveur nommé), pas seulement une étiquette", ou, champ)
            return
        manquants = ["%s/%s" % so for so in vises if champ not in vocab.arguments_de(*so)]
        if manquants:
            v.err("%s : l'argument %s n'existe pas pour %s, visé par la cible : la condition y serait "
                  "indéterminée et l'appel refusé ; restreins la cible ou fais une directive par outil",
                  ou, champ, ", ".join(manquants))

    def visiter(cle, f, dans_quantif):
        ou = "condition (%s)" % cle
        if cle in ("existe", "tous"):
            if not dans_quantif:
                pointeur_racine(ou, f.get("champ", ""))
        elif cle == "argument":
            if not dans_quantif:
                pointeur_racine(ou, f.get("champ", ""))
            if "valeur" in f:
                v.valeur(ou, f["valeur"])
        elif cle == "session.a_consulte":
            sv = f.get("serveur") or (f.get("outil") or "").split("/")[0]
            if sv and not source_designee(sv, source):
                v.err("%s : la source %s n'est pas désignée par le juriste (ni son nom ni ses mots dans le texte "
                      "ou les réponses) ; ne pas ajouter de condition", ou, sv)
            if f.get("critere") == "transmis" and not transmis_ecrit(source):
                v.err("%s : critere transmis (un appel parti suffit, même sans réponse) n'est pas écrit par le juriste ; "
                      "« consulter » s'écrit donnees_recues", ou)
            if f.get("etiquette"):
                v.nom_ferme("etiquette", f["etiquette"], vocab.etiquettes)
        elif cle in ("appelant", "sous_agent_declare"):
            for x in [f.get("egal"), f.get("prefixe")] + list(f.get("dans") or []):
                if x and not nom_ecrit(x, source) and _normaliser(x) not in _normaliser(source):
                    v.err("%s : %r n'est pas écrit par le juriste", ou, x)
        elif cle == "demandeur":
            if f.get("groupe"):
                v.nom_ferme("groupe", f["groupe"], vocab.groupes)
            ident = f.get("identite") or {}
            for x in ([ident["egal"]] if ident.get("egal") else []) + list(ident.get("dans") or []):
                if _normaliser(x) not in _normaliser(source):
                    v.err("%s : l'identité %r n'est pas écrite par le juriste", ou, x)
        elif cle == "sujet":
            v.nom_ferme("groupe", f.get("groupe", ""), vocab.groupes)
            if not dans_quantif and f.get("champ"):
                pointeur_racine(ou, f["champ"])
        elif cle == "temps":
            fz = f.get("fuseau", "")
            if fz not in vocab.fuseaux:
                v.err("%s : fuseau %r hors de la liste admise", ou, fz)
            elif not fuseau_ecrit(fz, source):
                v.err("%s : le fuseau %s n'est pas écrit par le juriste ; pose une question (sujet fuseau)", ou, fz)
            ecrits = jours_ecrits(source)
            for j in f.get("jour_semaine") or []:
                if j not in ecrits:
                    v.err("%s : le jour %s n'est pas écrit par le juriste", ou, j)
            h = f.get("heure")
            if h:
                ecrites = heures_ecrites(source)
                for x in (h.get("de", ""), h.get("a", "")):
                    if x not in ecrites:
                        v.err("%s : l'heure %s n'est pas écrite par le juriste", ou, x)
            dt = f.get("date")
            if dt and dt.get("valeur") not in dates(source):
                v.err("%s : la date %s n'est pas écrite par le juriste", ou, dt.get("valeur"))
        elif cle == "etiquette_cible":
            v.nom_ferme("etiquette", f.get("contient", ""), vocab.etiquettes)
        elif cle == "metadonnee_reponse":
            if f.get("etiquette"):
                v.nom_ferme("etiquette", f["etiquette"], vocab.etiquettes)
            if f.get("champ") == "source.date":
                val = f.get("valeur")
                if not isinstance(val, str) or val[:10] not in dates(source):
                    v.err("%s : la date %r n'est pas écrite par le juriste", ou, val)
            elif "valeur" in f:
                v.valeur(ou, f["valeur"])

    arbre_mod.parcourir(d.get("condition"), visiter)

    for i, e in enumerate(d.get("effets") or []):
        ou = "effets[%d] (%s)" % (i, e.get("effet"))
        for cle in ("par_role", "destinataire_role"):
            if e.get(cle):
                _role(v, ou, e[cle])
        if e.get("quatre_yeux") and not quatre_yeux_ecrit(source):
            v.err("%s : quatre_yeux n'est demandé que si le juriste écrit « quatre yeux » ou deux personnes", ou)
        if e.get("effet") == "marquer" and e.get("etiquette"):
            v.nom_ferme("etiquette", e["etiquette"], vocab.etiquettes)
        admis = set()
        for so in vises:
            admis |= vocab.arguments_de(*so)
        for a in e.get("champs_autorises") or []:
            if admis and a not in admis:
                v.err("%s : l'argument %s n'existe pour aucun outil visé (%s)", ou, a, ", ".join(sorted(admis)))

    for i, f in enumerate(d.get("fondements") or []):
        for cle in ("texte", "article"):
            x = f.get(cle)
            if x and _normaliser(x) not in _normaliser(source):
                v.err("fondements[%d].%s : %r n'est pas écrit par le juriste", i, cle, x)
        if f.get("texte") and not f.get("article"):
            omis = article_ecrit_apres(f["texte"], source)
            if omis:
                v.err("fondements[%d] : le juriste cite l'article %s de %s ; recopie-le dans article", i, omis, f["texte"])
    rev = p.get("revision_avant")
    if rev and rev not in dates(source):
        v.err("revision_avant : la date %s n'est pas écrite par le juriste ; pose une question", rev)
    return v.erreurs


def _role(v, ou, nom):
    connus = v.vocab.roles
    if nom not in connus:
        v.err("%s : rôle %r inconnu (%s)", ou, nom, ", ".join(connus) or "aucun")
    elif not nom_ferme_ecrit(nom, v.source):
        v.err("%s : le rôle %r n'est ni écrit ni choisi par le juriste ; pose une question en proposant les rôles connus",
              ou, nom)
