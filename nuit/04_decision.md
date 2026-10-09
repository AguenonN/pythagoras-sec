# 04 — Décision, journal, rejeu

**Démarre seulement une fois le chantier 01 fusionné dans `main`.** Tu dépends
du type `Verdict` et de `internal/effet`.

**C'est le chemin critique du projet.** Ce chantier porte la preuve : si le
rejeu exact n'est pas démontré ici, rien de ce qui est construit ailleurs n'a
de valeur de preuve. Il reste dans la session principale.

Lire `nuit/00_regles_communes.md` avant de commencer.

---

## Contexte

`Decide` est une fonction pure : mêmes entrées, même sortie, toujours. C'est
elle qui rend le rejeu possible — on reprend le journal, on réinjecte ce que
`Decide` avait lu, et on retrouve la décision au bit près.

Ton chantier lui ajoute une entrée — les **verdicts** d'un modèle de
classification — sans lui faire perdre cette propriété. Le modèle observe, il
ne décide pas : ses sorties entrent comme des données, au même titre que
l'attestation, et la combinaison se fait par une table écrite à la main.

`internal/directive` est sous test de pureté. Tout ce qui touche au réseau vit
ailleurs. Ce test ne se contourne pas.

## Worktree et branche

```
git worktree add ../pythagore-decision -b feat/decision
```

Réseau : registres de paquets Go. **Aucun appel de modèle** : tu travailles
exclusivement avec des verdicts fixtures. Aucun secret.

## Conception déjà tranchée

- La logique est **trivalente** — `Faux < Indetermine < Vrai`, `Et` = min. Le
  type `Trivalent` existe déjà dans `internal/directive`, avec ses tables.
- Les effets sont ordonnés et `plusFort` retient le plus contraignant. Ça
  existe déjà aussi : la monotonie est en place, il s'agit de ne pas la casser.
- La combinaison des verdicts se fait par **table de décision**, en lignes
  lisibles, jamais par une formule ni par un modèle. C'est ce qu'un humain
  relit pour valider une politique.
- `Decide(…, nil)` doit rendre **exactement** ce que l'actuel rend. Les
  scénarios existants sont le test de non-régression de toute la refonte.

## Ordre de travail — une étape, un commit

### 1. La table de décision

Le type d'une ligne : un motif sur les états des contrôles — avec joker — et
l'issue. L'ordre est celui du document, et la première ligne qui correspond
gagne.

```
{ effet_est_paiement: vrai, adresse_coherente: faux }        → refuser
{ effet_est_paiement: vrai, adresse_coherente: vrai }        → attendre
{ effet_est_paiement: *,    adresse_coherente: indetermine } → attendre
{ effet_est_paiement: faux, adresse_coherente: vrai }        → autoriser
```

### 2. Le vérificateur statique

Au chargement, et c'est l'invariant 4 :

- **Couverture** : toute combinaison de `{vrai, faux, indetermine}ⁿ` tombe sur
  au moins une ligne. Sinon le jeu est refusé — pas de refus à l'exécution sur
  une combinaison oubliée, un refus au chargement.
- **Monotonie** : sur une même ligne, remplacer un état par `indetermine` ne
  peut pas rendre une issue plus faible.

Pour cinq contrôles, 243 combinaisons : l'énumération exhaustive est la bonne
implémentation, pas une astuce.

Tests : une table complète passe ; une table à laquelle il manque une
combinaison est refusée en nommant laquelle ; une table non monotone est
refusée en nommant la ligne.

### 3. `Decide` avec verdicts

Ajouter le paramètre. Les verdicts entrent comme données ; `Decide` ne les
produit ni ne les demande.

Un verdict `Indetermine` suit la règle existante `si_indetermine`, dont le
défaut est `refuser`.

Tests : les scénarios v2 existants rendent exactement ce qu'ils rendaient —
`Decide(…, nil)` est non régressé — puis les cas avec verdicts.

### 4. Le test de propriété de monotonie

C'est l'invariant 1. Pour tout jeu, tout état, tout effet et tout vecteur de
verdicts engendrés :

```
Decide(…, verdicts).Effet >= Decide(…, nil).Effet
```

Un modèle ne peut que durcir. Ce test doit échouer si quelqu'un, un jour, câble
un verdict de façon à assouplir une décision.

### 5. L'entrée de journal `effet`

L'entrée `appel` devient une entrée `effet` et porte **tout ce que `Decide` a
lu**, verdicts compris.

Les verdicts journalisent la **distribution brute**. Mode, confiance et état se
recalculent en Go à la relecture : le rejeu ne doit dépendre d'aucun
post-traitement d'un fournisseur de modèle.

Le journal porte l'**empreinte** de l'état passé au modèle, jamais l'état en
clair — même traitement que les sujets aujourd'hui. L'état lui-même va au
coffre sous cette empreinte, comme les arguments.

Tests : une entrée se sérialise et se relit sans perte ; la chaîne MAC reste
vérifiable ; aucune valeur de champ en clair dans l'entrée.

### 6. Le rejeu logique

Réinjecter les verdicts journalisés et retrouver l'issue. `internal/rejeu`
qualifie déjà les écarts en *purge*, *altération* et *effet de règle* : ne pas
toucher à cette machinerie, l'étendre.

Ajouter une cause distincte pour le mode « rejeu modèle », qui relance
l'inférence et compare : un écart y est une **dérive du modèle**, et ne remet
jamais en cause la décision d'origine. Le mode lui-même n'est pas à
implémenter — seulement la cause et sa place dans le rapport.

Tests : un journal de bout en bout se rejoue à l'identique ; un journal dont un
octet a changé est détecté comme altéré.

### 7. Rapport final

## Critère de réussite

```bash
go test ./internal/directive/... ./internal/journal/... ./internal/rejeu/... -count=1
```

Et surtout, le critère **C1** du plan :

> 100 % des effets journalisés se rejouent à l'identique.

Avec, en plus : `purete_test.go` toujours vert, le test de propriété de
monotonie vert, et les scénarios v2 existants inchangés.

## En cas de blocage

- Si le vérificateur de couverture devient lent sur un grand nombre de
  contrôles : garde l'énumération exhaustive et borne le nombre de contrôles
  par jeu. La vérifiabilité vaut mieux que la vitesse d'un chargement qui a
  lieu une fois.
- Si un scénario existant régresse : **c'est un bug de ton côté**, pas un
  scénario à mettre à jour. Ne jamais modifier un test existant pour le faire
  passer ; noter et s'arrêter sur ce point.
- Ne jamais ajouter d'appel réseau dans `internal/directive`, même temporaire,
  même « pour tester ». Le test de pureté est la raison pour laquelle le rejeu
  est exact.
