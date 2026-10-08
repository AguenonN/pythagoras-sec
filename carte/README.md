# Carte des briques

Documentation navigable du dépôt : un arbre où chaque brique est un paquet Go
et où **les liens sont les imports réels**, extraits du code. Une carte dessinée
à la main ment dès le premier refactor ; celle-ci se régénère.

## Régénérer

```bash
python3 scripts/carte-paquets.py        # lit le code → carte/graphe.json
```

Le script scanne `internal/`, `cmd/` et `pkg/`, lit les `import` des fichiers
non-test, et calcule pour chaque paquet ses dépendances, ses dépendants et sa
profondeur topologique (0 = aucune dépendance interne). Les colonnes de la
carte sont cette profondeur : fondations à gauche, consommateurs à droite.

## Les trois fichiers

| Fichier | Rôle | Qui l'écrit |
|---|---|---|
| `graphe.json` | Structure + poids, régénéré à chaque fois | le script |
| `annotations.json` | Couche, sort, explications | à la main |
| `page.html` | Le rendu, avec `/*__DATA__*/` à substituer | une fois |

Le graphe s'écrase, les annotations non : ajouter un paquet, relancer le
script, il apparaît dans la liste « sans annotation » à compléter.

## Annoter un paquet

```json
"internal/chaine": {
  "layer":  "moteur",              // socle | moteur | lab | demo
  "fate":   "garder",              // garder | adapter | jeter
  "title":  "Chaîne infalsifiable",
  "what":   "Ce que ça fait. <code>HTML</code> admis.",
  "why":    "Pourquoi ça compte — la propriété de sécurité.",
  "future": "Ce que ça devient dans la refonte. Peut rester vide."
}
```

`what` est la description, `why` la raison d'être, `future` le devenir. Les
deux dernières sont facultatives : une brique du lab n'a pas besoin des trois.

## Publier

```bash
python3 - <<'PY'
import json, pathlib
tpl = pathlib.Path("carte/page.html").read_text(encoding="utf-8")
data = pathlib.Path("carte/graphe.json").read_text(encoding="utf-8")
pathlib.Path("carte/carte.html").write_text(
    tpl.replace("/*__DATA__*/", data), encoding="utf-8")
PY
```

`carte/carte.html` est une page autonome : aucune dépendance hors les polices
Google. Les couleurs sont des tokens CSS sur `:root`, redéfinis pour le thème
sombre.

## Réutiliser sur un autre dépôt

```bash
cp scripts/carte-paquets.py carte/page.html ../autre-projet/
python3 carte-paquets.py --racine ../autre-projet --module nom-du-module
```

Seul `RACINES` en tête de script dépend d'une convention de dépôt. Le reste
marche sur n'importe quel module Go.
