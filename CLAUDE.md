# pythagoras-sec — consignes pour le hackathon Accel AI Innovate (22/10/2026)

Branche de travail : `hackathon/aiven`. Rendu vers 15h30, **gel du code à 14h00** :
après, seulement des corrections de démo, plus de fonctionnalité.

## Ce que fait le projet

Des agents IA (orchestrateur Mistral + sous-agents déclaratifs, `agents/orchestrateur`)
appellent des outils MCP à travers `mcp-gate`. Chaque appel passe par
`directive.Decide` : fonction pure (ni horloge, ni réseau, ni LLM) qui applique
les directives écrites par la conformité. Chaque décision est écrite dans un
journal chaîné par HMAC, avec tout ce qu'elle a lu ; les arguments vont au coffre.
Le rejeu (`internal/rejeu`) recalcule toute décision passée.

Track du hackathon : *Data-Centric Agents*. Ajout du jour : la couche de données
sur Aiven — voir `docs/hackathon-aiven.md`.

| Ajout du jour | Où |
|---|---|
| Journal → PostgreSQL, impact rétroactif (`-gels`), « et si cette règle avait existé » (`-apres`) | `cmd/journal-sql` |
| Page Detective en lecture seule (pour Aiven Runtime) | `apps/detective` |
| Règle de démo prête | `demo/scenarios/jeux/hypothese-seuil-officiel.json` |

## Règles à ne pas enfreindre

- **Ne pas toucher au moteur** (`internal/directive`, `internal/journal`, `internal/rejeu`,
  `internal/gate`). `internal/directive/purete_test.go` échoue si `Decide` importe
  `os`, `net`, `context`, `time.Now`… Tout ajout se branche autour.
- **Aucune dépendance Go ajoutée** aujourd'hui (`go.mod`, `go.sum` inchangés).
  `journal-sql` écrit du SQL sur stdout, envoyé à `psql`.
- **Aucun secret dans le dépôt** : `AIVEN_PG_URI`, `MCP_GATE_CLE_JOURNAL`,
  `MISTRAL_API_KEY` vivent dans l'environnement. Ne jamais les afficher dans la
  vidéo ni les committer. `apps/detective/.venv` reste local.
- **Données fictives** (banque synthétique). Une entreprise réelle de l'annuaire
  public (ex. le SIREN de DOS-FICTIF-003) ne se présente jamais comme gelée :
  « supposons que ce SIREN soit ajouté ». Dire ce qui vient d'un passage réel et
  ce qui vient d'un rejeu enregistré.
- Pas de rebase ni de force-push sur `hackathon/aiven` : commits courts, `git pull` avant.

## Commandes

Pour la démo en direct, tout passe par `scripts/demo-hackathon.sh` (`verifier`,
`preparer`, `passage`, `gels`, `regle`, `regle-live "…"`, `detective`). Le détail :

```bash
# Go >= 1.25
go test ./cmd/journal-sql/ && go build -o bin/journal-sql ./cmd/journal-sql
go test ./...                                   # avant tout push qui touche du Go

# Préparation et passages hors ligne (clé du journal : la même pour toute la journée)
export MCP_GATE_CLE_JOURNAL=…
scripts/lancer-demo.sh
scripts/lancer-demo.sh scenario base-interne-avec-d009 --mode rejeu
scripts/lancer-demo.sh scenario base-interne-sans-d009 --mode rejeu

# Aiven
bin/journal-sql schema | psql "$AIVEN_PG_URI"
bin/journal-sql exporter <session>/journal.jsonl | psql -q "$AIVEN_PG_URI"
bin/journal-sql impacts <session>/journal.jsonl -directives demo/scenarios/jeux/avec-d009.json \
    -apres demo/scenarios/jeux/hypothese-seuil-officiel.json | psql -q "$AIVEN_PG_URI"

# Page Detective (en local tant que Runtime n'est pas activé)
python3 -m venv apps/detective/.venv && apps/detective/.venv/bin/pip install -r apps/detective/requirements.txt
DATABASE_URL="$AIVEN_PG_URI" PORT=8090 apps/detective/.venv/bin/python apps/detective/app.py
```

`impacts -apres` sur la session « avec D-009 » doit donner 2 décisions changées
(les deux `attribuer_pret`). Le jeu passé à `-directives` doit être exactement
celui du journal (même identifiant, version et empreinte), sinon le rejeu s'arrête.

## Modifier l'interface (console)

Le front est dans le dépôt : `internal/console/web/` (HTML, CSS, JavaScript sans
dépendance ni build), servi par `cmd/console`. Guide complet :
`docs/equipe/modifier-la-console.md` (un écran = un fichier `ecran-*.js`, socle et
helper `h()` dans `app.js`, styles dans `app.css`).

```bash
source ~/.pythagoras-demo.env     # MCP_GATE_CLE_JOURNAL (sinon aucun journal affiché), MISTRAL_API_KEY
go run ./cmd/console -racine "$PWD" -dev internal/console/web -conseiller camille.conseiller@exemple.test
# http://127.0.0.1:8088 ; avec -dev, modifier un fichier puis recharger la page
```

- Sans `-dev`, les fichiers sont embarqués : `go build` pour voir un changement.
- Ne changer les formes JSON (`/api/...`) qu'avec leur structure Go (`vues2.go`,
  `compil.go`…) et `go test ./internal/console/...`.
- Aucune ressource externe (CDN, police distante) : l'interface tourne hors ligne.
- Un fichier par personne si possible : `app.css` et `app.js` sont partagés par
  tous les écrans, se répartir les écrans évite les conflits.
- Avant de pousser : `go test ./internal/console/...`.

## Compiler une règle en direct (compilateur, Mistral)

Nommer l'action dans le texte (« attribution de prêt »), sinon le modèle peut
cibler `modifier_plafond_decouvert`. Fondement par `--fondement`, réponse à
`si_indetermine` par `--reponses` (`{"champ": "si_indetermine", "reponse": "refuser",
"origine": "compilateur", …}`). Les réponses aux questions de Mistral, elles,
n'ont **pas** de champ `origine`. Lire la relecture avant tout rejeu.

## Hors périmètre aujourd'hui

Les contrôles segmentés avec un classifieur (Jev, `docs/controles-segmentes.md`) :
« next step » sur la slide, pas de code.
