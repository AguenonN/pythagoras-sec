# Rapport — session « Intégration » : premier passage de bout en bout

Branche `feat/integration`, worktree `~/cybercar/mcp_rogue-integration`. Rien n'a été poussé ni fusionné, `main` n'a pas été touchée.

## 1. Ce qui est fait

| Étape | Commit | Objet |
|---|---|---|
| 1 | `529a0c3` | `cmd/registre-demo` + `internal/registredemo` : `demo/registre.json` et `demo/registre-attaque.json` générés depuis `bin/manifeste.json` |
| 2 | `87f4166` | Catalogue du compilateur généré depuis le registre (`agents/compilateur/exemples/catalogue-demo.json`), tests de divergence Go et Python |
| 3 | `b1432c4` | `demo/directives/credit-demo.json` (v1, quatre directives actives), accepté par le gate devant le registre de démo |
| 4 | `352253e` | `agents/boucle` alignée sur le §6 du contrat de l'engine (refus, attente, indisponible, outil qui décline), `MAX_APPELS` = 25 |
| 5 | (ce commit) | Sondes Mistral (`boucle.sondes`), conversion relâchée (`additionalProperties` booléen, `type` `[T, "null"]`), 5 scénarios de la boucle réenregistrés |

### Étape 1 — registre de démo

- `go run ./cmd/registre-demo` lit `bin/manifeste.json`, vérifie l'empreinte de chaque binaire (imposteur compris) et de chaque `schema.json` contre le manifeste, et écrit les deux registres. Il échoue si un serveur manque, est en double ou inattendu, ou si un fichier diffère du manifeste.
- Huit serveurs (pas l'imposteur), stdio, attestation `binaire`. Outils recopiés de `tools/list` : nom, description et `inputSchema` **tels quels** (c'est ce que le backend épingle) ; `read` seulement si `readOnlyHint` est vrai, `action` sinon.
- `env` par nom : `DONNEES_MODE` pour les huit, plus `PISTE_CLIENT_ID` et `PISTE_CLIENT_SECRET` pour `legifrance`.
- Chemins absolus : binaire, `-enregistrements` (taux-officiel, recherche-entreprises, eurlex, legifrance, gels-avoirs), `-instantane` (banque-synthetique).
- Registre d'attaque : même document, commande de `taux-officiel` → `bin/taux-imposteur`, empreinte attendue inchangée.
- Deux exécutions : mêmes octets (vérifié à la main et par test).

### Étape 2 — catalogue du compilateur

- `go run ./cmd/registre-demo -catalogue agents/compilateur/exemples/catalogue-demo.json` écrit, en plus des registres, le catalogue dérivé du registre de démo : huit serveurs, outils avec description et accès, arguments en pointeurs JSON (descente dans les sous-objets, RFC 6901).
- Tests de divergence : `internal/registredemo.TestCatalogueVersionne` (Go) et `agents/tests/test_catalogue_demo.py` (Python) redérivent le catalogue des `schema.json` versionnés, et de `demo/registre.json` quand il a été généré ; toute différence échoue.
- `README`, `compilateur/__main__.py`, `compilateur/demo.py` et `test_cli` utilisent le catalogue généré.

### Étape 3 — jeu de directives de démo

- `demo/directives/credit-demo.json`, jeu `credit-demo` v1, verrous v1 existants, toutes `active`, date d'effet 2026-09-01 (passée : le gate lit l'horloge réelle) :
  - D-001 `provenance_session` : `taux-officiel` consulté → `attendre_validation` sur `banque-synthetique/attribuer_pret` ;
  - D-002 `seuil_argument` : `/montant > 500000` → `refuser` sur `attribuer_pret` (`si_indetermine: refuser`) ;
  - D-003 `source_attestee` sur `taux-officiel/*` → `refuser` ;
  - D-004 `champs_autorises_par_destination` : `/siren` seul vers `recherche-entreprises/*`, mode `caviarder`.
- Fondements marqués dans `fondement.texte` : « RGPD — à valider par le juriste » (le schéma v1 n'a pas de champ de note, et je ne le modifie pas) ; `texte_fr` le rappelle.
- `internal/registredemo.TestDirectivesDemo` : chargeur strict, puis `gate.Demarrer` devant le registre de démo (contrôle des cibles et des provenances) ; contre-épreuve : sans `taux-officiel` au registre, le même jeu est refusé (D-001, D-003).
- Vérifié aussi à la main : `mcp-gate` réel avec `demo/registre.json` et ce jeu, `DONNEES_MODE=REJEU` → les huit serveurs `attesté=true`.

### Étape 4 — boucle alignée sur le contrat de l'engine

- `boucle.classer` lit le `statut` du `structuredContent` **seulement sur un résultat `isError`** (une réponse normale du gate n'a jamais de `structuredContent` en clair ; un serveur ne peut donc pas se faire passer pour l'engine) :
  - `refuse` : directive citée « D-002 v1 », « le registre demo v1 » ou « l'engine » selon `origine` ; motif et fondement gardés ; message de blocage inchangé ;
  - `en_attente` : `empreinte_transmise` (et plus `empreinte` / `action_digest`), directive citée dans la réponse au demandeur ;
  - `indisponible` : nouveau cas ; le modèle lit « Aucune donnée obtenue de serveur/outil : motif. Ne présente aucune donnée de cette source comme obtenue. », la boucle continue ;
  - `isError` sans structure : l'outil décline, contenu transmis tel quel.
  - Une erreur JSON-RPC -32001 (convention de `cmd/mcp-proxy`) reste un refus.
- Faux serveur des tests au format du §6 ; nouveau `tests/test_boucle_contrat.py` sur les formes exactes de `internal/gate/reponses.go` (11 tests).
- `MAX_APPELS` de la boucle : 20 → 25, le budget de ce brief ; compteur dans `agents/.etat/boucle/` (vide au départ dans ce worktree).
- **Aucun scénario enregistré n'est touché par cette étape** : les messages envoyés à Mistral sont identiques octet pour octet (le refus « D-003 » version 1 s'affiche toujours « D-003 v1 », l'attente suspend la boucle sans rappeler le modèle), et les 5 scénarios rejoués passent sans nouvel enregistrement. 0 appel réel. Le réenregistrement viendra avec l'étape 5, qui change la définition d'un outil du faux serveur et donc toutes les requêtes.

### Étape 5 — mots-clés JSON Schema réellement utilisés

Mots-clés présents dans les 13 outils des huit schémas épinglés (`demo/registre.json`), avec leur nombre d'occurrences :

| Mot-clé | Occurrences | Avant | Après |
|---|---|---|---|
| `type` | 34 (dont 1 liste `["null","array"]`, `rh-synthetique/decider_prime`) | conservé ; la liste était **réduite** à `array` | conservé ; `[T, "null"]` conservé tel quel |
| `description` | 18 | conservé | conservé |
| `properties`, `required` | 13 chacun | conservés | conservés |
| `additionalProperties` (toujours `false`) | 13 | **retiré** | conservé s'il est booléen ; sous forme de schéma, toujours retiré (forme non sondée) |
| `enum` | 1 | conservé | conservé |
| `items` | 1 | conservé | conservé |

- Deux constructions retirées ou réduites → deux sondes (`python -m boucle.sondes --mode reel`) : requête minimale, une seule fonction portant la construction sous la forme exacte des schémas épinglés. **Les deux sont acceptées (HTTP 200)**. Verdicts dans `agents/boucle/sondes.json`, sans rien de la réponse. **3 appels réels** (une sonde a été retentée par le client : erreur transitoire).
- `boucle/schemas.py` relâché en conséquence ; les vrais schémas passent désormais **sans aucune perte ni signalement** (test `test_schemas_epingles_sans_perte`), et un test exige un verdict « accepté » pour chaque construction conservée au-delà du noyau.
- Conséquence : la définition d'`eurlex__chercher_texte` du faux serveur change (`additionalProperties: false` et `type: ["string","null"]` conservés), donc toutes les requêtes enregistrées des 5 scénarios. **Réenregistrés : 10 appels réels** (budget de l'étape 4), anciens enregistrements supprimés. Réenregistrement explicite par `BOUCLE_ENREGISTRER=<plafond>` ; sans cette variable, les tests restent en rejeu.
- Observation : sur « modifie le poste du matricule M1 », Mistral lisait d'abord la fiche et s'arrêtait ; il tente maintenant directement `modifier_poste`, reçoit le refus D-010 (forme §6) et la boucle bloque. Le test a été ajusté à ce comportement (`test_refus_par_structured_content`).
- Budget Mistral consommé à ce point : **13 / 25**.

## 2. Ce qui n'est pas fait

Étapes 6 et 7 : en cours.

## 3. Décisions prises seul

1. **`demo/registre*.json` ne sont pas versionnés** (ajoutés au `.gitignore`). Ils portent des chemins absolus propres au worktree et les empreintes des binaires construits localement (`bin/` n'est pas versionné non plus). Ils sont régénérés par `go run ./cmd/registre-demo` (et par le lanceur de l'étape 7). Le plus simple à défaire : retirer deux lignes du `.gitignore`.
2. **gels-avoirs pointe par défaut sur `enregistrements_locaux`** (registre complet, hors Git) dans le registre de démo ; `-gels servers/gels-avoirs/enregistrements` donne le registre synthétique, utilisé par les tests. Registre local absent → le serveur répond `indisponible` (comportement existant), jamais « aucune correspondance ».
3. **`DONNEES_MODE` est transmis aux huit serveurs**, y compris les trois synthétiques : tous passent par `lanceur`, qui lit la variable ; elle est sans effet sur eux.
4. **Conséquence du « fermé par défaut »** : `get_taux_usure`, `rechercher_entreprise`, `verifier_gel`, les outils eurlex et legifrance n'ont pas d'annotation `readOnlyHint` et sont donc classés `action` au registre, alors que ce sont des lectures. Je n'ai pas modifié les serveurs (hors de ma mission). Voir question 1.
5. **L'ancien catalogue écrit à la main est gelé, pas supprimé** : déplacé dans `agents/tests/donnees/catalogue-enregistrements.json`. Les réponses Mistral enregistrées du compilateur (`test_scenarios`, `test_demo`) sont indexées par l'empreinte de la requête, qui contient le catalogue : le remplacer invalidait 8 enregistrements, et les réenregistrer aurait pris ~8 appels réels hors du budget de ce brief (25, répartis sur les étapes 4, 5 et 7). Ces tests gardent donc l'ancien catalogue ; tout le reste passe au catalogue généré.

## 4. Blocages

Aucun pour l'instant.

## 5. Questions pour Manu

1. (moyenne) Ajouter `readOnlyHint: true` aux outils de lecture des serveurs de données officiels ? Aujourd'hui ils sont `action` au registre, ce qui est correct par défaut mais trompeur dans la vue « contrôles en vigueur ».
2. (basse) Réenregistrer les scénarios du compilateur sur le catalogue généré (≈ 8 appels Mistral), puis supprimer `agents/tests/donnees/catalogue-enregistrements.json` ?

## 6. Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-integration
scripts/construire-serveurs.sh
go run ./cmd/registre-demo && sha256sum demo/*.json
go run ./cmd/registre-demo && sha256sum demo/*.json   # mêmes empreintes
go test -race ./internal/registredemo/
(cd agents && python3 -m unittest tests.test_catalogue_demo -v)
```
