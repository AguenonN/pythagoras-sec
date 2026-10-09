# Lancement

Mode opératoire. À suivre dans l'ordre.

---

## 0. Avant tout — renommer le dépôt

Le projet change de nom. **Renommer plutôt que cloner** : un clone diverge, et
on en a déjà la preuve avec `secu-mcp`, qui s'est écarté de `mcp_rogue` sur
`approval`, `identity` et `proxy` sans que personne ne l'ait décidé. Un
troisième dépôt referait la même erreur.

Sur GitHub : **Settings → Repository name**. L'historique, les tags et les
issues sont conservés, et l'ancienne URL redirige.

Puis, dans le clone local :

```bash
git remote set-url origin https://github.com/AguenonN/<nouveau-nom>
git remote -v
```

`secu-mcp` reste l'extraction publique du socle, à sens unique.

---

## 1. Préparatifs

- `git status` vide sur `main`, et `main` à jour.
- Mise en veille de la machine désactivée.
- Aucun secret n'est nécessaire pour ces quatre chantiers. Si une session en
  réclame un, c'est qu'elle sort de son brief.

## 2. Le chantier 01 passe seul, et en premier

Les trois autres lisent les types qu'il écrit. Tant qu'il n'est pas fusionné
dans `main`, **ne pas lancer les autres** : trois sessions écriraient chacune
sa version des contrats, et la fusion coûterait plus que le parallélisme n'aura
rapporté.

```bash
git worktree add ../pythagore-socle -b feat/socle
mkdir -p ../pythagore-socle/nuit
cp nuit/00_regles_communes.md nuit/01_socle.md ../pythagore-socle/nuit/
```

Puis, dans un terminal :

```bash
cd ../pythagore-socle
claude
```

Vérification dans la session — le `!` collé au début de la ligne, **sans
espace avant**, sinon la commande part au modèle au lieu du shell :

```
!pwd && git branch --show-current && ls nuit/
```

Message de démarrage :

> Lis `nuit/00_regles_communes.md` puis `nuit/01_socle.md`, et exécute ce brief
> en entier, de manière autonome, jusqu'à son critère de réussite ou jusqu'à ce
> que plus rien ne soit faisable. Personne ne répondra : applique les règles
> d'autonomie. Tiens `RAPPORT_NUIT.md` à jour après chaque commit.

Quand il est fini : relire ses décisions prises seul, fusionner dans `main`,
tests verts.

```bash
git checkout main && git merge --no-ff feat/socle
go test ./... -count=1
git worktree remove ../pythagore-socle
```

## 3. Les trois chantiers en parallèle

Seulement après la fusion du 01.

```bash
git worktree add ../pythagore-executeur -b feat/executeur
git worktree add ../pythagore-perimetre -b feat/perimetre
git worktree add ../pythagore-decision  -b feat/decision
git worktree list
```

Copier dans chaque worktree les règles communes **et son seul brief** :

```bash
for c in executeur perimetre decision; do mkdir -p ../pythagore-$c/nuit; done
cp nuit/00_regles_communes.md nuit/02_executeur.md ../pythagore-executeur/nuit/
cp nuit/00_regles_communes.md nuit/03_perimetre.md ../pythagore-perimetre/nuit/
cp nuit/00_regles_communes.md nuit/04_decision.md  ../pythagore-decision/nuit/
```

Un terminal par session, `cd` dans le worktree, puis `claude`. Vérifier `pwd`
et la branche avant le message de démarrage — une session lancée dans le dépôt
principal au lieu de son worktree est l'erreur la plus fréquente, et la plus
coûteuse.

Même message de démarrage, avec le bon numéro de brief.

**Le chantier 04 reste dans ta session principale** : c'est le chemin critique,
c'est lui qui porte la preuve.

Rester 10 à 15 minutes pour accorder les autorisations — l'option vaut pour
toute la session — puis attendre le premier commit de chacune.

## 4. Le lendemain

```bash
for c in executeur perimetre decision; do
  echo "===== $c"
  git -C ../pythagore-$c log --oneline main..HEAD
  sed -n '1,40p' ../pythagore-$c/RAPPORT_NUIT.md
done
```

Dans cet ordre :

1. **Relire d'abord les décisions prises seules** par chaque session, et les
   accepter, corriger ou rejeter. C'est là que la dérive se loge.
2. Fusionner **une branche à la fois**, tests verts entre chaque. L'ordre
   conseillé : `perimetre`, puis `decision`, puis `executeur` — le dernier
   dépend des interfaces que les deux premiers remplissent.
3. Conflit bénin attendu sur `go.mod` si une session a ajouté une dépendance.
4. `git worktree remove ../pythagore-<chantier>` pour chaque chantier terminé.

## Erreurs à ne pas commettre

- Lancer une session dans le dépôt principal au lieu de son worktree.
- Un espace avant le `!` dans Claude Code : la commande part au modèle.
- Lancer les trois chantiers avant que le 01 soit fusionné.
- Laisser deux sessions écrire leur propre version d'un contrat de la
  section 6 des règles communes.
