# Rapport de nuit — Session H « opération » (fusion-acquisition ATLAS)

Branche : `feat/operation` (worktree). Engine **non modifié**. Tout est **fictif**.

## Compteur Mistral

- Budget brief : 150 ; plafond des règles communes : **40 appels réels** (les règles
  communes priment, §« Coûts »). **Décision prise seul** : je retiens le plafond le plus
  prudent, **40**, et je compte chaque appel réel ci-dessous.
- Appels réels effectués : **0** à ce stade.

## 1. Ce qui est fait

- **Quatre serveurs gouvernables créés par le kit** (`scripts/nouveau-serveur.sh`, hors ligne) :
  - `data-room` : `lister_documents`, `lire_document` (étiqueté `information-privilegiee`,
    `projet-atlas`), `partager_document` (destination `interne`/`externe`, étiqueté
    `transfert_donnees`). 11 documents fictifs (10 ATLAS + 2 publics).
  - `messagerie` : `rediger_message` (lecture de gabarits), `envoyer_message`
    (destination `interne`/`externe`, destinataire, objet, corps, pièces).
  - `bourse` : `consulter_cours`, `passer_ordre` (titre, sens, quantité, pour le compte de) ;
    titres fictifs VENTOUX, BIOLYS, ALPHA, OMEGA.
  - `communication` : `preparer_communique`, `publier_communique` (canal).
  - `go vet ./...` et `go test ./test/extension/` + tests des 4 serveurs **verts**.

## 2. Ce qui n'est pas fait, et pourquoi

## 3. Décisions prises seul (avec justification)

- **Plafond Mistral 40** (voir ci-dessus) : les règles communes priment sur le brief.
- (suite ci-dessous au fil de l'eau)

## 4. Blocages rencontrés

## 5. Questions pour Manu (par urgence)

## 6. Vérifier en 5 minutes

```sh
# depuis la racine du worktree
go vet ./...
go test ./test/extension/        # le kit : serveurs, sous-agents, directive, scénarios (hors ligne)
go run ./cmd/directive verifier < demo/scenarios/jeux/operation.json
```
