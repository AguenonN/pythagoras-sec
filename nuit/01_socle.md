# 01 — Socle : nettoyage et contrats

**Cette session passe en premier et seule.** Les trois autres chantiers lisent
les types que tu écris ici. Tant qu'elle n'est pas fusionnée dans `main`,
aucune autre ne démarre.

Lire `nuit/00_regles_communes.md` avant de commencer. La section 6 contient les
contrats : tu les **transcris**, tu ne les redessines pas.

---

## Contexte

Le dépôt contient un moteur de décision pour appels d'outils MCP : une fonction
pure `Decide`, un journal chaîné par MAC, un rejeu exact, l'identité de workload
SPIFFE. Il suppose aujourd'hui un catalogue d'outils connu à l'avance.

Le projet abandonne cette hypothèse : l'outil est fabriqué à la demande, sous
forme de **descripteur** interprété par un runtime écrit à la main. Ton chantier
retire ce qui supposait le catalogue fini, et pose les types du nouveau modèle.

Déjà commité et utilisable : `internal/chaine`, `session`, `annuaire`,
`identity`, `approval`, `rejeu`, `controle`, `journal`, `directive`, `toolpin`,
`envelope`, `scrub`, `guardrail`.

## Worktree et branche

```
git worktree add ../pythagore-socle -b feat/socle
```

Réseau : registres de paquets Go uniquement. Aucun secret.

## Conception déjà tranchée

- Deux types d'état dans le descripteur : `appel` et `choix`. La liaison est
  une propriété de `appel`. Ne pas en ajouter un troisième.
- Les classes d'effet et les paliers partagent la même échelle ordonnée.
- Le chargeur est strict : tout champ hors schéma est un refus désigné par sa
  position, comme le chargeur de directives existant.
- Les types vont dans des paquets **nouveaux**, sauf `Verdict` qui va dans
  `internal/directive` parce qu'il est lu par `Decide`. Comme le paquet est
  sous test de pureté, `Verdict` ne contient que des données.

## Ordre de travail — une étape, un commit

### 1. Tag et inventaire

```bash
git tag pre-refonte && git tag -l
```

### 2. Extraire les fixtures AVANT toute suppression

`test/integration/integration_test.go`, `test/integration/v2_test.go` et
`deploy/lab/registre-lab` importent `internal/registredemo` pour `Serveurs` et
`Imposteur`. **Supprimer `registredemo` avant d'avoir extrait cette liste casse
les tests d'intégration**, c'est-à-dire le filet, au moment où il sert le plus.

Créer `test/fixtures` avec la liste des serveurs neutres, et faire pointer les
tests dessus. `go test ./test/...` vert avant de commiter.

### 3. Retirer la couche applicative

`internal/console`, `cmd/console`, `cmd/console/simulateur`,
`internal/console/simulateur`, `internal/registredemo`, `cmd/registre-demo`,
`cmd/greffe`, `agents/analyste`, `agents/orchestrateur`, `pyagent`, `demo/`,
et les serveurs applicatifs de `servers/` sauf les deux ou trois gardés en
fixtures. Retirer aussi `test/extension/kit_test.go`, qui testait la console.

Régénérer le catalogue du compilateur. `go test ./...` vert.

### 4. Les classes d'effet

Paquet `internal/effet` : le type `Classe`, son ordre, son `String()`, son
encodage JSON, et `Max(a, b Classe) Classe`.

Test : `Max` est bien le maximum sur l'ordre déclaré, et le zéro de `Classe`
est `Lecture`.

### 5. Le schéma du descripteur

Paquet `internal/descripteur` : les types Go du schéma de la section 6.2, et un
chargeur strict.

Le chargeur refuse, en nommant la position : un champ inconnu, un `operation`
vide, un `id` dupliqué, un `choix` dont les `cas` ne sont pas exhaustifs, et
`reprise.max > 0` sur une étape `reversible: false`.

Il ne vérifie **pas** l'existence des `operation` dans une spec : c'est le
chantier 02 qui s'en charge, avec la spec en main.

Tests : un descripteur valide se charge ; chacun des cinq refus a son test et
son message désigne le fautif.

### 6. Le périmètre

Paquet `internal/perimetre` : `EClasse`, `EInstance`, `Canoniser()`, et
l'arrondi d'une classe demandée au palier minimal qui la couvre.

`Canoniser` est déterministe : domaines triés et en minuscules, palier sur
l'ensemble fermé, puis sha256 d'une sérialisation canonique. Deux `EClasse`
égales au sens métier rendent la même empreinte, dans n'importe quel ordre
d'insertion.

`Administration` n'est jamais rendu par l'arrondi : la fonction retourne une
erreur qui dit qu'une admission propre est requise.

Tests : déterminisme de l'empreinte sur permutations, arrondi sur les quatre
paliers, refus de l'arrondi vers `Administration`.

### 7. `Verdict`

Dans `internal/directive` : le type de la section 6.5 et ses trois méthodes.
Aucun client, aucun réseau — le test de pureté doit rester vert.

`Mode` est l'indice du maximum de la distribution. `Confiance` suit la formule
publiée : `(p_max − 1/n) / (1 − 1/n)`. `Etat` rend `Indetermine` si `Erreur`
est non vide, si la distribution est vide ou mal formée, ou si la confiance
tombe dans la zone grise du contrôle.

Tests : distribution à deux valeurs, à n valeurs, vide, erreur non vide, zone
grise aux deux bords.

### 8. Rapport final

Compléter `RAPPORT_NUIT.md`.

## Critère de réussite

```bash
go test ./... -count=1          # vert
go vet ./...                    # propre
git log --oneline main..HEAD    # un commit par étape
```

Et, dans l'ordre : le tag `pre-refonte` existe, les quatre nouveaux paquets
existent avec leurs tests, `internal/directive/purete_test.go` passe toujours.

## En cas de blocage

- Si une suppression de l'étape 3 casse un test que tu n'arrives pas à
  réparer en trois tentatives : **laisser le paquet en place**, le noter, et
  continuer. Un paquet mort de plus est moins coûteux qu'un socle non livré.
- Si le chargeur strict devient compliqué sur un cas de bord : refuser plutôt
  qu'accepter, et noter le cas.
- Ne jamais désactiver ni assouplir `purete_test.go`.
