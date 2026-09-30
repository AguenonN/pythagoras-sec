"""Conversion des outils MCP (inputSchema) en définitions de fonctions Mistral.

Mistral ne documente pas précisément le sous-ensemble de JSON Schema qu'il
accepte dans ``parameters``. On s'en tient donc à un noyau sûr et l'on
signale tout le reste, jamais en silence :

- ``ACCEPTES`` : conservés tels quels ;
- ``RETIRES`` : contraintes et annotations retirées. L'information est
  perdue pour le modèle, mais le serveur valide toujours ses arguments :
  retirer ne relâche rien côté serveur ;
- ``const`` : réécrit en ``enum`` à une valeur (même sens) ;
- ``additionalProperties`` booléen : conservé ; sous forme de schéma, retiré
  (forme non sondée) ;
- ``type`` en liste : ``[T, "null"]`` (dans un ordre ou l'autre) est
  conservé ; toute autre liste exclut l'outil ;
- ``STRUCTURANTS`` (composition, références, conditionnels) : les retirer
  changerait la forme des arguments ; l'outil entier est exclu et signalé.

Un mot-clé inconnu est traité comme structurant : dans le doute, on exclut.

Ce qui est conservé au-delà du noyau l'est parce que l'API l'a accepté,
sondé par ``python -m boucle.sondes`` (verdicts dans ``sondes.json``) sur la
forme exacte qu'ont les schémas épinglés : ``additionalProperties: false``
et ``type: ["null", "array"]``.
"""

import re

ACCEPTES = {"type", "properties", "required", "items", "enum", "description"}
RETIRES = {
    "$schema", "$id", "$comment", "title", "default", "examples", "format", "pattern",
    "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf",
    "minLength", "maxLength", "minItems", "maxItems", "uniqueItems",
    "minProperties", "maxProperties",
    "readOnly", "writeOnly", "deprecated", "contentEncoding", "contentMediaType",
}
STRUCTURANTS = {
    "anyOf", "oneOf", "allOf", "not", "if", "then", "else", "$ref", "$defs", "definitions",
    "patternProperties", "propertyNames", "dependentSchemas", "dependentRequired",
    "dependencies", "prefixItems", "contains", "minContains", "maxContains",
    "unevaluatedProperties", "unevaluatedItems", "$anchor", "$dynamicRef", "$dynamicAnchor",
}

NOM_FONCTION = re.compile(r"^[a-zA-Z0-9_-]{1,64}$")


class Exclu(Exception):
    """L'outil ne peut pas être offert au modèle sans en changer le sens."""


def _convertir(schema, chemin, signalements, sous_schema=False):
    if not isinstance(schema, dict):
        raise Exclu("%s : schéma non objet" % chemin)
    out = {}
    for cle in sorted(schema):
        valeur = schema[cle]
        ici = chemin + "." + cle
        if cle in ACCEPTES:
            if cle == "properties":
                if not isinstance(valeur, dict):
                    raise Exclu("%s : properties n'est pas un objet" % ici)
                out[cle] = {nom: _convertir(s, ici + "." + nom, signalements, True)
                            for nom, s in valeur.items()}
            elif cle == "items":
                if isinstance(valeur, list):
                    raise Exclu("%s : items en liste (tuple) non accepté" % ici)
                out[cle] = _convertir(valeur, ici, signalements, True)
            elif cle == "type" and isinstance(valeur, list):
                types = [t for t in valeur if t != "null"]
                if len(types) != 1 or len(valeur) != 2:
                    raise Exclu("%s : type multiple %r non accepté" % (ici, valeur))
                out[cle] = list(valeur)
            else:
                out[cle] = valeur
        elif cle == "additionalProperties":
            if isinstance(valeur, bool):
                out[cle] = valeur
            else:
                signalements.append("%s : additionalProperties sous forme de schéma non accepté, retiré" % ici)
        elif cle == "const":
            if "enum" in schema:
                raise Exclu("%s : const et enum ensemble" % ici)
            out["enum"] = [valeur]
            signalements.append("%s : const réécrit en enum à une valeur" % ici)
        elif cle in RETIRES:
            signalements.append("%s : mot-clé %r non accepté, retiré" % (ici, cle))
        elif cle in STRUCTURANTS:
            raise Exclu("%s : mot-clé %r non accepté, l'outil est exclu" % (ici, cle))
        else:
            raise Exclu("%s : mot-clé inconnu %r, l'outil est exclu" % (ici, cle))
    return out


def convertir(outil):
    """Rend (définition Mistral, signalements) ou lève ``Exclu``."""
    nom = outil.get("name", "")
    if not NOM_FONCTION.match(nom):
        raise Exclu("%s : nom de fonction invalide pour Mistral ([a-zA-Z0-9_-], 64 caractères)" % nom)
    signalements = []
    schema = outil.get("inputSchema") or {"type": "object"}
    parametres = _convertir(schema, nom, signalements)
    if parametres.get("type") != "object":
        raise Exclu("%s : inputSchema doit être de type object" % nom)
    parametres.setdefault("properties", {})
    description = outil.get("description") or outil.get("title") or ""
    return {
        "type": "function",
        "function": {"name": nom, "description": description, "parameters": parametres},
    }, signalements


def convertir_tous(outils):
    """Rend (définitions, signalements, exclus) ; l'ordre des outils est gardé."""
    definitions, signalements, exclus = [], [], []
    for o in outils:
        try:
            d, s = convertir(o)
        except Exclu as e:
            exclus.append(str(e))
            continue
        definitions.append(d)
        signalements += s
    return definitions, signalements, exclus
