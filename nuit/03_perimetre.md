# 03 — Périmètre et plancher déterministe

**Démarre seulement une fois le chantier 01 fusionné dans `main`.** Tu dépends
de `internal/effet` et `internal/perimetre`.

Lire `nuit/00_regles_communes.md` avant de commencer.

---

## Contexte

Un connecteur est admis sous un **périmètre**. La part qui entre dans
l'attestation, `EClasse`, est une fonction pure de la demande normalisée. La
part propre au demandeur, `EInstance`, se vérifie mécaniquement à chaque
exécution.

Ton chantier écrit les deux vérifications qui n'ont besoin d'aucun modèle, et
qui tranchent la majorité des cas : **le plancher déterministe** et le contrôle
d'enveloppe. C'est le code qui garantit que le générateur et le classifieur ne
peuvent que durcir, jamais assouplir.

C'est aussi, en pratique, ce qui fait que trois effets sur cinq se décident
sans appeler aucun modèle. Chaque refus que tu rends ici est un aller-retour
réseau économisé.

## Worktree et branche

```
git worktree add ../pythagore-perimetre -b feat/perimetre
```

Réseau : registres de paquets Go. Aucun secret. Ton code est **entièrement
pur** : aucune I/O, aucune horloge.

## Conception déjà tranchée

- Le plancher va dans `internal/controle`, à côté de `Registre` qui existe
  déjà. Même forme : une fonction pure, appelée à l'identique par l'exécuteur
  et par le rejeu, pour que ce qu'affirme le journal se recalcule.
- Le plancher **ne lit jamais** la classe déclarée par le descripteur. Il
  observe la requête et conclut seul. La classe retenue est ensuite
  `effet.Max(déclarée, plancher)`.
- Les règles du plancher sont **déclarées par famille**, pas écrites en dur
  dans le code : une table que l'opérateur peut lire et relire.

## Ordre de travail — une étape, un commit

### 1. Les règles du plancher

Paquet `internal/controle`, fichier `plancher.go`. Le type d'une règle et son
chargement depuis une table déclarative.

Les règles de départ, qui sont un minimum et pas une liste close :

| Observation | Classe plancher |
|---|---|
| méthode ≠ `GET` | au moins `Ecriture` |
| le corps porte un champ `amount`, `price`, `payment`, `card`, `total` | au moins `Transaction` |
| le chemin contient `users`, `account`, `address`, `auth`, `token` | `Administration` |

Ces listes sont des données, pas des constantes dans une fonction.

Tests : chaque règle isolément, puis une requête qui en déclenche deux — la
classe retenue est la plus forte.

### 2. `Plancher`

```go
func Plancher(dec Decision, e Effet, regles []Regle) (Decision, bool)
```

Pure. Rend la décision finale et `true` si c'est elle qui l'a imposée, comme
`controle.Registre`. Une décision déjà `Refuser` reste celle qui l'a refusée —
le motif doit citer qui a tranché.

Tests : classe montée par le plancher, décision déjà refusée laissée
intacte, le motif nomme la règle.

### 3. Domaine hors enveloppe

Un effet vers un domaine absent de `EClasse.Domaines` est un **refus immédiat**,
rendu sans consulter quoi que ce soit d'autre. C'est le chemin le plus court du
système et il doit le rester.

Test : refus, et le motif nomme le domaine.

### 4. `manifeste ⊆ enveloppe`

Le manifeste d'un descripteur est l'ensemble des classes d'effet que ses étapes
peuvent produire. Comme les `choix` portent sur des ensembles fermés, cet
ensemble se calcule **statiquement**, en énumérant les chemins — c'est
précisément ce que les deux types d'état garantissent.

Écrire le calcul, puis la vérification d'inclusion dans le palier de
l'enveloppe.

Tests : descripteur conforme, descripteur dont une branche dépasse le palier,
descripteur avec branchement où seule une branche dépasse.

### 5. Vérification de `EInstance`

Les contrôles mécaniques à l'exécution : montant ≤ plafond, adresse = adresse
de session, demandeur habilité. Chacun rend un motif nommé.

Tests : un par contrôle, et le cas où deux échouent — le motif les cite tous.

### 6. Propriétés

Un test de propriété sur l'ensemble : pour toute requête et toute table de
règles, `classe_retenue >= classe_déclarée`. Le générateur peut sur-déclarer,
jamais sous-déclarer. C'est l'invariant 2 des règles communes, et il doit être
prouvé, pas commenté.

### 7. Rapport final

## Critère de réussite

```bash
go test ./internal/controle/... ./internal/perimetre/... -count=1
```

Le test de propriété passe. Un effet vers un domaine hors enveloppe est refusé
sans qu'aucune autre fonction ne soit appelée. Le manifeste d'un descripteur de
test se calcule statiquement et l'inclusion se vérifie.

## En cas de blocage

- Si le calcul statique du manifeste bute sur un descripteur tordu : refuse-le,
  et note le cas. Un descripteur dont on ne peut pas énumérer les effets est un
  descripteur qu'on ne doit pas exécuter.
- Si une règle de plancher te paraît trop large — beaucoup de faux positifs :
  **la garder quand même** et la noter. Un faux positif coûte une attente
  humaine ; un faux négatif coûte un effet non décidé.
- Ne pas introduire d'appel de modèle, même pour « améliorer » une
  classification. Ce chantier est le témoin déterministe contre lequel on
  mesurera l'apport des classifieurs ; s'il en contient, la mesure ne veut plus
  rien dire.
