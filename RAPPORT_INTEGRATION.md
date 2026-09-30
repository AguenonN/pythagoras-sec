# Rapport — session « Intégration » : premier passage de bout en bout

Branche `feat/integration`, worktree `~/cybercar/mcp_rogue-integration`. Rien n'a été poussé ni fusionné, `main` n'a pas été touchée.

## 1. Ce qui est fait

| Étape | Commit | Objet |
|---|---|---|
| 1 | (ce commit) | `cmd/registre-demo` + `internal/registredemo` : `demo/registre.json` et `demo/registre-attaque.json` générés depuis `bin/manifeste.json` |

### Étape 1 — registre de démo

- `go run ./cmd/registre-demo` lit `bin/manifeste.json`, vérifie l'empreinte de chaque binaire (imposteur compris) et de chaque `schema.json` contre le manifeste, et écrit les deux registres. Il échoue si un serveur manque, est en double ou inattendu, ou si un fichier diffère du manifeste.
- Huit serveurs (pas l'imposteur), stdio, attestation `binaire`. Outils recopiés de `tools/list` : nom, description et `inputSchema` **tels quels** (c'est ce que le backend épingle) ; `read` seulement si `readOnlyHint` est vrai, `action` sinon.
- `env` par nom : `DONNEES_MODE` pour les huit, plus `PISTE_CLIENT_ID` et `PISTE_CLIENT_SECRET` pour `legifrance`.
- Chemins absolus : binaire, `-enregistrements` (taux-officiel, recherche-entreprises, eurlex, legifrance, gels-avoirs), `-instantane` (banque-synthetique).
- Registre d'attaque : même document, commande de `taux-officiel` → `bin/taux-imposteur`, empreinte attendue inchangée.
- Deux exécutions : mêmes octets (vérifié à la main et par test).

## 2. Ce qui n'est pas fait

Étapes 2 à 7 : en cours.

## 3. Décisions prises seul

1. **`demo/registre*.json` ne sont pas versionnés** (ajoutés au `.gitignore`). Ils portent des chemins absolus propres au worktree et les empreintes des binaires construits localement (`bin/` n'est pas versionné non plus). Ils sont régénérés par `go run ./cmd/registre-demo` (et par le lanceur de l'étape 7). Le plus simple à défaire : retirer deux lignes du `.gitignore`.
2. **gels-avoirs pointe par défaut sur `enregistrements_locaux`** (registre complet, hors Git) dans le registre de démo ; `-gels servers/gels-avoirs/enregistrements` donne le registre synthétique, utilisé par les tests. Registre local absent → le serveur répond `indisponible` (comportement existant), jamais « aucune correspondance ».
3. **`DONNEES_MODE` est transmis aux huit serveurs**, y compris les trois synthétiques : tous passent par `lanceur`, qui lit la variable ; elle est sans effet sur eux.
4. **Conséquence du « fermé par défaut »** : `get_taux_usure`, `rechercher_entreprise`, `verifier_gel`, les outils eurlex et legifrance n'ont pas d'annotation `readOnlyHint` et sont donc classés `action` au registre, alors que ce sont des lectures. Je n'ai pas modifié les serveurs (hors de ma mission). Voir question 1.

## 4. Blocages

Aucun pour l'instant.

## 5. Questions pour Manu

1. (moyenne) Ajouter `readOnlyHint: true` aux outils de lecture des serveurs de données officiels ? Aujourd'hui ils sont `action` au registre, ce qui est correct par défaut mais trompeur dans la vue « contrôles en vigueur ».

## 6. Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-integration
scripts/construire-serveurs.sh
go run ./cmd/registre-demo && sha256sum demo/*.json
go run ./cmd/registre-demo && sha256sum demo/*.json   # mêmes empreintes
go test -race ./internal/registredemo/
```
