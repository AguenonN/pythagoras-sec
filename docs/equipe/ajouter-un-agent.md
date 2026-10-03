# Ajouter un sous-agent

> Recette 2 de [`docs/demo/etendre.md`](../demo/etendre.md), la référence (testée par `test/extension/`). Ce document en détaille les champs.

Un sous-agent = **un fichier JSON**, aucune ligne de code. L'orchestrateur découvre les sous-agents dans `agents/orchestrateur/sous_agents/` au lancement de chaque demande et liste leurs rôles dans sa consigne.

## 1. Écrire le fichier

`agents/orchestrateur/sous_agents/<nom>.json` (le nom du fichier = le champ `nom` ; minuscules, chiffres, tirets) :

```json
{
  "nom": "veille",
  "role": "cherche un nom dans le registre des gels des avoirs.",
  "consigne": "Tu es le sous-agent veille. Cherche le nom indiqué avec gels-avoirs__verifier_gel. N'invente aucun résultat.",
  "outils": ["gels-avoirs__verifier_gel"],
  "schema_sortie": {
    "type": "object",
    "properties": {"nom": {"type": "string"}, "gel": {"type": "boolean"}, "detail": {"type": "string"}},
    "required": ["nom", "gel", "detail"]
  }
}
```

- `role` (facultatif) : une ligne, lue par l'orchestrateur pour choisir à qui déléguer. Absent : la première phrase de la consigne.
- `consigne` : ce que le sous-agent doit faire. C'est une **demande au modèle**, pas une règle : une règle s'écrit en directive (le juriste), l'engine l'applique.
- `outils` : noms exposés par le gate, `serveur__outil`, ou `serveur__*` pour tous les outils d'un serveur. Le sous-agent ne voit que ceux-là.
- `schema_sortie` : la forme de sa réponse finale (objet JSON). Mots-clés admis : `type`, `properties`, `required`, `items`, `enum`, `additionalProperties` (booléen), `description`. Une sortie non conforme est rejetée après une relance.

Le code pose `_meta.sous_agent` = `nom` sur chaque appel ; le modèle ne peut pas le changer.

## 2. Vérifier

```sh
scripts/lancer-demo.sh                          # si un serveur a été ajouté (nouveau-serveur.sh le fait déjà) : régénère demo/registre.json
(cd agents && python3 -m orchestrateur sous-agents)
```

Chaque sous-agent est listé avec ses outils. Une erreur nomme le fichier et la cause (outil absent du registre, champ inconnu, schéma non admis…) ; rien ne se lance tant qu'un fichier est invalide.

## 3. L'utiliser

Rien d'autre à faire : la prochaine demande (`scripts/lancer-demo.sh demande …` ou `scenario …`) le propose à l'orchestrateur. Pour le retirer, supprimer le fichier.
