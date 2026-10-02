# Ajouter un serveur MCP

## 1. Créer le squelette

```sh
scripts/nouveau-serveur.sh <nom>        # minuscules, chiffres, tirets ; pas de « __ »
```

Crée `servers/<nom>/` sur le modèle de `banque-synthetique` : données **fictives** embarquées (`donnees_fictives.json`), un outil de lecture `lire_fiche`, un test, et `schema.json` (le `tools/list` du binaire), puis lance le test. Remplacer les données et les outils ; jamais de donnée réelle d'une personne.

Après toute modification des outils, régénérer le schéma et relancer le test :

```sh
go build -o /tmp/<nom> ./servers/<nom> && go run ./servers/exporter-schema -o servers/<nom>/schema.json /tmp/<nom>
go test ./servers/<nom>/
```

Un outil de lecture porte `ReadOnlyHint: true` (accès `read` au registre) ; sans cette annotation, l'outil est classé `action` (fermé par défaut).

## 2. Le brancher au registre de démo

Trois fichiers **hors du périmètre de la session démo** (à faire par leur responsable, ou à signaler) :

1. `scripts/construire-serveurs.sh` : ajouter `<nom>` à la liste `serveurs` (c'est l'ordre du manifeste).
2. `internal/registredemo/registredemo.go` : ajouter `"<nom>"` à `Serveurs` (et adapter le test qui attend huit serveurs).
3. `demo/etiquettes.json` (facultatif) : étiquettes du serveur ou de ses outils (`donnees_client`, `donnees_personnelles`, `production_rapport`, `externe`…), que les directives peuvent viser.

## 3. Construire, enregistrer, vérifier

```sh
scripts/lancer-demo.sh      # construit tout, écrit bin/manifeste.json et demo/registre.json, vérifie que le gate démarre
```

La ligne `gate prêt … N serveur(s) attesté(s) sur N` doit compter le nouveau serveur. Ses outils sont alors exposés à l'agent sous `<nom>__<outil>` ; un sous-agent peut les recevoir (`docs/equipe/ajouter-un-agent.md`).
