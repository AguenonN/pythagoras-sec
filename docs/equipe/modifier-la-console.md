# Modifier la console

La console est un binaire Go (`cmd/console`) qui sert une interface web simple
(HTML, CSS, JavaScript sans dépendance). Elle lit les sessions de
`demo/execution/sessions/*/` et les règles de `demo/directives/demo.json`.

## Travailler sans recompiler

```bash
export MCP_GATE_CLE_JOURNAL=<hex>      # la même clé que le gate
go run ./cmd/console -dev internal/console/web
```

Avec `-dev`, les fichiers HTML, CSS et JS sont lus **sur le disque** à chaque
requête : modifier, recharger la page. Sans `-dev`, ils sont embarqués dans le
binaire (`go build` pour les y remettre).

Pour essayer sans la démo réelle (règles, annuaire signé et clés fictifs,
orchestrateur simulé) :

```bash
docs/rapports/console-captures/preparer-captures.sh /tmp/essai 8088
```

## Où est chaque écran

Tout est dans `internal/console/web/`. Un écran = un fichier qui s'enregistre
dans `ECRANS` (`app.js` porte le socle : gabarit, navigation, formats, helper
`h()`).

| Écran | Fichier | API appelée (`internal/console/serveur.go`) |
|---|---|---|
| Journal et chaîne | `ecran-journal.js` | `GET /api/journal/{id}/session/{sid}/chaine` |
| Poste conseiller | `ecran-poste.js` | `GET …/poste`, `GET /api/identites`, `POST /api/lancer`, `GET /api/lancements` |
| Vitrine des règles | `ecran-regles.js` | `GET /api/regles?directive=`, `POST /api/regles/statut` |
| Avant / après | `ecran-avant-apres.js` | `GET /api/avant-apres?directive=&sessions=` |
| File du superviseur | `ecran-validation.js` | `GET /api/file`, `POST /api/validation/valider`, `POST /api/validation/rejeter` |
| Rédaction | `ecran-redaction.js` | `GET /api/redaction`, `POST /api/redaction/compiler`, `…/essai`, `…/mettre-en-vigueur`, `POST /api/propositions/{id}/decision` |
| Notifications | `ecran-notifications.js` | `GET /api/notifications` |
| Bandeau, onglets | `app.js` | `GET /api/etat` |

Les formes JSON sont les structures Go : `chaine.go` (écran 1), `poste.go`
(2), `vues2.go` (3 et 4), `vues3.go` (5), `compil.go` (6), `notifs.go`.

## Ajouter un écran

1. Un fichier `ecran-xxx.js` : `ECRANS.xxx = async (ctx) => { … ctx.monter(noeud); }`
   (`ctx.q` = paramètres de l'adresse, `ctx.suivre(f)` = sondage toutes les 2 s,
   `ctx.vivant()` = l'écran est toujours affiché).
2. L'ajouter à `index.html` (avant `demarrage.js`) et à `NAV` dans `app.js`.
3. Une route dans `Serveur.Handler` et une fonction de vue Go (qui **lit**, ne
   décide pas) ; un test dans `internal/console`.

## Règles à ne pas casser (testées)

- **Aucune ressource externe** : ni CDN, ni police distante (`TestAucuneRessourceExterne`).
- **Pas d'`innerHTML`** ; le style passe par `style:` du helper `h()` (la CSP
  interdit l'attribut `style`) (`TestVocabulaireEtCSP`).
- **La console ne décide rien** : chaque badge, compteur ou contrôle vient du
  journal, de `etat.json` ou d'une commande de l'engine. Ce qui manque s'affiche
  « non disponible » ou « non mesuré », jamais une valeur supposée.
- **Vocabulaire** pour des juristes : pas de « jeu », de « v1 / v2 » ; une
  directive s'écrit « D-001, version 2 » ; les règles « en vigueur depuis le … » ;
  la comparaison « Sans la directive D-00n » / « Avec la directive D-00n ».
- Écritures limitées à : `demo/directives/demo.json` (et `historique/`),
  `origines.json`, `demo/execution/approbations/`,
  `demo/execution/propositions/*.decision.json`. Toujours atomiques.
- Les tranches vides d'une réponse JSON sont `[]`, jamais `null` (l'interface
  appelle `.includes`, `.map`…).
