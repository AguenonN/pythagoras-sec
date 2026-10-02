# Ajouter un scénario

Un scénario = **un fichier** `demo/scenarios/<nom>.json`, joué par `scripts/lancer-demo.sh scenario <nom>`.

## 1. Écrire le fichier

```json
{
  "titre": "Nominal : attribution en attente de l'analyste",
  "demande": "Peut-on accorder le découvert demandé dans le dossier DOS-FICTIF-002 ?",
  "demandeur": "camille.conseiller@exemple.test",
  "validations": [
    {"valideurs": [{"identite": "claire.analyste@exemple.test", "role": "analyste-risques"}], "attendu": "acceptee"}
  ],
  "attendu": {"statut": "termine", "directives": ["D-002"], "description": "…"}
}
```

- `demande`, `demandeur` (une identité de `demo/annuaire-source.json`) : obligatoires.
- Facultatifs : `sans_directive` (« D-00n » : même jeu sans cette directive), `jeu` (chemin d'un jeu de travail, par exemple `demo/scenarios/jeux/avec-d009.json`), `registre` (`normal` ou `attaque`).
- `validations` : ce que l'opérateur devra déposer à chaque attente (affiché au lancement).
- `attendu` : `statut` (`termine`, `bloque`, `en_attente`, `erreur`, ou `null` si le résultat n'est pas prévisible) et `directives` citées (ou `null`) ; comparé à la fin (« attendu : conforme » ou « ÉCART »).

## 2. Le jouer

```sh
export MCP_GATE_CLE_JOURNAL=...           # la même pour toute la démo
scripts/lancer-demo.sh scenario <nom>     # réel : Mistral (budget compté), données officielles
```

À une attente, `demo/execution/sessions/<…>/etat.json` passe `en_attente` avec l'empreinte et les rôles ; chaque valideur dépose son jeton :

```sh
scripts/lancer-demo.sh valider <empreinte> --valideur claire.analyste@exemple.test --role analyste-risques
```

Les réponses de Mistral sont enregistrées dans `demo/scenarios/enregistrements/<nom>/<date>.json`.

## 3. Vérifier et garder un secours

```sh
scripts/lancer-demo.sh verifier demo/execution/sessions/<passage>      # chaîne et rejeu des décisions
scripts/lancer-demo.sh scenario <nom> --mode rejeu                     # sans réseau ni Mistral (dernier enregistrement)
scripts/lancer-demo.sh comparer <passage réel> <passage rejoué>        # « aucun écart »
```

## Répéter à une autre date : `--horloge-depart`

`scripts/lancer-demo.sh scenario <nom> --horloge-depart 2026-10-04T09:00:00+02:00` lance le gate sur une horloge simulée et transmet cette date aux sous-agents (« date de la décision »).

**Limite connue** : le gate vérifie le jeton d'identité du demandeur (valable 8 h) et les jetons de validation (15 min au plus) à l'horloge simulée, alors que `mcp-approve` les signe à l'heure réelle. À plus de quelques heures de l'heure réelle, le gate refuse de démarrer (« jeton du demandeur refusé : hors de sa période de validité »). Correction confiée à la session D (date d'émission dans `mcp-approve`). Le jour J, ne pas utiliser l'option : horloge réelle.
