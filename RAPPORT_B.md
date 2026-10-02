# Rapport — session B « compilateur » : fiable sur scène, branché sur la console

Branche `feat/compilateur-final`, worktree `~/cybercar/mcp_rogue-compilateur-final`.
Rien n'est poussé ni fusionné ; `main` n'est pas touchée.

**Compteur d'appels Mistral réels : 0 / 25.**

## 1. Ce qui est fait

| Étape | Commit | Objet |
|---|---|---|
| — | `3e9149c` | `demo/directives/demo.json` et `demo/annuaire-source.json` repris **à l'identique** de `feat/demo-reelle` (`ca3bbdd`), voir décision 1 |
| 1 | (ce commit) | Vocabulaire généré : `go run ./cmd/registre-demo -vocabulaire …` ; tests de divergence Go et Python ; ancien vocabulaire gelé pour les enregistrements existants |

## 2. Ce qui n'est pas fait

(en cours)

## 3. Décisions prises seul

1. **Copie des fichiers de la session A.** `demo/directives/demo.json` n'existait
   pas sur ma branche et l'annuaire n'y portait ni `analyste-risques`, ni
   `responsable-rh`, ni `direction-juridique`. Or le critère de réussite passe
   par `--jeu-existant demo/directives/demo.json`, et chaque requête Mistral
   enregistrée embarque le vocabulaire tiré de l'annuaire : enregistrer sur
   l'ancien annuaire rendrait les enregistrements caducs dès la fusion. J'ai
   donc repris ces deux fichiers **octet pour octet** depuis `feat/demo-reelle`
   (`ca3bbdd`, dernier commit de A). Je ne les modifie pas. Mêmes contenus des
   deux côtés : la fusion ne crée pas de conflit. Si A les change encore, il
   faudra régénérer le vocabulaire et réenregistrer (voir §5).
2. **Groupes du vocabulaire** : ceux de l'annuaire (feuille `demandeur`) **et**
   ceux du référentiel RH (`servers/referentiel-rh/groupes_fictifs.json`,
   feuille `sujet`). Le brief ne cite que l'annuaire, mais sans le référentiel
   RH le compilateur ne pourrait plus nommer `salaries-proteges` ni
   `contentieux-en-cours` (D-006, D-007, scénario RH). La description de
   chaque groupe dit de quelle feuille il relève.
3. **`-vocabulaire` écrit seulement le vocabulaire**, sans lire le manifeste
   des binaires : il ne dépend que de fichiers versionnés, il se régénère
   partout.
4. **Tous les rôles de l'annuaire** entrent au vocabulaire, `emetteur-identite`
   compris : aucun tri à la main.

## 4. Blocages

Aucun pour l'instant.

## 5. Questions pour Manu

(aucune pour l'instant)

## 6. Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-compilateur-final
go vet ./... && go test -race -count=1 ./cmd/registre-demo ./cmd/directive ./internal/relecture
(cd agents && python3 -m unittest discover -s tests -t .)
go run ./cmd/registre-demo -vocabulaire agents/compilateur/exemples/vocabulaire-demo.json && git diff --stat
```
