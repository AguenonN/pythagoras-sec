# Contrôles segmentés — design doc

Refonte de l'évaluation : la décision reste une fonction pure, mais elle
reçoit en entrée des **verdicts** produits par un modèle discriminatif
(Jev). Le modèle observe, il ne décide pas.

Ce document couvre P0 à P3 en détail et P4 en vue d'ensemble. Il ne traite
pas la conformité (DORA, souveraineté du modèle) : ce sont des questions de
déploiement, pas de validation de l'idée.

---

## 1. Hypothèse à valider

> Une décision de sécurité peut être rendue par la combinaison
> **déterministe** de verdicts de classification, avec une erreur jointe
> mesurable, sans qu'un modèle puisse jamais assouplir une issue.

Deux conditions de succès, mesurables, qui décident de la suite :

| # | Condition | Nature |
|---|---|---|
| C1 | Rejeu logique exact sur 100 % des appels journalisés portant des verdicts | Invariant, binaire |
| C2 | L'erreur jointe du vecteur de verdicts fait mieux que le plancher déterministe seul, sur un jeu étiqueté incluant des injections | Mesure, avec témoin |

**C2 a un témoin obligatoire : le plancher seul.** Si les contrôles Jev
n'apportent rien par rapport à lui, l'idée ne tient pas, et il vaut mieux
le savoir à P3 qu'après avoir écrit la sandbox de P4.

---

## 2. Ce que l'existant fournit déjà

Rien de ce qui suit n'est à créer. C'est l'ossature sur laquelle la refonte
se branche.

| Brique existante | Rôle dans la refonte |
|---|---|
| `directive.Trivalent` (Kleene forte, `Faux < Indetermine < Vrai`) | État d'un verdict. `Et`/`Ou` sont déjà min/max |
| `directive.plusFort` sur `Autoriser < Caviarder < AttendreValidation < Refuser` | **Monotonie des issues**, déjà en place |
| `SiIndetermine: refuser` par défaut, `ignorer` seulement validé nommément | **Fail-closed sur incertitude**, déjà en place |
| `directive.Decide` pure, vérifiée par `TestPurete_AucuneIONiHorloge` | Reste pure ; les verdicts entrent comme données |
| `internal/controle` (`Registre`) : fonction pure appliquée après Decide, appelée à l'identique par l'engine et le rejeu | **Patron exact du plancher déterministe** |
| `journal.Entree` porte déjà tout ce que Decide a lu | Les verdicts s'y ajoutent au même titre que `EtiquettesCible` ou `Sujets` |
| `rejeu` recalcule la décision dans le cadre historique | Rejeu logique : réinjecte les verdicts, rien d'autre à changer |
| `Appel.Horodatage` (Decide ne lit pas l'horloge) | Inchangé |

### Contrainte vérifiée par test

`internal/directive/purete_test.go` interdit à tout fichier non-test du
paquet d'importer `os`, `net`, `net/http`, `io`, `context`, `regexp`,
`reflect`, `math/rand`, et d'employer `time.Now` et consorts.

**Conséquence non négociable :** le client Jev ne peut pas vivre dans
`internal/directive`. Le type `Verdict` (donnée pure) y vit ; le client HTTP
va dans `internal/jev`. Decide ne connaît que le type.

---

## 3. Invariants

Ce sont les propriétés que le code doit rendre vraies, et que les tests
doivent prouver. Tout le reste est négociable.

### I1 — Decide reste pure

```
Decide(jeu, etat, appel, verdicts) → decision
```

Aucun appel réseau, aucune horloge, aucune lecture hors arguments. Les
verdicts sont une entrée comme l'attestation. `TestPurete` est étendu au
nouveau paramètre : il doit continuer de passer sans exception.

### I2 — Monotonie : un modèle ne peut que durcir

Un verdict ne peut jamais produire une issue plus faible que celle des
règles déterministes. L'issue finale est `plusFort(issue_déterministe,
issue_contrôles)`. Un refus déterministe (serveur non attesté, outil hors
catalogue épinglé, plancher d'effet) n'est pas levable par Jev.

Vérification : test de propriété. Pour tout jeu, tout état, tout appel et
tout vecteur de verdicts, `Decide(…, verdicts).Effet >= Decide(…,
nil).Effet` au sens de `rang()`.

### I3 — Monotonie des classes d'effet

La classe d'un effet retenue est `max(classe_déclarée_au_manifeste,
classe_plancher, classe_Jev)` sur l'ordre
`lecture < panier < écriture < compte < paiement`.

Le générateur de code et le modèle peuvent rendre le moteur **plus strict**
(sur-déclarer), jamais plus laxiste. C'est ce qui rend acceptable que la
table de classification soit produite par la partie non fiable (§6).

### I4 — Fail-closed sur incertitude

Un verdict est `Indetermine` quand : la confiance est sous le seuil de zone
grise, le modèle n'a pas répondu, l'entrée dépasse la taille admise, ou deux
questions contradictoires se contredisent. `SiIndetermine` existant
s'applique sans modification.

### I5 — Couverture totale de la table de décision

Toute combinaison de verdicts non couverte par une ligne de la table
produit `Refuser`. Vérifié **statiquement** à la compilation de la
directive, pas à l'exécution.

### I6 — Admissibilité mesurée

Un contrôle n'est évaluable qu'accompagné d'un jeu d'évaluation étiqueté,
de FPR/FNR mesurés et d'un seuil justifié, versionnés avec la directive. Un
contrôle sans mesure rend `Indetermine` par construction — donc, par I4,
refus.

### I7 — Moindre privilège sur les entrées

Chaque contrôle déclare les champs qu'il lit. Decide ne lui passe que
ceux-là. Deux conséquences : l'empreinte d'entrée est stable (donc
rejouable), et une injection dans un champ n'atteint pas les contrôles qui
ne le lisent pas.

---

## 4. Jev : ce que l'API impose

Source : documentation TypeSafe AI, `jev-1.13.0`.

| Fait | Conséquence de conception |
|---|---|
| Les questions d'une requête sont répondues **en parallèle, sans dépendance possible** entre elles | La combinaison déterministe côté Decide n'est pas un choix, c'est la seule voie. Une dépendance réelle = deux requêtes en séquence |
| Une requête = **un état + N questions** | Un seul aller-retour par lot de contrôles. Latence = max, pas somme |
| La **seule** sortie du modèle est une liste de distributions de probabilité | `choice`, `score`, `confidence` sont des formules fixes appliquées hors modèle |
| Trois types de question : `noul` (V/F), `choice` (liste non ordonnée), `score` (liste ordonnée) | Mapping au §5 |
| Limite 64k jetons par entrée, 32k pour l'état | Borne le moindre privilège ; au-delà → `Indetermine` |

### Décision : on journalise la distribution brute

L'engine **ne lit pas** `choice`, `score` ni `confidence` de la réponse
API. Il journalise la distribution et recalcule tout en Go.

Trois raisons :

1. Le rejeu logique devient indépendant du fournisseur — on ne dépend même
   pas de son post-traitement.
2. Les formules de confiance sont publiques et déterministes : la relecture
   peut les vérifier.
3. Remplacer Jev par un modèle embarqué ne touche que `internal/jev`, pas
   le moteur.

### Limite à écrire noir sur blanc

Jev est propriétaire : pas de poids, pas d'empreinte de poids vérifiable.
On épingle `jev-1.13.0` et l'empreinte du gabarit de question, rien de plus.

Donc **deux modes de rejeu, qui ne promettent pas la même chose** :

| Mode | Garantie | Usage |
|---|---|---|
| Rejeu logique | **Exact.** Verdicts réinjectés depuis le journal → même issue au bit près | Audit, preuve, C1 |
| Rejeu modèle | Aucune. Relance l'inférence et compare | Mesure de dérive. Un écart ne remet pas en cause une décision passée |

Ne jamais présenter le second comme une garantie. C'est un capteur.

### Ce qu'il ne faut pas reprendre du marketing

« Zéro hallucination » et « erreurs de type impossibles » signifient que la
sortie est bien formée sur un ensemble de choix prédéfini. Cela ne dit rien
sur la **justesse** de la réponse ni sur sa résistance à une injection dans
l'état. Les mesures de I6 restent nécessaires et c'est tout ce qu'on
avance.

---

## 5. Schéma : le contrôle

Un contrôle est une unité d'évaluation attachée à une directive.

```jsonc
{
  "id": "effet_est_paiement",
  "type": "noul",                     // noul | choice | score
  "instructions": "La requête déclenche-t-elle un débit ?",
  "criteres": { "true": "...", "false": "..." },
  "champs_entree": ["/corps/amount", "/methode", "/chemin"],  // I7
  "seuil": 0.80,
  "zone_grise": 0.10,                 // |confiance - seuil| < zone_grise → Indetermine
  "modele": "jev-1.13.0",
  "evaluation": {                     // I6 : sans ce bloc, Indetermine
    "jeu": "eval-paiement-v2",
    "fpr": 0.021,
    "fnr": 0.034,
    "n": 240
  }
}
```

Affectation des types :

| Question | Type |
|---|---|
| « est-ce un paiement ? », « l'adresse diffère-t-elle de la session ? » | `noul` |
| classe d'effet observée (lecture / panier / écriture / compte / paiement) | `choice` |
| niveau de risque, cohérence montant/panier | `score` |

### Lot : l'unité de requête

Un lot regroupe les contrôles qui lisent **le même niveau de privilège**.
Un lot = une requête Jev = un état.

Au minimum deux lots par famille d'effets :

- `origine_utilisateur` — champs influençables par l'agent ou par une
  réponse de serveur (nom d'un article, texte libre). Zone d'injection.
- `origine_moteur` — champs produits par l'engine (méthode HTTP, domaine,
  état de session, montant déjà lu). Non influençables.

Séparer les deux est ce qui empêche une injection dans un champ libre
d'atteindre le contrôle « montant ≤ panier ».

### Table de décision

```jsonc
"combinaison": [
  { "motif": { "effet_est_paiement": "vrai",  "adresse_coherente": "faux" }, "issue": "refuser" },
  { "motif": { "effet_est_paiement": "vrai",  "adresse_coherente": "vrai" }, "issue": "attendre_validation" },
  { "motif": { "effet_est_paiement": "*",     "adresse_coherente": "indetermine" }, "issue": "attendre_validation" },
  { "motif": { "effet_est_paiement": "faux",  "adresse_coherente": "vrai" }, "issue": "autoriser" }
]
```

Règles du vérificateur (statique, au chargement) :

- **couverture** : toute combinaison de {vrai, faux, indetermine}ⁿ tombe sur
  au moins une ligne, sinon le jeu est refusé au chargement (I5) ;
- **monotonie** : sur une même ligne, remplacer un état par `indetermine` ne
  peut pas rendre une issue plus faible ;
- **ordre canonique** : la première ligne correspondante gagne, et l'ordre
  est celui du document haché, comme pour les directives.

Pour 5 contrôles : 243 combinaisons, la plupart couvertes par jokers. Le
compilateur génère la table, le vérificateur la prouve, `relecture` la rend
en gabarit. **Un humain valide sans LLM** — c'est la propriété à ne pas
perdre.

---

## 6. Manifeste et plancher

Décision actée : **la table de classification des effets est produite par le
générateur de code dans le manifeste.** C'est la partie non fiable qui
déclare la classe de ses propres effets. Trois mécanismes la tiennent.

### 6.1 Plancher moteur (`internal/controle`)

Classification minimale, indépendante du manifeste, déclarée par famille
dans la directive :

| Observation | Classe plancher |
|---|---|
| méthode ≠ `GET` | ≥ `écriture` |
| corps contient `amount`, `price`, `payment`, `card`, `total` | ≥ `paiement` |
| chemin contient `users`, `account`, `address`, `auth`, `token` | ≥ `compte` |
| domaine hors manifeste | refus immédiat, sans consulter Jev |

C'est le seul évaluateur à biais nul vis-à-vis du modèle, et le **mode
dégradé** : Jev indisponible, le plancher seul tourne (§9, décision à
prendre).

### 6.2 Admission de la table (phase A)

- contrôle Jev : « cette table sous-classe-t-elle un effet ? » ;
- comparaison déterministe à la dernière table admise pour le même
  domaine : toute **régression de classe** (un chemin qui passe de
  `paiement` à `panier`) ⇒ attente humaine.

### 6.3 Mémoire du registre

La première table admise pour un domaine devient la référence. Les
suivantes peuvent **ajouter** des chemins, jamais **rétrograder** la classe
d'un chemin déjà connu. Même mécanisme que `toolpin` sur les schémas.

### 6.4 À l'exécution

Les contrôles Jev de classe **ne lisent pas** la classe déclarée. Si Jev
répond « paiement » là où le manifeste dit « panier », I3 fait remonter la
classe. Le générateur peut durcir, jamais assouplir.

---

## 7. Interfaces Go

### `internal/directive` — types purs uniquement

```go
// Etat d'un verdict : réutilise Trivalent (Kleene forte) tel quel.
// Faux < Indetermine < Vrai ; Et = min, Ou = max.

// Verdict est ce qu'un contrôle a observé. Donnée pure : Decide la lit,
// ne la produit jamais. La distribution est la SEULE sortie du modèle ;
// mode, confiance et état sont recalculés ici.
type Verdict struct {
    ControleID      string
    Distribution    []float64 // journalisée telle quelle
    EmpreinteEtat   string    // sha256 des champs_entree effectivement passés
    EmpreinteQuestion string  // sha256 du gabarit figé
    Modele          string    // "jev-1.13.0" — pas d'empreinte de poids disponible
    Erreur          string    // non vide ⇒ Indetermine
}

func (v Verdict) Mode() int
func (v Verdict) Confiance() float64              // formule publique, recalculée
func (v Verdict) Etat(c Controle) Trivalent       // Vrai | Faux | Indetermine

// Decide gagne un paramètre. Le reste de la signature ne bouge pas.
func Decide(jeu Jeu, etat EtatSession, appel Appel, verdicts []Verdict) Decision
```

`Decide(…, nil)` doit rendre exactement ce que l'actuel rend : la refonte
n'a pas le droit de changer une décision existante. C'est un test de
non-régression sur `scenarios_v2_test.go`.

### `internal/jev` — tout l'impur

```go
// Classifier est l'unique porte vers le modèle. Interface étroite :
// changer de fournisseur ne touche rien d'autre.
type Classifier interface {
    Classer(ctx context.Context, lot Lot) ([]directive.Verdict, error)
}

type Lot struct {
    Etat      map[string]any      // champs du niveau de privilège, et rien d'autre
    Controles []directive.Controle
}

// Fixtures : implémentation déterministe pour tester Decide sans réseau.
type Fixtures map[string][]float64
```

Un timeout, un refus ou une réponse malformée produit des verdicts
`Erreur` — jamais une erreur remontée qui ferait tomber l'appel dans un
chemin non journalisé.

### `internal/controle` — le plancher

```go
// Plancher est pure, appelée à l'identique par l'engine et le rejeu,
// exactement comme Registre.
func Plancher(dec directive.Decision, effet Effet, regles []RegleP) (directive.Decision, bool)
```

---

## 8. Journal et rejeu

### Entrée `appel`, champs ajoutés

```go
// Verdicts consignés pour le rejeu : la distribution brute, l'empreinte
// de l'état réellement passé au modèle (jamais l'état en clair), la
// version du gabarit et celle du modèle.
Verdicts []Verdict `json:"verdicts,omitempty"`

// Lots : par niveau de privilège, les champs passés et la latence
// observée. Ne porte aucune valeur de champ.
Lots []RefLot `json:"lots,omitempty"`

// ClasseEffet : déclarée au manifeste, plancher, Jev, et retenue.
ClasseEffet *ClasseEffet `json:"classe_effet,omitempty"`
```

Règle de confidentialité, alignée sur le traitement existant des sujets :
**le journal porte l'empreinte de l'état, jamais l'état.** L'état lui-même
va au coffre sous cette empreinte, comme les arguments aujourd'hui.

### Rejeu

`internal/rejeu` ne change pas de structure. Il réinjecte `Verdicts` et
recalcule. Une nouvelle cause d'écart est à ajouter, distincte de
`alteration` :

```go
// CauseDeriveModele : le rejeu modèle (option explicite) a obtenu une
// distribution différente de celle journalisée. Ne qualifie jamais la
// décision d'origine, qui reste celle du rejeu logique.
CauseDeriveModele Cause = "derive_modele"
```

---

## 9. Décisions ouvertes

| # | Question | Option A | Option B | Penchant |
|---|---|---|---|---|
| D1 | Mode dégradé (Jev indisponible) | Tout refuser | Plancher seul : `lecture` autorisée, tout le reste en attente humaine | **B** — A rend le moteur inutilisable sur une panne réseau |
| D2 | Nombre de lots par famille | 2 (origine utilisateur / moteur) | N par champ | **A** pour P3 ; affiner si la mesure de corrélation le justifie |
| D3 | Caviardage de l'état avant envoi à Jev | Oui | Non | **Oui** si une démo bancaire est visée ; sinon reportable |
| D4 | Second étage de contrôles (DAG) | Oui | Non | **Non** en P3. Coût de latence et de rejeu pour un gain non démontré |

---

## 10. Protocole d'évaluation (P3)

C'est la phase qui tranche l'hypothèse. Sans elle, on a une démo, pas une
preuve.

### Jeu étiqueté

~200 cas minimum, dont **30 % d'injections** (texte hostile dans un champ
d'origine utilisateur : nom d'article, libellé, réponse de serveur).
Étiquette = l'issue attendue, posée à la main.

### Mesures

| Mesure | Calcul | Ce qu'elle prouve |
|---|---|---|
| FPR / FNR par contrôle | matrice de confusion standard | Admissibilité du contrôle (I6) |
| Calibration | confiance annoncée vs justesse observée, par décile | Si la confiance est honnête, la zone grise a un sens |
| **Erreur jointe** | `P(tous faux)` observé vs `Π P(faux_i)` attendu sous indépendance | **Corrélation réelle des erreurs** |
| Témoin plancher | mêmes cas, plancher déterministe seul | C2 : Jev apporte-t-il quelque chose ? |
| Résistance | FNR sur le seul sous-ensemble injections | Le vrai modèle de menace |

### Sur la corrélation des erreurs

Point à ne pas éluder : avec **un seul modèle**, les erreurs de contrôles
différents ne sont pas indépendantes. L'erreur se décompose en biais
(poids, représentation, sensibilité à la formulation) et variance
d'échantillonnage. Seule la seconde est indépendante d'un appel à l'autre.
Le biais est partagé, puisque tous les contrôles d'un lot passent par les
mêmes poids **et le même état**.

Un adversaire n'exploite pas la variance, il exploite le biais : une
injection pousse tous les contrôles qui lisent le champ infecté dans la
même direction. Un seul modèle interrogé N fois n'est pas un comité.

Ce qui décorrèle réellement, par ordre de coût :

1. moindre privilège sur les entrées (I7) — gratuit, déjà dans le design ;
2. facettes distinctes : structure / sémantique / cohérence de session /
   conformité au manifeste — question de conception des contrôles ;
3. plancher déterministe — biais nul vis-à-vis du modèle ;
4. redondance contradictoire sur les effets critiques (poser la question
   dans les deux sens ; incohérence ⇒ `Indetermine`) — 2× appels ;
5. second modèle d'une autre famille sur `paiement` et `compte` — un modèle
   de plus à épingler et à évaluer.

La mesure d'erreur jointe tranche. Le design ci-dessus ne dépend pas de son
résultat.

---

## 11. Phases

### P0 — Nettoyage (prérequis)

Taguer `pre-refonte` avant toute suppression. Tag `baseline-pre-hackathon`
déjà posé.

À retirer : `internal/console` (8 183 LOC), `cmd/console`,
`internal/registredemo`, `cmd/registre-demo`, `agents/orchestrateur`,
`agents/analyste`, `pyagent`, `docs/demo`, `demo/`, et les `servers/`
applicatifs.

**Couplage à traiter en premier** — découvert à la lecture, il casse les
tests si on supprime dans le mauvais ordre :

| Importeur | Dépendance | Traitement |
|---|---|---|
| `test/integration/integration_test.go` | `registredemo.Serveurs`, `registredemo.Imposteur` | Extraire la liste des fixtures dans `test/fixtures` **avant** de supprimer `registredemo` |
| `test/integration/v2_test.go` | idem | idem |
| `deploy/lab/registre-lab` | `registredemo` | Décider : garder le lab (et extraire) ou le retirer aussi |
| `test/extension/kit_test.go` | `internal/console` | Retirer le test de kit console avec la console |

Garder 2–3 serveurs neutres comme fixtures (`modele-synthetique`,
`gels-avoirs`, `taux-imposteur` sont ceux que les tests citent nommément),
régénérer le catalogue du compilateur.

Sortie : `main` propre, `go test ./...` vert.

### P1 — Paquet `jev` et type `Verdict`

- `directive.Verdict`, `directive.Controle`, `Mode`, `Confiance`, `Etat` ;
- `internal/jev` : `Classifier`, client HTTP, `Fixtures` ;
- `TestPurete` étendu : vérifier que `internal/directive` n'importe toujours
  pas `internal/jev`.

Sortie : Decide testable avec des verdicts, sans réseau.

### P2 — Decide, table de décision, journal, rejeu

- signature de Decide, combinaison par table, vérificateur statique de
  couverture et de monotonie ;
- plancher dans `internal/controle` ;
- champs de journal, rejeu logique ;
- test de propriété I2 ; non-régression `Decide(…, nil)`.

Sortie : **C1 démontrée.** C'est le cœur de la preuve, et le contenu du
talk.

### P3 — Évaluation

Jeu étiqueté, les cinq mesures du §10, témoin plancher.

Sortie : **C2 tranchée.** Succès ou échec, le résultat est publiable.

### P4 — Scénario code généré (vue d'ensemble)

Sandbox, proxy d'egress comme unique sortie, manifeste, admission, cache
épinglé (clé = `hash(spec normalisée, modèle, gabarit)` — **jamais les
arguments d'appel**, sinon le cache devient un vecteur d'injection
persistante), décision par effet.

Scénario de référence : commande Uber Eats.
`GET /stores` → `GET /menu` → `POST /carts` → `GET /carts/{id}` →
`POST /orders` (classe `paiement` ⇒ attente humaine) → fin. Six entrées
`appel` chaînées.

Cas qui justifie l'architecture : le code tente `PATCH
/users/me/addresses`, non couvert par la sandbox d'admission. Le plancher
le classe `compte`, hors manifeste ⇒ refus déterministe, journalisé, sans
consulter aucun modèle.

**P0→P3 ne touche pas au code généré.** La moitié risquée de l'hypothèse se
valide avant d'écrire la sandbox et le proxy, qui sont le gros de l'effort.
Si P3 échoue, P4 est économisée.

---

## 12. Ce que le document ne promet pas

- Que Jev soit déterministe. Il ne l'est pas. La décision l'est.
- Que le rejeu modèle garantisse quoi que ce soit. Il mesure une dérive.
- Que l'absence d'hallucination implique la justesse. Elle implique la
  bonne formation de la sortie.
- Que les erreurs des contrôles soient indépendantes. §10 les mesure.
- Que la sandbox d'admission observe tous les chemins. Elle en observe un ;
  c'est pourquoi la décision est prise par effet, à l'exécution.
