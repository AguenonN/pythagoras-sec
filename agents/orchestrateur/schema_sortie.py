"""Schéma de la sortie structurée d'un sous-agent : un sous-ensemble fermé de
JSON Schema, vérifié au chargement, puis appliqué à chaque sortie.

Mots-clés admis : ``type`` (object, array, string, number, integer,
boolean, null, ou une liste de ces types), ``properties``, ``required``,
``items``, ``enum``, ``additionalProperties`` (booléen), ``description``.
Tout autre mot-clé est refusé au chargement : un schéma que le code ne
saurait pas appliquer aurait l'air de contraindre sans rien contraindre.
"""

import json

TYPES = {"object", "array", "string", "number", "integer", "boolean", "null"}
MOTS_CLES = {"type", "properties", "required", "items", "enum", "additionalProperties", "description"}


class SchemaInvalide(Exception):
    pass


def verifier_schema(schema, chemin="schema_sortie"):
    """Lève SchemaInvalide, avec le chemin du nœud fautif, si le schéma sort
    du sous-ensemble admis. La racine doit être un objet."""
    erreurs = []
    _verifier(schema, chemin, erreurs)
    if not erreurs and schema.get("type") != "object":
        erreurs.append("%s : la racine doit être de type object" % chemin)
    if erreurs:
        raise SchemaInvalide("\n".join(erreurs))


def _verifier(s, chemin, erreurs):
    if not isinstance(s, dict):
        erreurs.append("%s : un schéma est un objet JSON" % chemin)
        return
    for cle in sorted(s):
        if cle not in MOTS_CLES:
            erreurs.append("%s : mot-clé non admis « %s » (admis : %s)" % (chemin, cle, ", ".join(sorted(MOTS_CLES))))
    if "type" not in s and "enum" not in s:
        erreurs.append("%s : « type » ou « enum » obligatoire" % chemin)
    types = s.get("type")
    if types is not None:
        liste = types if isinstance(types, list) else [types]
        if not liste or any(t not in TYPES for t in liste):
            erreurs.append("%s.type : %r hors de %s" % (chemin, types, ", ".join(sorted(TYPES))))
    if "enum" in s and (not isinstance(s["enum"], list) or not s["enum"]):
        erreurs.append("%s.enum : liste non vide attendue" % chemin)
    if "description" in s and not isinstance(s["description"], str):
        erreurs.append("%s.description : texte attendu" % chemin)
    if "additionalProperties" in s and not isinstance(s["additionalProperties"], bool):
        erreurs.append("%s.additionalProperties : booléen attendu" % chemin)
    props = s.get("properties")
    if props is not None:
        if not isinstance(props, dict):
            erreurs.append("%s.properties : objet attendu" % chemin)
        else:
            for nom in sorted(props):
                _verifier(props[nom], "%s.properties.%s" % (chemin, nom), erreurs)
    req = s.get("required")
    if req is not None:
        if not isinstance(req, list) or not all(isinstance(r, str) for r in req):
            erreurs.append("%s.required : liste de noms attendue" % chemin)
        elif isinstance(props, dict):
            for r in req:
                if r not in props:
                    erreurs.append("%s.required : « %s » absent de properties" % (chemin, r))
    if "items" in s:
        _verifier(s["items"], chemin + ".items", erreurs)


def _type_de(v):
    if v is None:
        return "null"
    if isinstance(v, bool):
        return "boolean"
    if isinstance(v, int):
        return "integer"
    if isinstance(v, float):
        return "number"
    if isinstance(v, str):
        return "string"
    if isinstance(v, list):
        return "array"
    if isinstance(v, dict):
        return "object"
    return "?"


def valider(valeur, schema, chemin="$"):
    """Rend la liste des écarts (vide si conforme)."""
    erreurs = []
    _valider(valeur, schema, chemin, erreurs)
    return erreurs


def _valider(v, s, chemin, erreurs):
    types = s.get("type")
    if types is not None:
        liste = types if isinstance(types, list) else [types]
        t = _type_de(v)
        if not (t in liste or (t == "integer" and "number" in liste)):
            erreurs.append("%s : type %s, attendu %s" % (chemin, t, " ou ".join(liste)))
            return
    if "enum" in s and v not in s["enum"]:
        erreurs.append("%s : valeur hors de %s" % (chemin, json.dumps(s["enum"], ensure_ascii=False)))
    if isinstance(v, dict):
        props = s.get("properties") or {}
        for r in s.get("required") or []:
            if r not in v:
                erreurs.append("%s : champ obligatoire « %s » absent" % (chemin, r))
        for k in sorted(v):
            if k in props:
                _valider(v[k], props[k], "%s.%s" % (chemin, k), erreurs)
            elif s.get("additionalProperties") is False:
                erreurs.append("%s : champ non prévu « %s »" % (chemin, k))
    if isinstance(v, list) and "items" in s:
        for i, x in enumerate(v):
            _valider(x, s["items"], "%s[%d]" % (chemin, i), erreurs)


def lire_sortie(texte):
    """Extrait l'objet JSON de la réponse finale d'un sous-agent : le texte
    entier, ou le contenu d'un unique bloc ```json. Rend (objet, erreur)."""
    t = (texte or "").strip()
    if t.startswith("```"):
        lignes = t.split("\n")
        if len(lignes) >= 2 and lignes[-1].strip() == "```":
            t = "\n".join(lignes[1:-1]).strip()
    try:
        obj = json.loads(t)
    except ValueError:
        return None, "la réponse finale n'est pas un objet JSON"
    if not isinstance(obj, dict):
        return None, "la réponse finale doit être un objet JSON"
    return obj, None
