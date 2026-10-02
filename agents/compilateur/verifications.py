"""Lecture de ce que le juriste a écrit : montants, durées, dates, valeurs.

Le chargeur Go garantit qu'une directive est bien formée ; il ne peut pas
savoir si un montant, une date ou un fondement vient du juriste. Ces
fonctions retrouvent dans le texte du juriste et ses réponses les valeurs
écrites ; verifications_v2 y compare chaque valeur proposée.
"""

import datetime
import re
import unicodedata

MOIS = {
    "janvier": 1, "fevrier": 2, "mars": 3, "avril": 4, "mai": 5, "juin": 6,
    "juillet": 7, "aout": 8, "septembre": 9, "octobre": 10, "novembre": 11, "decembre": 12,
}

# Un montant : un nombre en notation française, un multiplicateur facultatif,
# puis une unité obligatoire (€, euro(s), %). « 500 000 » seul ne justifie
# rien, pas plus qu'un numéro d'article.
_MONTANT = re.compile(
    r"(?<![\w,.])(\d{1,3}(?:[ \u00a0\u202f.]\d{3})+(?:,\d+)?|\d+(?:[.,]\d+)?)"
    r"\s*(?:(k|m|md|mds|mille|millions?|milliards?)(?![a-z])\s*)?"
    r"(?:(?:d['’]|de\s)\s*)?"
    r"(€|euros?(?![a-z])|%)",
    re.IGNORECASE,
)
# Ce qui, juste avant un nombre, en fait une référence et non une quantité :
# « art. 22 », « article 1240 », « L561-10 », « R. 123-4 ».
_REFERENCE = re.compile(r"(?:\b[Aa]rt(?:icle)?\.?|\b[LR]\.?)\s*[\d.\-]*\s*$")
# Une durée : un nombre suivi de jour(s), mois, an(s) ou année(s). La valeur
# est celle écrite, sans conversion (« 24 mois » justifie 24).
_DUREE = re.compile(
    r"(?<![\w,.])(\d+(?:,\d+)?)\s*(jours?|mois|ans|an|ann[ée]es?)(?![a-zà-ÿ])",
    re.IGNORECASE,
)
_DATE_ISO = re.compile(r"^(\d{4}-\d{2}-\d{2})(?:T[\d:.]+(?:Z|[+-]\d{2}:\d{2})?)?$")
_MULTIPLICATEURS = {"k": 1e3, "mille": 1e3, "m": 1e6, "million": 1e6, "millions": 1e6,
                    "md": 1e9, "mds": 1e9, "milliard": 1e9, "milliards": 1e9}


def sans_accents(s):
    return "".join(c for c in unicodedata.normalize("NFD", s) if unicodedata.category(c) != "Mn")


def _normaliser(s):
    return re.sub(r"\s+", " ", sans_accents(s).lower()).strip()


def montants(texte):
    """Les montants écrits dans le texte, multiplicateur appliqué
    (« 1,5 million d'euros » donne 1 500 000, pas 1,5). Un nombre sans
    unité (€, euro(s), %) ou précédé de art., article, L ou R n'en est pas un."""
    out = set()
    for m in _MONTANT.finditer(texte):
        if _REFERENCE.search(texte[:m.start()]):
            continue
        brut = re.sub(r"[ \u00a0\u202f]", "", m.group(1))
        if re.fullmatch(r"\d{1,3}(?:\.\d{3})+", brut):
            brut = brut.replace(".", "")
        brut = brut.replace(",", ".")
        try:
            v = float(brut)
        except ValueError:
            continue
        if m.group(2):
            v *= _MULTIPLICATEURS[m.group(2).lower()]
        out.add(v)
    return out


def durees(texte):
    """Les durées écrites dans le texte (jours, mois, ans), valeur telle
    qu'écrite. Un nombre précédé de art., article, L ou R n'en est pas une."""
    out = set()
    for m in _DUREE.finditer(texte):
        if _REFERENCE.search(texte[:m.start()]):
            continue
        out.add(float(m.group(1).replace(",", ".")))
    return out


def quantites(texte):
    """Ce qu'une valeur numérique peut invoquer : montants et durées. Un
    nombre sans unité ne justifie rien."""
    return montants(texte) | durees(texte)


def date_iso(v):
    """AAAA-MM-JJ si ``v`` est une date ISO valide (heure facultative), sinon None."""
    m = _DATE_ISO.match(v)
    if not m:
        return None
    try:
        datetime.date.fromisoformat(m.group(1))
    except ValueError:
        return None
    return m.group(1)


def dates(texte):
    """Toutes les dates écrites dans le texte, en AAAA-MM-JJ."""
    t = sans_accents(texte).lower()
    out = set()
    for j, m, a in re.findall(r"\b(\d{1,2})[/.-](\d{1,2})[/.-](\d{4})\b", t):
        out.add("%s-%02d-%02d" % (a, int(m), int(j)))
    for a, m, j in re.findall(r"\b(\d{4})-(\d{1,2})-(\d{1,2})\b", t):
        out.add("%s-%02d-%02d" % (a, int(m), int(j)))
    for j, mois, a in re.findall(r"\b(\d{1,2})(?:er)?\s+(" + "|".join(MOIS) + r")\s+(\d{4})\b", t):
        out.add("%s-%02d-%02d" % (a, MOIS[mois], int(j)))
    return out


def _nombre(v):
    if isinstance(v, bool):
        return None
    if isinstance(v, (int, float)):
        return float(v)
    return None


def valeur_ecrite(v, source):
    """La valeur d'une condition figure-t-elle dans ce que le juriste a écrit ?"""
    if isinstance(v, list):
        return all(valeur_ecrite(e, source) for e in v)
    n = _nombre(v)
    if n is not None:
        return n in quantites(source)
    if isinstance(v, str):
        d = date_iso(v)
        if d is not None:
            return d in dates(source)  # date écrite explicitement, quel qu'en soit le format
        return _normaliser(v) in _normaliser(source)
    if isinstance(v, bool):
        return True  # vrai / faux : porté par l'opérateur, pas une quantité
    return False
