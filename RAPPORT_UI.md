# RAPPORT_UI — session « ui »

## 1. Fait
- Option de conservation des journaux de test : `MCP_CONSERVER_JOURNAUX=<dir> go test ./test/integration/` copie journal et coffre de chaque scénario dans `<dir>/<scénario>/` (commit 1).

## 2. Pas fait
- Console (en cours).

## 3. Décisions prises seul
- Clé des journaux de test = `cleTest` (chaîne sans valeur, dans le test) ; la console la lit dans `MCP_GATE_CLE_JOURNAL` (hex) comme le gate.

## 4. Blocages
Aucun.

## 5. Questions pour Manu
Aucune.

## 6. Vérification en 5 minutes
À compléter.
