# 02 — L'exécuteur de descripteur

**Démarre seulement une fois le chantier 01 fusionné dans `main`.** Tu dépends
de `internal/descripteur`, `internal/effet` et `internal/perimetre`.

Lire `nuit/00_regles_communes.md` avant de commencer.

---

## Contexte

Un modèle produit un **descripteur** : de la donnée décrivant une suite
d'appels d'API, leurs liaisons et leur classe d'effet. Rien n'est généré en
code, rien n'est compilé.

Ton chantier écrit **la seule chose qui s'exécute** : le programme qui lit un
descripteur et le déroule. Un seul exécuteur pour tous les connecteurs, écrit à
la main, audité une fois. Quelques centaines de lignes — s'il en fait beaucoup
plus, c'est que de la logique propre à un service y est entrée, et c'est une
erreur.

## Worktree et branche

```
git worktree add ../pythagore-executeur -b feat/executeur
```

Réseau : registres de paquets Go, et `localhost` pour tes serveurs de test.
Aucun appel vers une API réelle. Aucun secret.

## Conception déjà tranchée

- Paquet `internal/executeur`.
- Il lit un descripteur, des paramètres (`EInstance`) et un état de session. Il
  **ne lit jamais** la spec OpenAPI depuis le réseau : elle lui est fournie.
- La boucle :

```
pour chaque étape, dans l'ordre :
    résoudre `operation` dans la spec → méthode, chemin, schéma
    construire la requête : gabarit + liaisons accumulées
    classer l'effet                          ← chantier 03, interface seulement
    décider                                  ← chantier 04, interface seulement
    si autorisé : transmettre via le transport
    extraire les liaisons de la réponse
    écrire l'entrée de journal
```

- **Tu ne codes ni la classification, ni la décision.** Tu déclares deux
  interfaces étroites et tu travailles contre des implémentations de test. Les
  chantiers 03 et 04 les remplissent.
- Le transport est une interface lui aussi : l'exécuteur ne fait pas d'HTTP
  directement, il passe par un transport qu'on lui donne. C'est ce qui
  permettra d'y brancher le proxy d'egress plus tard, et c'est ce qui rend tes
  tests possibles sans réseau.
- **Pas de boucle de convergence.** Une étape qui échoue ne se rejoue pas pour
  atteindre un état voulu. La reprise suit la section 6.3 des règles communes :
  `max = 0` par défaut, interdite sur une étape non réversible.

## Ordre de travail — une étape, un commit

### 1. Les interfaces

```go
type Classifieur interface { Classer(e Effet) effet.Classe }
type Decideur    interface { Decider(e Effet, c effet.Classe) Issue }
type Transport   interface { Envoyer(ctx context.Context, r Requete) (Reponse, error) }
```

Plus une implémentation de test pour chacune. Les signatures exactes sont à
toi, mais elles ne doivent exposer **que** ce dont l'exécuteur a besoin.

### 2. La résolution d'opération

Charger une spec OpenAPI depuis un fichier, et résoudre un `operation` vers
méthode, chemin et schéma attendu. Un `operation` absent de la spec est une
erreur nommée, pas un `panic`.

Tests : opération présente, absente, spec malformée.

### 3. La construction de requête

Gabarits `{{ .nom }}` résolus depuis les paramètres et les liaisons déjà
accumulées. Une variable non résolue est une erreur, jamais une chaîne vide —
une substitution silencieuse par du vide est un envoi de requête fausse.

Tests : substitution simple, variable manquante, variable issue d'une étape
précédente.

### 4. L'extraction des liaisons

Le champ `lie` d'une étape `appel`, appliqué à la réponse. Chemin absent ⇒
erreur nommée. Une liaison ne peut pas écraser un paramètre d'entrée.

Tests : extraction simple, chemin absent, tentative d'écrasement.

### 5. Le branchement

L'étape `choix` : évaluer `sur`, chercher dans `cas`, sauter à l'étape nommée.
Une valeur hors de l'ensemble fermé est un refus d'exécution, pas un défaut
silencieux. `_fin` termine.

Tests : chaque branche, valeur hors ensemble, saut vers un `id` inexistant.

### 6. La boucle complète

Assembler. Journaliser chaque effet via une interface `Journal` que tu déclares
et que le chantier 04 remplira.

Test de bout en bout : un descripteur écrit à la main, un serveur HTTP de test
(`internal/backendtest` fournit déjà de quoi en monter un), et la vérification
que les requêtes partent dans le bon ordre avec les bonnes valeurs.

### 7. La reprise

`reprise.max` respecté, et refusée sur `reversible: false` même si le
descripteur le demandait — le chargeur l'a déjà refusé, mais l'exécuteur ne
fait pas confiance à son entrée.

Test : une étape réversible qui échoue est reprise jusqu'à `max` ; une étape
non réversible qui échoue ne l'est jamais.

### 8. Rapport final

## Critère de réussite

```bash
go test ./internal/executeur/... -count=1 -race
```

Un descripteur écrit à la main s'exécute de bout en bout contre un serveur de
test, les cinq requêtes partent dans l'ordre avec les valeurs attendues, et
aucune reprise ne se produit sur l'étape non réversible.

## En cas de blocage

- Si la résolution OpenAPI te demande une bibliothèque : prends-en une, note-la
  dans le rapport, et attends-toi à un conflit bénin sur `go.mod` à la fusion.
- Si un cas de gabarit devient tordu après trois tentatives : refuse-le au
  chargement, note-le, continue.
- **Ne code pas la classification ni la décision**, même si ça paraît plus
  simple que de passer par l'interface. C'est le travail de deux autres
  sessions, et deux implémentations concurrentes rendront la fusion pénible.
