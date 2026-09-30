# Rapport — session « Intégration » : premier passage de bout en bout

Branche `feat/integration`, worktree `~/cybercar/mcp_rogue-integration`. Rien n'a été poussé ni fusionné, `main` n'a pas été touchée. Ce rapport est aussi versé dans `docs/rapports/integration.md`.

## Résumé

**Critère de réussite atteint.**

- **Étape 6 verte** : `go test -race ./test/integration/` fait tourner le binaire `mcp-gate` devant les **vrais binaires** des huit serveurs (REJEU, registre des gels synthétique, `credit-demo.json`) ; les quatre scénarios passent (nominal → en attente par D-001, seuil → refus D-002, attaque → imposteur jamais lancé et refus D-003, base périmée → consultation journalisée) ; chaque journal relu depuis le disque se vérifie et se rejoue sans écart.
- **Un passage réel consigné** (étape 7, section dédiée ci-dessous) : Mistral devant le gate en MIXTE sur « Peut-on accorder le découvert demandé dans le dossier DOS-FICTIF-002 ? », 5 outils appelés, tous autorisés, pas de tentative d'attribution ; journal vérifié et rejoué sans écart.
- **Budget Mistral : 19 appels réels sur 25** (sondes 3, réenregistrement de la boucle 10, passage réel 6).
- `go vet ./...`, `go test -race ./...` et les 113 tests Python verts à chaque commit. Aucun secret, binaire, `.env` ni donnée réelle de personne physique commité ; le registre national des gels téléchargé reste hors Git. `internal/directive` (et les autres paquets protégés) non modifiés.

## 1. Ce qui est fait

| Étape | Commit | Objet |
|---|---|---|
| 1 | `529a0c3` | `cmd/registre-demo` + `internal/registredemo` : `demo/registre.json` et `demo/registre-attaque.json` générés depuis `bin/manifeste.json` |
| 2 | `87f4166` | Catalogue du compilateur généré depuis le registre (`agents/compilateur/exemples/catalogue-demo.json`), tests de divergence Go et Python |
| 3 | `b1432c4` | `demo/directives/credit-demo.json` (v1, quatre directives actives), accepté par le gate devant le registre de démo |
| 4 | `352253e` | `agents/boucle` alignée sur le §6 du contrat de l'engine (refus, attente, indisponible, outil qui décline), `MAX_APPELS` = 25 |
| 5 | `4d695c1` | Sondes Mistral (`boucle.sondes`), conversion relâchée (`additionalProperties` booléen, `type` `[T, "null"]`), 5 scénarios de la boucle réenregistrés |
| 6 | `565c65c` | `test/integration/` : gate réel devant les vrais binaires, quatre scénarios, journaux vérifiés et rejoués sans écart |
| 7a | `e2250b9` | `scripts/lancer-demo.sh` : préparation, vérification du démarrage, sous-commande `gate` lancée par la boucle |
| 7b | `e99e25a` | Le MIXTE de la démo n'écrit plus dans les enregistrements versionnés (`registre-demo -enregistrements`, copie de travail, `boucle --enregistrements`) ; passage réel consigné |
| — | (ce commit) | Rapport final, versé dans `docs/rapports/integration.md` |

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

### Étape 6 — test de bout en bout, sans réseau ni Mistral (**vert**)

`go test -race ./test/integration/` (≈ 6 s) :

- `TestMain` construit les neuf serveurs et `cmd/mcp-gate` dans un répertoire temporaire, écrit le manifeste qui les décrit et génère les deux registres par `registredemo.Generer` (gels-avoirs sur le registre **synthétique**). Rien n'est écrit dans `bin/` ni `demo/`.
- Chaque scénario lance le **binaire** `mcp-gate` en stdio, comme un hôte MCP, avec `demo/directives/credit-demo.json`, un environnement réduit à la clé du journal et `DONNEES_MODE=REJEU` (aucun secret PISTE). Le client MCP du SDK Go joue l'agent.
- `TestDemarrage` : 13 outils exposés, les huit serveurs attestés (empreinte du binaire et schémas épinglés).
- **1. Nominal** : `rechercher_entreprise` avec `raison_sociale`, `dirigeant`, `note` en trop → caviardé (note de l'engine, empreintes d'origine et transmise distinctes, `champs_retires` = `/dirigeant,/note,/raison_sociale`) → `verifier_gel` → `get_taux_usure` au 2026-10-04 → `"trimestre":"2026-Q4"`, source `rejeu` → `attribuer_pret` DOS-FICTIF-002 400 000 € → **en attente** (`statut: en_attente`, D-001 v1, `empreinte_transmise` = celle du journal, aucune transmission, état de session à trois consultations).
- **2. Seuil** : `attribuer_pret` 900 000 € → **refusé**, `origine: directive`, D-002 v1, fondement « RGPD — à valider par le juriste » art. 22 ; le motif ne cite pas le montant ; aucune transmission.
- **3. Attaque** (`registre-attaque.json`) : attestation de `taux-officiel` invalide, `observe` = empreinte de l'imposteur, détail « processus non lancé » ; **ni `taux-imposteur:` ni `taux-officiel: mode` sur stderr** (chaque serveur s'y annonce au démarrage ; la banque, elle, y apparaît) ; l'agent voit l'outil épinglé ; `get_taux_usure` → **refusé par D-003** (`source_attestee`, fondement cité, origine directive et non registre, conformément au §5) ; les autres serveurs restent joignables.
- **4. Base périmée** : `get_taux_interne` → `"trimestre":"2026-Q3"` ; l'entrée `appel` (statut ok, empreinte de réponse) et son entrée `transmission` sont au journal ; l'`attribuer_pret` qui suit est **autorisé et transmis** (aucune directive ne vise la base interne) et son instantané de session montre la consultation `banque-synthetique/get_taux_interne`.
- Pour chaque scénario : journal relu depuis le disque, **chaîne HMAC vérifiée**, **rejeu sans écart** de chaque appel (effet rejoué = effet journalisé), aucune transmission sans appel.
- Stable sur 3 exécutions consécutives (`-count=3`).

### Étape 7 — lanceur de démo

- `scripts/lancer-demo.sh` : construit les serveurs (`construire-serveurs.sh`) et `bin/mcp-gate`, génère `demo/registre.json`, vérifie le registre local des gels (sinon `gels-avoirs preparer`), puis **vérifie que le gate démarre** avec ce registre et `credit-demo.json` (REJEU, journal jetable, 8/8 attestés exigés) et affiche la commande de la boucle.
- `scripts/lancer-demo.sh gate` démarre `mcp-gate` avec les serveurs en `DONNEES_MODE=MIXTE`. Le gate est un serveur MCP stdio : c'est la boucle qui le lance (`python3 -m boucle … -- ../scripts/lancer-demo.sh gate`). Environnement **réduit** par `env -i` à la clé du journal, `DONNEES_MODE` et aux identifiants PISTE : `MISTRAL_API_KEY` ne passe pas au gate. Journal, coffre et traces dans `demo/execution/` (ignoré par Git).
- **Correction après le passage réel** : en MIXTE, un serveur de données écrit chaque réponse obtenue dans son répertoire d'enregistrements. Le passage réel a ainsi **réécrit deux enregistrements versionnés** (`recherche-entreprises` BIOCOOP et un seuil de `taux-officiel`), ce qui a fait échouer `servers/taux-officiel.TestInstantaneVersionne` au contrôle final. Je les ai restaurés (`git checkout`, rien de ce genre n'a été commité) et corrigé la cause : `registre-demo -enregistrements <dir>` fait pointer les serveurs de données vers `<dir>/<serveur>`, et le lanceur y place une copie de travail (`demo/execution/enregistrements/`, ignorée par Git). De même, la boucle a gagné `--enregistrements` : la commande affichée garde les réponses Mistral de la démo dans `demo/execution/mistral/`, pas parmi celles que rejouent les tests. Vérifié sans Mistral : client MCP direct devant `lancer-demo.sh gate`, `get_taux_usure` et `rechercher_entreprise` servis en `direct` (le second caviardé), `git status` propre ensuite.
- Premier lancement : registre national des gels téléchargé (publication du 2026-09-29, 6 611 entrées) dans `servers/gels-avoirs/enregistrements_locaux/` (ignoré par Git).

### Étape 7 — passage réel (observation, pas un test)

Lancement (avant la correction ci-dessus, d'où l'écriture dans les enregistrements versionnés, restaurés depuis) : `scripts/lancer-demo.sh`, puis la boucle Mistral (un seul agent, les 13 outils, sous-agent `credit`, `mistral-small-latest`, température 0) devant `scripts/lancer-demo.sh gate` (`DONNEES_MODE=MIXTE`, clé du journal éphémère exportée pour le passage et la vérification). Demande : « Peut-on accorder le découvert demandé dans le dossier DOS-FICTIF-002 ? ». Aucune conversion de schéma signalée, aucun outil exclu.

**6 appels Mistral** (6 tours ; plafond technique posé à 10), fin `termine`.

| Seq | Outil appelé | Arguments | Décision du gate | Résultat | Source déclarée |
|---|---|---|---|---|---|
| 1 | `banque-synthetique__evaluer_dossier` | `dossier_id: DOS-FICTIF-002` | autoriser (aucune directive applicable) | ok | non déclarée |
| 2 | `banque-synthetique__get_taux_interne` | `categorie: decouvert-societes-non-financieres` | autoriser | ok | non déclarée |
| 3 | `taux-officiel__get_taux_usure` | `categorie: decouvert-societes-non-financieres` (sans date : aujourd'hui) | autoriser | ok | **direct** (API Webstat) |
| 4 | `recherche-entreprises__rechercher_entreprise` | `siren: 382891752` (rien à caviarder) | autoriser | ok | **direct** |
| 5 | `gels-avoirs__verifier_gel` | `nom: BIOCOOP` | autoriser | ok | **direct** |

Journal (`demo/execution/journal.jsonl`, 22 entrées) : **chaîne vérifiée, 5 appels rejoués, 0 écart, 0 transmission sans appel** ; les huit serveurs attestés au démarrage. Les 6 réponses Mistral de ce passage, que la boucle avait mises en cache parmi les enregistrements des tests, n'ont pas été versionnées (inutiles aux tests, données en direct).

Message final du modèle (tel quel) :

> Le dossier **DOS-FICTIF-002** (BIOCOOP, SIREN 382891752) demande un découvert de **400 000 €** sur **12 mois** avec un taux proposé de **19,05 %**.
>
> ### Analyse des contraintes :
> 1. **Seuil de l'usure** (Banque de France, Q3 2026) :
>    - **Seuil officiel** : **19 %** (en vigueur jusqu'au 30/09/2026).
>    - **Seuil suivant** (Q4 2026) : **19,08 %** (à partir du 01/10/2026).
>    - **Taux proposé (19,05 %)** dépasse le seuil actuel (**19 %**), mais serait conforme au seuil du trimestre suivant (**19,08 %**).
>
> 2. **Vérifications réalisées** :
>    - **Identité de l'entreprise** : BIOCOOP (SIREN 382891752) est active et valide (forme juridique : SA coopérative).
>    - **Registre des gels** : Aucune correspondance trouvée pour "BIOCOOP".
>
> ### Conclusion :
> Le taux proposé (**19,05 %**) **dépasse le seuil légal en vigueur (19 %)** pour le trimestre en cours. Il serait conforme **uniquement à partir du 01/10/2026** (seuil à 19,08 %).
>
> **Recommandation** :
> - **Attendre le 01/10/2026** pour attribuer le découvert, **ou**
> - **Négocier un taux ≤ 19 %** pour une attribution immédiate (mais cela réduirait la marge bénéficiaire).
>
> Souhaitez-vous que je simule l'attribution du prêt avec un taux ajusté à **19 %** (montant maximal possible) ou que j'attende la mise à jour du seuil ?

Observations :

- Le modèle a fait les trois contrôles que demande la note de l'analyste (identité, gels, usure), dans un ordre raisonnable, un outil à la fois, et **n'a pas tenté `attribuer_pret`** : D-001 (attente après consultation de `taux-officiel`) n'a donc pas été exercée en réel — elle l'est par le scénario 1 de l'étape 6.
- Il a interrogé **à la fois** la base interne périmée et le taux officiel, et sa conclusion s'appuie sur le taux officiel (source `direct`). Le seuil T3 qu'il cite (19 %) n'a pas été recoupé par moi ; il vient de la réponse de l'API, dont l'empreinte est au journal.
- Sans `date_reference`, `get_taux_usure` rend le seuil du jour (30/09, dernier jour du T3) et signale le T4 publié : le modèle en tire une conclusion datée correcte dans sa forme (« conforme à partir du 01/10 »). La démo du 04/10 devra fixer la date, ou le résultat changera.
- Le message final se termine par une question au demandeur : la boucle n'a pas d'humain dans la boucle, elle s'arrête là (`termine`).
- Aucun refus, aucune attente : ce passage montre le chemin nominal en lecture, pas les garde-fous.

## 2. Ce qui n'est pas fait

- Rien dans le brief. Hors brief, et volontairement :
  - la **reprise d'un appel en attente avec jeton** n'existe toujours pas côté gate (§6 du contrat) : la boucle rend la main avec `empreinte_transmise` et s'arrête ;
  - les **scénarios enregistrés du compilateur** tournent toujours sur l'ancien catalogue gelé (décision 5, question 5) ;
  - **aucun outil de vérification de journal en ligne de commande** n'est livré : pour le passage réel, j'ai vérifié chaîne et rejeu avec un programme jetable (`journal.Verifier` + `rejeu.RejouerJournal`), non commité. Voir question 4.

## 3. Décisions prises seul

1. **`demo/registre*.json` ne sont pas versionnés** (`.gitignore`). Ils portent des chemins absolus propres au worktree et les empreintes des binaires construits localement (`bin/` n'est pas versionné non plus) ; `go run ./cmd/registre-demo` et le lanceur les régénèrent. Le plus simple à défaire : retirer deux lignes du `.gitignore`.
2. **gels-avoirs pointe par défaut sur `enregistrements_locaux`** (registre complet, hors Git) dans le registre de démo ; `-gels servers/gels-avoirs/enregistrements` donne le registre synthétique, utilisé par les tests. Registre local absent → le serveur répond `indisponible` (comportement existant).
3. **`DONNEES_MODE` est transmis aux huit serveurs**, y compris les trois synthétiques : tous passent par `lanceur`, qui lit la variable ; elle est sans effet sur eux.
4. **Fermé par défaut, sans toucher aux serveurs** : `get_taux_usure`, `rechercher_entreprise`, `verifier_gel`, les outils eurlex et legifrance n'ont pas d'annotation `readOnlyHint` et sont classés `action` au registre, bien que ce soient des lectures. Voir question 2.
5. **L'ancien catalogue écrit à la main est gelé, pas supprimé** : déplacé dans `agents/tests/donnees/catalogue-enregistrements.json`. Les réponses Mistral enregistrées du compilateur (`test_scenarios`, `test_demo`) sont indexées par l'empreinte de la requête, qui contient le catalogue : le remplacer invalidait 8 enregistrements, et les réenregistrer aurait pris ~8 appels hors du budget de ce brief. Ces tests gardent l'ancien catalogue ; tout le reste (CLI, README, `test_cli`) passe au catalogue généré.
6. **Fondements « à valider par le juriste » écrits dans `fondement.texte`** : le schéma v1 n'a pas de champ de note et le brief interdit de le modifier.
7. **Réenregistrement de la boucle après l'étape 5 plutôt qu'à l'étape 4** : l'étape 4 ne changeait aucune requête (vérifié : les scénarios rejouaient tels quels) ; l'étape 5 les changeait toutes. Un seul réenregistrement, 10 appels, le budget de l'étape 4. Un test de scénario a été ajusté au nouveau comportement observé du modèle (étape 5), pas l'inverse.
8. **La boucle ne lit le `statut` de l'engine que sur un résultat `isError`**, conformément au §6 (une réponse normale n'a jamais de `structuredContent` en clair) : un serveur appelé sans gate ne peut pas simuler un refus ou une attente.
9. **Scénario 4 : l'attribution après la base périmée est autorisée**, et le test l'affirme : c'est ce que fait le jeu v1 (D-001 ne vise que `taux-officiel`). Le test documente le trou plutôt que de le masquer. Voir question 1.
10. **Le lanceur exige `MCP_GATE_CLE_JOURNAL` dans l'environnement** et n'en génère jamais : une clé générée et perdue rendrait le journal invérifiable, une clé écrite dans un fichier contredirait la règle des secrets.
11. **`scripts/lancer-demo.sh gate` réduit l'environnement du gate par `env -i`** : la boucle lance le gate en sous-processus et lui passerait sinon tout son environnement, `MISTRAL_API_KEY` compris.
12. **Téléchargement du registre national des gels** (`gels-avoirs preparer`, DG Trésor) au premier lancement, comme le brief le demande : 6 611 entrées, dont des personnes physiques, stockées hors Git ; le serveur ne rend jamais le nom d'une personne physique.
13. **Les réponses du passage réel ne sont pas versionnées** : ni les 6 réponses Mistral (aucun test ne les rejoue), ni les réponses en direct des API officielles, qui restent dans `demo/execution/` (ignoré par Git).

## 4. Blocages

Aucun bloquant.

- Une sonde Mistral a subi une erreur transitoire et a été retentée par le client (3 appels pour 2 sondes).
- Le passage réel en MIXTE a réécrit deux enregistrements versionnés (détecté par la suite de tests au contrôle final, restauré, cause corrigée : voir étape 7).
- Un programme de vérification de journal placé hors du module ne compile pas (règle `internal/` de Go) : je l'ai construit depuis un dossier temporaire du worktree, supprimé aussitôt, jamais commité.

## 5. Questions pour Manu

Par urgence décroissante :

1. **(haute, pour la v2 de ce soir)** Un prêt fondé sur la base interne périmée passe sans validation (scénario 4, et le modèle réel consulte bien cette base) : directive de provenance sur `banque-synthetique/get_taux_interne`, ou verrou v2 sur la fraîcheur de la donnée ?
2. **(moyenne)** Ajouter `readOnlyHint: true` aux outils de lecture des serveurs de données officiels ? Ils sont `action` au registre : correct par défaut, trompeur dans la vue « contrôles en vigueur ».
3. **(moyenne)** Pour la démo du 04/10, faut-il imposer une date de référence (consigne de la boucle, ou argument fixé) ? Sans date, `get_taux_usure` sert le seuil du jour et la conclusion du modèle change avec le trimestre.
4. **(basse)** Livrer une commande `journal verifier|rejouer` (chaîne + rejeu d'un fichier) ? Utile pour montrer le journal d'une démo sans passer par un test.
5. **(basse)** Réenregistrer les scénarios du compilateur sur le catalogue généré (≈ 8 appels Mistral), puis supprimer le catalogue gelé ?

## 6. Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-integration
go test -race -count=1 -v ./test/integration/       # critère de réussite : 5 tests verts, ~10 s
go vet ./... && go test -race ./...                 # tout le Go
(cd agents && python3 -m unittest discover -s tests -t .)   # 113 tests Python, rejeu, aucun réseau

scripts/lancer-demo.sh                              # prépare ; affiche « 8 serveur(s) attesté(s) sur 8 »
sha256sum demo/*.json; go run ./cmd/registre-demo; sha256sum demo/*.json   # mêmes empreintes
cat agents/boucle/sondes.json                       # verdicts des sondes Mistral
cat agents/.etat/boucle/budget.json                 # 19 appels réels
```

Pour rejouer un passage réel (≤ 10 appels Mistral) :

```sh
export MCP_GATE_CLE_JOURNAL=$(openssl rand -hex 32)
cd agents && python3 -m boucle --sous-agent credit --mode reel --enregistrements ../demo/execution/mistral \
  --demande "Peut-on accorder le découvert demandé dans le dossier DOS-FICTIF-002 ?" \
  -- ../scripts/lancer-demo.sh gate
git status --short     # aucun fichier versionné modifié
```
