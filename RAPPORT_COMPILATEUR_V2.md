# Rapport — session « compilateur-v2 » : du texte du juriste au jeu v2

Branche `feat/compilateur-v2`, worktree `~/cybercar/mcp_rogue-compilateur-v2`.
Rien n'est poussé ni fusionné ; `main` n'est pas touchée ; `internal/directive`
et le gate ne sont pas modifiés.

État : **en cours**.

## 1. Fait

| Étape | Commit | Objet |
|---|---|---|
| 1 | (ce commit) | `internal/relecture` : relecture v2 déterministe (déclencheur, arbre, feuilles, effets, fuseau, indéterminé), fichiers attendus octet pour octet, trois fuseaux machine |

### Étape 1 — relecture v2

- `relecture.Directive` rend une directive v2 par `directiveV2` ; la relecture
  v1 est inchangée octet pour octet (les attendus v1 et `agents/tests/attendus/d001.txt`
  passent sans modification). Un jeu qui contient au moins une directive v2
  porte une ligne de plus sur le cumul des notifications et marques.
- Forme : identité et fondements (le premier est dit « cité par les motifs »),
  validité, périmètre, `Quand :` (déclencheur), `Si …` (arbre indenté, une
  condition par tiret, `et`/`ou` annoncés, `non` en phrase niée quand la
  feuille le permet, quantificateurs avec leur valeur sur liste vide),
  `Alors :` (un effet par tiret), puis la règle de l'indéterminé à la racine.
- Le fuseau d'une feuille `temps` est toujours nommé ; les dates de
  l'enveloppe et de `source.date` sont écrites en UTC ; les dates de feuille
  `temps` sont réécrites sans passer par une heure.
- Tests : 7 jeux (les 6 scénarios v2 de `internal/directive/testdata/v2`, lus
  sans être modifiés, et `testdata/variantes-v2.json`) comparés à
  `testdata/v2/*.txt` ; changement de `time.Local` en cours d'exécution ;
  sous-processus avec `TZ=UTC`, `Asia/Tokyo`, `America/Los_Angeles` ; test de
  couverture (chaque nœud, feuille, opérateur, effet et évènement employé par
  au moins un jeu relu) ; aucun gabarit de repli sur un jeu accepté.

## 2. Pas fait (encore)

Étapes 2 à 7.

## 3. Décisions prises seul

1. **Relecture v1 inchangée** : un jeu purement v1 est relu exactement comme
   avant (d'autres tests et le compilateur v1 en dépendent). La phrase sur le
   cumul n'apparaît que si le jeu contient une directive v2.
2. **Règle de l'indéterminé** écrite seulement si quelque chose peut être
   indéterminé (condition présente, ou cible portant sur `etiquette`/`acces`) ;
   la mention « les notifications et marques s'appliquent aussi » seulement si
   la directive en a.

## 4. Blocages

Aucun.

## 5. Questions pour Manu

(à venir)

## 6. Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-compilateur-v2
go vet ./... && go test -race -count=1 ./internal/relecture ./cmd/directive
go run ./cmd/directive relire < internal/directive/testdata/v2/credit.json
```
