# Rapport — session « compilateur-v2 » : du texte du juriste au jeu v2

Branche `feat/compilateur-v2`, worktree `~/cybercar/mcp_rogue-compilateur-v2`.
Rien n'est poussé ni fusionné. `main` n'est pas touchée. `internal/directive`,
le gate et les paquets protégés ne sont pas modifiés.

## Résumé

**Critère de réussite partiellement atteint.** Les étapes 1, 2, 3, 4, 5 et 7
sont faites et testées. L'étape 6 est faite pour **crédit**, **week-end** et
**demandeur** : questions attendues, puis jeu accepté par le chargeur et
relecture stable, le tout rejoué sans appel. Elle est **partielle pour top
management et RH**. Dans ces deux scénarios, les questions et le refus motivé
sont obtenus et enregistrés. La proposition réelle de Mistral est ensuite
refusée à juste titre par le code. La correction n'a pas pu être obtenue :
générations interrompues côté Mistral pour l'un, budget de 30 appels épuisé
pour l'autre.

- `go vet ./...` et `go test -race ./...` sont verts à chaque commit, ainsi que
  les **184 tests Python** (113 avant la session).
- **Mistral : 30 appels réels sur 30.** Les tests rejouent les réponses
  enregistrées et ne font jamais d'appel réel.
- Aucun secret, binaire, `.env` ni donnée réelle de personne physique n'est
  commité. Aucune clé ne figure dans les enregistrements (vérifié par `grep`).

## 1. Ce qui est fait

| Étape | Commit | Objet |
|---|---|---|
| 1 | `cd8cf89` | `internal/relecture` : relecture v2 déterministe, fichiers attendus octet pour octet, trois fuseaux machine |
| 2 | `cd8917a` | `cmd/directive` : `verifier` et `relire` sur les jeux v2, `--exiger-brouillon` conservé |
| 3 | `dafdb6e` | Function calling sans récursion : arbre à plat reconstruit par le code, énumérations tirées du vocabulaire, `directive fuseaux` |
| 4, 5 | `1a37a2b` | Un texte → un jeu v2 complet en brouillon ; refus motivés ; vérifications v2 ; questions fermées du code |
| 6 | `d3fa1d2` | Scénarios crédit et week-end enregistrés et rejoués ; durcissements trouvés en mode réel |
| 6, 7 | `b1a5f00` | Top management (partiel) ; contrôle « source désignée » ; aucune génération en erreur mise en cache ; démo texte `--v2` |
| 6 | `d77df35` | Scénarios demandeur (complet) et RH (partiel) |
| — | `f402647` | Rapport final |
| 7 | `4823844` | Démo en v2 par défaut, `--v1` disponible (décision de Manu) |

### Étape 1 — relecture v2 (`internal/relecture/relecture_v2.go`)

- Une directive v2 est relue en plusieurs blocs :
  - identité et fondements (le premier est indiqué comme celui que citent les
    motifs), puis validité et périmètre ;
  - `Quand :` donne le déclencheur ;
  - `Si …` donne l'arbre, indenté : `et` et `ou` sont annoncés, `non` devient
    une phrase niée quand la feuille le permet, et chaque quantificateur dit
    ce qu'il vaut sur une liste vide ;
  - `Alors :` donne un effet par tiret ;
  - une dernière ligne donne la règle de l'indéterminé à la racine.
- Le fuseau d'une feuille `temps` est toujours nommé. Les dates de
  l'enveloppe et de `source.date` sont écrites en UTC.
- Un jeu purement v1 est relu **octet pour octet comme avant** (les attendus
  v1 et `agents/tests/attendus/d001.txt` sont inchangés).
- Tests :
  - 7 jeux comparés à `internal/relecture/testdata/v2/*.txt` : les 6
    scénarios de `internal/directive/testdata/v2`, lus sans être modifiés,
    et `testdata/variantes-v2.json` ;
  - `time.Local` changé en cours d'exécution ;
  - sous-processus lancés avec `TZ=UTC`, `Asia/Tokyo` et
    `America/Los_Angeles` ;
  - test de couverture : chaque nœud, feuille, opérateur, effet et évènement
    est employé au moins une fois ;
  - aucun gabarit de repli n'apparaît sur un jeu accepté.

### Étape 2 — `cmd/directive`

- Il suffisait de la relecture v2 : le chargeur acceptait déjà les jeux v2.
  `--exiger-brouillon` s'applique aussi aux directives v2.
- Nouvelle sous-commande `directive fuseaux` : elle rend la liste fermée des
  fuseaux du chargeur, dont le compilateur tire son énumération.
- Tests (`cmd/directive/main_v2_test.go`) : les 6 jeux v2 sont vérifiés et
  relus octet pour octet ; `--exiger-brouillon` est testé sur du v2 ; les
  erreurs v2 sont rendues en bloc avec le chemin du nœud ; `fuseaux` est
  couvert.

### Étape 3 — function calling sans récursion

- **`agents/compilateur/arbre.py`.** La condition arrive à plat (`noeuds` :
  une liste de `{id, type, enfants, paramètres}`, plus `racine`). Le code
  reconstruit l'arbre et refuse les cas suivants :
  - identifiant en double, enfant ou racine inconnus ;
  - cycle, nœud à deux parents, nœud orphelin ;
  - mauvais nombre d'enfants, ou enfant d'une feuille ;
  - paramètre non admis ou manquant ;
  - profondeur supérieure à 6, ou plus de 64 nœuds.

  Comme chaque nœud n'a qu'un parent, la reconstruction ne peut pas boucler.
- **`agents/compilateur/outils_v2.py`.** Mistral dispose de trois outils :
  `proposer_jeu`, `poser_question` et `signaler_non_observable`.
  - Aucun `$ref` ni `anyOf`.
  - Énumérations : les 15 types de nœuds, les opérateurs, les effets, les
    jours, les modes d'attestation, les serveurs, les outils, les arguments,
    les étiquettes, les rôles, les groupes et les fuseaux.
  - Absents des outils : statut, version, identifiants, `texte_fr`,
    `date_effet`, `si_indetermine`, `rythme`, `si_echec`, `evenement`.
- **`agents/compilateur/vocabulaire.py`.** Le vocabulaire réunit trois
  sources :
  - le catalogue généré, inchangé ;
  - `exemples/vocabulaire-demo.json` pour les étiquettes, rôles et groupes ;
  - les fuseaux du chargeur Go.

### Étapes 4 et 5 — un texte → un jeu ; questions et refus motivés

- **`compilateur_v2.compiler_jeu`.** Un seul `proposer_jeu` peut porter
  plusieurs directives.
  - Le code les numérote (`<préfixe>-001`…) et les met **toutes en
    brouillon**.
  - Il fixe `date_effet`.
  - `texte_fr` est l'extrait recopié par Mistral s'il figure mot pour mot
    dans le texte, sinon le paragraphe entier.
  - Le jeu passe ensuite par `directive verifier --exiger-brouillon`, puis
    par `directive relire`.
- **États possibles** : `questions`, `non_observable` (refus motivé : extrait,
  raison, et une question demandant un critère observable), `jeu`, `refuse`,
  `erreurs`. Mistral a droit à une seule correction.
- **Questions du code.** Elles sont fermées, consignées, et jamais envoyées
  à Mistral :
  - `si_indetermine`, une seule question pour le jeu ;
  - pour chaque notification, son `rythme` et son `si_echec`.
- **`verifications_v2.py` vérifie que rien n'est inventé** :
  - montants, dates, heures et jours (« week-end » donne samedi et
    dimanche) ;
  - fuseau : son nom IANA ou sa ville doit être écrit ;
  - étiquettes, rôles et groupes : **forme exacte** exigée (« données
    client » ne justifie pas `donnees_client`) ;
  - quatre yeux, fondements, et article omis alors qu'il est écrit ;
  - argument présent pour **chaque** outil visé ;
  - `acces` qui ne restreint rien ;
  - `critere: transmis` qui n'est pas écrit ;
  - source de `a_consulte` non désignée par le juriste (condition ajoutée).
- **Client Mistral (`mistral.py`).** Une génération interrompue
  (`finish_reason: error`) n'est plus mise en cache comme une réponse : elle
  compte comme une erreur transitoire et provoque une nouvelle tentative.

### Étape 6 — scénarios enregistrés

`agents/compilateur/scenarios_v2.py` déroule un scénario
(`exemples/scenarios-v2/<nom>.json`). Les réponses aux questions de Mistral
y sont retrouvées par le texte de la question, celles du code par leur champ.
Tests : `agents/tests/test_scenarios_v2.py`, avec les relectures attendues
dans `agents/tests/attendus/v2/`.

| Scénario | Questions obtenues | Résultat |
|---|---|---|
| Crédit (un texte → 4 directives) | seuil, taux officiel, recherche d'entreprises ; `si_indetermine` | **Jeu accepté.** Contenu : `non(a_consulte gels-avoirs/verifier_gel)` → refus ; `/montant > 500000` → refus ; `a_consulte taux-officiel` → attente ; caviardage `/siren` vers `recherche-entreprises` ; articles recopiés |
| Week-end | consultation ou production ; puis, après refus du code, **fondement** et **fuseau** ; `si_indetermine` | **Jeu accepté**, identique à `internal/directive/testdata/v2/weekend.json` (déclencheur, condition, effets, fondements) |
| Demandeur (groupe X) | groupe ; puis, après refus du code, **étiquette de « rapport »**, **étiquette de « données client »** et fondement ; `si_indetermine` | **Jeu accepté** : cible `production_rapport`, `et(demandeur prestataires-externes, a_consulte étiquette donnees_client)` → refus |
| Top management | **refus motivé** (« impliquant le top management » n'est pas observable) et demande d'un critère | **Partiel.** Mistral propose la bonne cible, `> 2 M€`, `directeur-risques`, quatre yeux et notification, mais **ajoute une condition inventée**. Le code la refuse. La correction a échoué : Mistral a rendu `finish_reason: error` aux 3 tentatives |
| RH (groupes X et Y) | groupe X, groupe Y | **Partiel.** La proposition est juste sur le fond (une directive par outil : `existe /salarie_ids`, `sujet /salarie_id`). Le code la refuse pour un `acces` superflu et un article omis. La correction aurait demandé un 31ᵉ appel |

### Étape 7 — démo texte

La commande est la suivante :

```sh
python -m compilateur.demo --jeu <id> --catalogue compilateur/exemples/catalogue-demo.json \
    --auteur … --sortie jeu.json [--texte …] [--date-effet …] [--prefixe-id W] [--mode reel] [--v1]
```

**Le v2 est le parcours par défaut** (décision de Manu). `--v1` garde
l'ancien parcours (une directive à verrou unique). `--v2` reste accepté, et
les deux options sont exclusives.

Déroulé : texte → refus motivés et questions de Mistral → questions fermées
du code (une réponse hors options est redemandée) → relecture v2 côte à côte
→ jeu en brouillon. La consignation est écrite dans
`<sortie>.reponses.json`. Tests (`test_demo_v2.py`) :
- parcours week-end rejoué de bout en bout ;
- v2 par défaut sans option ;
- `--v1` et `--v2` exclusifs.

`agents/tests/test_demo.py` passe désormais `--v1`.

## 2. Ce qui n'est pas fait, et pourquoi

- **Top management et RH ne vont pas jusqu'au jeu accepté.** Pour top
  management, Mistral a interrompu sa génération à 3 reprises sur la requête
  de correction. Pour RH, le budget de 30 appels était épuisé. Pour finir, il
  faut **environ 2 appels pour RH** (la correction) et **2 à 3 pour top
  management** : `python -m compilateur.scenarios_v2 rh --mode reel`, puis
  `top-management`, puis ajouter les jeux attendus à `test_scenarios_v2.py`.
- **« Ne pas discriminer » n'est pas enregistré.** Le seul appel a été une
  génération interrompue côté Mistral. Le refus motivé est couvert par top
  management en réel et par un test scripté. Le scénario
  `exemples/scenarios-v2/discrimination.json` est prêt, sans enregistrement.
- **Question « l'utilisateur : demandeur ou personne concernée ? »** :
  Mistral ne l'a pas posée pour le demandeur. « pour lui » l'a peut-être
  levée. La consigne la demande. Aucun contrôle de code ne peut l'imposer.
- **Seuil non chiffré** : il n'y a pas de scénario v2 réel. Le contrôle des
  montants le refuse (test scripté), et un enregistrement v1 existe.

## 3. Décisions prises seul

1. **Relecture v1 inchangée** pour un jeu purement v1 : d'autres tests en
   dépendent. La phrase sur le cumul des notifications et des marques
   n'apparaît que si le jeu contient une directive v2.
2. **Règle de l'indéterminé** : elle n'est écrite que si quelque chose peut
   être indéterminé (condition, ou cible portant sur `etiquette` ou
   `acces`).
3. **Étiquettes, rôles et groupes viennent d'un complément écrit à la main**
   (`agents/compilateur/exemples/vocabulaire-demo.json`). Ni le registre de
   démo ni le catalogue généré ne les portent, et `cmd/registre-demo` est
   hors de mon périmètre. Les groupes RH sont ceux de
   `servers/referentiel-rh`, les autres noms ceux des scénarios v2.
4. **Les fuseaux sont tirés du Go** (`directive fuseaux`), plutôt que d'une
   copie en Python.
5. **Seul `appel_outil` est offert à Mistral** : les autres évènements ne
   sont pas évalués par l'engine.
6. **`texte_fr`** est l'extrait recopié mot pour mot, sinon le paragraphe
   entier. Jamais une reformulation.
7. **`si_indetermine` fait l'objet d'une seule question pour le jeu**, qui
   liste les directives concernées. Si la réponse est `ignorer`, le juriste
   est nommé dans chacune.
8. **Rythme et conduite en cas d'échec d'une notification** : ce sont des
   questions du code. Ces paramètres sont obligatoires, et le texte les fixe
   rarement.
9. **Un nombre écrit en chaîne** (`"500000"`) est relu comme un nombre, et
   un flottant entier comme un entier : même valeur, autre écriture.
10. **Les contrôles fermés par défaut sont ajoutés après les essais réels** :
    `critere: transmis` seulement s'il est écrit (sinon `donnees_recues`) ;
    `acces` refusé s'il ne restreint rien (il ajouterait un indéterminé) ;
    source de `a_consulte` désignée par le juriste.
11. **La démo est passée en v2 par défaut** sur décision de Manu, et `--v1`
    reste disponible. Seul fichier existant modifié dans `agents/tests` :
    `test_demo.py`, qui passe `--v1`. `agents/README.md` n'est pas modifié.
12. **J'ai écrit moi-même les textes des scénarios.** Le dépôt ne contient ni
    « la politique de crédit complète » ni « la règle RH des groupes X et
    Y » : je les ai rédigés d'après `docs/orchestration/brief_v2.md` et les
    jeux de référence v2.
13. **Les enregistrements devenus inutiles** après chaque changement de
    consigne ont été déplacés hors du dépôt, et non commités. Chaque
    enregistrement versionné est rejoué par un test.

## 4. Blocages rencontrés

- **Générations interrompues côté Mistral** (`finish_reason: error`, sans
  contenu) : 6 appels sur 30 en tout, dont 3 sur la même requête. Le client
  les mettait en cache : c'est corrigé et testé
  (`test_mistral_fin_en_erreur.py`).
- **Le budget de 30 appels est épuisé.** Environ 10 appels ont servi à mettre
  au point la consigne (questions inutiles, valeurs non écrites).
- **Fiabilité de `mistral-small`** sur ce schéma : questions déjà répondues
  reposées, une proposition hallucinée (trois directives inventées), une
  condition ajoutée. Le code a refusé chacun de ces cas.

## 5. Questions pour Manu — réponses reçues

1. **Budget pour finir RH et top management** : aucun appel maintenant. Une
   autre session s'en chargera après la fusion (commandes prêtes, voir §2).
2. **Vocabulaire généré depuis le registre et l'annuaire** : oui, mais après
   la fusion, car il dépend de fichiers d'une autre branche. Je n'y ai pas
   touché.
3. **Démo en v2 par défaut** : fait (`4823844`). `--v1` reste disponible, et
   `test_demo.py` est adapté.
4. **Choix du modèle** (`mistral-small` ou `mistral-medium`) : après la
   fusion, sur mesures.
5. **`si_indetermine`** : une seule question par jeu, gardé tel quel.

Plus aucune question ouverte.

## 6. Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-compilateur-v2
go vet ./... && go test -race -count=1 ./internal/relecture ./cmd/directive
(cd agents && python3 -m unittest discover -s tests -t .)          # 184 tests, rejeu, aucun appel
go run ./cmd/directive relire < internal/directive/testdata/v2/rh.json
cd agents && python3 -m compilateur.scenarios_v2 credit             # déroulé rejoué + jeu + relecture
python3 -m compilateur.scenarios_v2 weekend
python3 -m compilateur.scenarios_v2 top-management   # lève EnregistrementAbsent sur la correction, jamais obtenue
```
