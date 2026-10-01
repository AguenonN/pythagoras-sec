# Rapport — session « fusion »

Branche `feat/fusion`, worktree `~/cybercar/mcp_rogue-fusion`, partie de
`782ff37` (`main`). Rien n'est poussé ; `main` et les worktrees des autres
branches ne sont pas touchés. Aucun appel Mistral, aucun réseau.

Précisions de Manu, qui priment sur le brief :
- `docs/demo/maquettes/` (arrivées par `feat/ui`) : non touchées ;
- `decision.duree_us` (gate-v2) est une mesure : vérifier sur les journaux
  régénérés qu'il ne fait jamais diverger le rejeu ;
- rien n'est retiré du format v1.

## Résumé

**Critère de réussite atteint.** Les quatre branches sont fusionnées dans
`feat/fusion` (correctifs `2f25f3f`, gate-v2 `fe81c1d`, compilateur-v2
`3315062`, ui `b7b78a5`). La suite est verte : `go vet`, `go test -race
-count=1 ./...` et 192 tests Python. L'intégration passe trois fois de suite
(`go test -count=3 ./test/integration/`, faite deux fois). Les fichiers
dérivés sont reproductibles : deux exécutions donnent les mêmes octets. La
liste de ce que la console n'affiche pas encore en v2 est en fin de rapport.

Réserve : le commit de fusion de compilateur-v2 (`3315062`) a été créé
automatiquement par Git, sans conflit textuel, avant que les tests aient
tourné. Il est donc rouge à lui seul. Il est corrigé par les deux commits
qui le suivent (voir fusion 3).

Contrôle après chaque fusion : `go vet ./...`, `go test -race -count=1 ./...`
(intégration comprise), `(cd agents && python3 -m unittest discover -s tests -t .)`.

## Fusion 1 — `feat/correctifs`

- Commit de fusion `2f25f3f`.
- **Conflits : aucun** (la branche part de `782ff37`, comme `feat/fusion`).
- Fichier dérivé : `scripts/construire-serveurs.sh` puis
  `go run ./cmd/registre-demo -gels servers/gels-avoirs/enregistrements -catalogue agents/compilateur/exemples/catalogue-demo.json` :
  catalogue **identique** à celui de la branche (aucune modification).
- Tests : vet vert, Go vert (race, intégration comprise), Python 113 OK.

## Fusion 2 — `feat/gate-v2`

- Commit de fusion `fe81c1d`. Commit séparé `52001db` (test ajouté).
- **Conflit : `test/integration/integration_test.go`** (seul fichier). Les
  deux branches avaient introduit chacune un `demarrerAvec` et un chargeur
  de jeu par chemin :
  - correctifs : `demarrerAvec(t, cheminReg, directives string)`, champ
    `banc.directives`, `chargerJeu(t, directives)` ;
  - gate-v2 : `demarrerAvec(t, lancement{registre, jeu, depart, annuaire,
    demandeur, ecran, precedents})`, champ `banc.jeu`,
    `chargerJeuChemin(t, chemin)`, champ `banc.notifications`.

  Résolution : la forme de gate-v2 est un sur-ensemble (registre + jeu +
  le reste) ; elle est gardée telle quelle, avec un commentaire de doc.
  Le champ `banc.directives` de correctifs fait doublon avec `banc.jeu`
  (même valeur, même usage : relire le jeu au rejeu) : seul `jeu` est
  gardé. `chargerJeu(t, directives)` = `chargerJeuChemin(t, chemin)` :
  seul le second est gardé. L'unique appelant de la forme correctifs,
  `base_perimee_v2_test.go`, devient
  `demarrerAvec(t, lancement{registre: cheminRegistre, jeu: directives})` ;
  comportement identique (même registre, même jeu, rejeu sur ce jeu).
  Le serveur fictif `rapports`, l'annuaire, l'horloge simulée et les
  scénarios v2 de gate-v2 sont gardés intégralement.
- **`TestDemarrage`** : 14 outils (valeur de correctifs ; gate-v2 en
  attendait 13). Compté sur ce que le gate réel expose devant
  `demo/registre.json` régénéré : le test passe à 14 (13 + `remettre_rapport`).
- Fichiers dérivés : serveurs reconstruits, `registre-demo` relancé ;
  catalogue **inchangé** (gate-v2 n'y ajoute rien : les étiquettes ne sont
  pas dans le catalogue). `demo/registre.json` régénéré :
  `banque-synthetique/remettre_rapport` porte `production_rapport`,
  `evaluer_dossier` porte `donnees_client`.
- **`demo/directives/rapport.json` se charge désormais** devant le
  registre de démo. Contrôle ajouté (`52001db`) :
  `internal/registredemo.TestTousLesJeuxDemo` charge **tous** les jeux de
  `demo/directives/` (chargeur strict + démarrage du gate, contrôle des
  cibles v1 et v2) devant le registre des schémas versionnés étiqueté par
  `demo/etiquettes.json`, et aussi devant `demo/registre.json` s'il existe
  (non versionné, donc facultatif ; le test le signale par un `t.Log`
  quand il manque). Il vérifie aussi les deux étiquettes ci-dessus.
  Vérifié qu'il échoue si l'étiquette `production_rapport` disparaît.
- Tests : vet vert, Go vert (race, intégration comprise, dont les
  scénarios week-end, top management, RH, demandeur/rapport), Python 113 OK.

## Fusion 3 — `feat/compilateur-v2`

- Commit de fusion `3315062`. Commits de correction séparés `07b423c`
  (Go) et `d40da1d` (Python).
- **Conflits textuels : aucun.** Git a donc créé le commit de fusion
  automatiquement, **avant** que la suite n'ait tourné : ce commit est
  rouge (6 erreurs Python, 4 échecs Go), corrigé par les deux commits
  suivants. Rien n'a été fusionné ensuite avant le retour au vert. Pour la
  fusion 4 : `--no-commit`, tests, puis commit.
- Fichiers dérivés : serveurs reconstruits, `registre-demo` relancé ;
  catalogue **inchangé** par rapport à la fusion 2 (compilateur-v2 ne
  le modifie pas).
- **Conflit sémantique Go, `internal/relecture`** (4 tests rouges) : le jeu
  de test `internal/relecture/testdata/variantes-v2.json` (compilateur-v2)
  contient W-003, évènement `attestation_echouee` avec `notifier
  si_echec: refuser`. Le chargeur de gate-v2 restreint désormais les
  effets par évènement (décision 28 de gate-v2, acceptée par Manu) :
  « refuser n'a pas d'objet pour l'évènement attestation_echouee ». Le
  chargeur fait foi ; la variante passe en `si_echec: continuer` et
  l'attendu `testdata/v2/variantes.txt` est réécrit par `-update`. Diff
  vérifié : seules l'empreinte du jeu et la ligne de W-003 changent
  (« l'évènement est refusé » → « l'évènement suit son cours »).
  `TestJeuV2_Couverture` reste vert (l'issue `refuser` reste couverte
  ailleurs).
- **Enregistrements Mistral (Python, 6 erreurs `EnregistrementAbsent`)** :
  la requête embarque le catalogue, qui a changé depuis l'enregistrement
  (6 accès `action` → `read` et `remettre_rapport`, venus de correctifs).
  Aucun appel Mistral. Selon le brief : copie du catalogue de
  l'enregistrement (`git show feat/compilateur-v2:agents/compilateur/exemples/catalogue-demo.json`,
  identique à celui de `main`) dans
  `agents/tests/donnees/catalogue-v2-enregistrements.json`, constante
  `CATALOGUE_V2_ENREGISTRE` dans `agents/tests/aides.py` ;
  `test_scenarios_v2.py` déroule tous ses scénarios sur un vocabulaire bâti
  sur cette copie (`derouler_gele`) et `test_demo_v2.py` passe cette copie
  au seul parcours week-end rejoué. Le reste (CLI, démo, autres tests)
  garde le catalogue généré. 192 tests Python OK.
- **Contrôle `internal/relecture` sur `$racine` et les évènements** :
  sonde (jeu des évènements de `internal/directive/evenement_test.go` —
  `ouverture_session`, `attestation_echouee`, `purge` — plus la directive
  `$racine/categorie` sous `existe` de `racine_test.go`), passée à
  `go run ./cmd/directive verifier` (ok) puis `relire` : **aucun gabarit
  de repli**. Mais deux défauts de sens, **signalés, non corrigés** (voir
  « Écarts ») :
  1. `$racine/categorie` sous un quantificateur est relu « le champ
     $racine/categorie **de l'élément** est égal à « sensible » » : c'est
     faux, `$racine/…` lit l'argument de l'appel, pas l'élément courant ;
  2. chaque évènement est relu « (évènement réservé, **pas encore évalué
     par l'engine**) » : faux depuis gate-v2, qui les évalue.
- Tests : vet vert, Go vert (race, intégration comprise), Python 192 OK.

## Fusion 4 — `feat/ui`

- Commit de fusion `b7b78a5` (fusion faite en `--no-commit`, commitée
  après la suite verte).
- **Conflit : `test/integration/integration_test.go`**, dans
  `banc.verifier` et autour du chargeur de jeu. ui ajoute l'appel
  `conserver(t, b, brut)` et la fonction `conserver`
  (`MCP_CONSERVER_JOURNAUX`), et garde l'ancien `chargerJeu(t)` (jeu fixe).
  Résolution : `conserver` gardé tel quel, appelé au même endroit ; le jeu
  rejoué reste celui du banc (`chargerJeuChemin(t, b.jeu)`), sans quoi les
  scénarios v2 et base périmée v2 se rejoueraient sur le mauvais jeu ;
  `chargerJeu(t)` d'ui n'a plus d'appelant et disparaît.
- `docs/demo/maquettes/` : arrivées par la fusion, **non modifiées**
  (`git diff HEAD feat/ui -- docs/demo/maquettes` vide).
- Fichiers dérivés : régénérés, catalogue inchangé.
- Tests : vet vert, Go vert (race, intégration comprise, console
  comprise), Python 192 OK.

## Après les quatre fusions

1. **Reproductibilité.** `scripts/construire-serveurs.sh` puis
   `go run ./cmd/registre-demo -gels servers/gels-avoirs/enregistrements -catalogue agents/compilateur/exemples/catalogue-demo.json`,
   deux fois : mêmes octets pour `bin/manifeste.json`, `demo/registre.json`,
   `demo/registre-attaque.json` et le catalogue (sha256 comparés). Le
   catalogue versionné n'a changé à aucune fusion.
2. **Suite verte.** vet, `go test -race -count=1 ./...`, Python 192 OK ;
   `go test -count=3 ./test/integration/` vert (deux fois, avant et après
   le dernier test ajouté).
3. **Journaux v2.**
   `MCP_CONSERVER_JOURNAUX=<dossier> go test ./test/integration/ -run 'WeekEnd|TopManagement|RH_|Demandeur_Rapport|BasePerimee'`
   produit 19 journaux (8 tests, sous-tests compris). Pour **chacun** :
   `go run ./cmd/journal verifier` → code 0, et
   `go run ./cmd/journal rejouer … -directives demo/directives/<jeu>.json -coffre …`
   → code 0 (aucun écart).
   - **`decision.duree_us` (précision de Manu).** Le champ est présent sur
     les 31 entrées `appel` des 19 journaux. Une sonde temporaire, non
     commitée, a repris chaque journal en remplaçant `duree_us` sur toutes
     les décisions (appel et transmission) par 0, 1, 2⁴⁰ et une valeur
     négative. Elle a rejoué avec `rejeu.RejouerJournal` : 0 écart et les
     mêmes effets dans les 4 × 19 cas.
   - Ce contrôle est maintenant **permanent** (`546450c`) : `banc.verifier`
     rejoue chaque journal d'intégration une seconde fois avec une
     `duree_us` changée, et exige le même effet sans écart.
   - Hors du rejeu, changer `duree_us` dans le fichier rompt la chaîne HMAC,
     comme pour toute autre modification. C'est voulu : c'est une
     altération du journal, pas une divergence du rejeu.
4. `RAPPORT_INTEGRATION.md` (versionné, identique octet pour octet à
   `docs/rapports/integration.md`, référencé nulle part) : supprimé
   (`f3f369c`).
5. Rapports des branches : `correctifs.md`, `gate-v2.md`,
   `compilateur-v2.md` et `ui.md` sont déjà dans `docs/rapports/`, arrivés
   avec les fusions. Rien à verser.

Rien n'est retiré du format v1 (précision de Manu) : jeux v1, chargeur,
relecture v1 et `credit-demo*.json` sont inchangés. Les jeux intégrés à la
console restent aussi.

## Console : ce qui n'est pas encore affiché en v2

Méthode : la console (`cmd/console`, clé `cleTest` des tests) a tourné sur
les 19 journaux v2, plus `testnominal` pour comparer avec un journal v1.
Avec `curl`, j'ai interrogé `/` (une coque HTML statique de 21 lignes : tout
est dessiné en JS à partir de l'API), `/api/etat`, puis `chaine` et `poste`
de chaque session, `/api/regles` et `/api/avant-apres`. J'ai comparé champ
par champ aux entrées des journaux, puis lu dans `internal/console/web/*.js`
ce que chaque écran dessine de l'API. **Rien n'est corrigé.**

Les 19 chaînes sont affichées vérifiées, et aucun jeu n'est refusé
(`jeux_refuses: []`) : les cinq jeux v2 de `demo/directives` se chargent.

**Écran 1 — Journal et chaîne**
- **Demandeur.** Affiché : identité et groupes, tirés de `session_ouverte`.
  Absents : `source_identite` (jeton signé par lui-même ou par un portail),
  la référence de l'annuaire (`session_ouverte.annuaire`) et l'entrée
  `annuaire_charge` (8 journaux). Le `demandeur` journalisé sur chaque appel
  n'est pas exposé (14 appels).
- **Étiquettes.** `appel.etiquettes_cible` n'est ni dans l'API ni à l'écran
  (10 appels : `donnees_client`, `production_rapport`, `externe`,
  `donnees_personnelles`). L'API ne porte pas non plus les étiquettes des
  consultations, qui expliquent un refus U-001 (« a consulté une source
  `donnees_client` »). Seul le lien de provenance vers la consultation est
  montré.
- **Notifications.** Ni `decision.notifications` (10 appels) ni
  `appel.remises` ne sont dans l'API, ni les 10 entrées `notification`
  (destinataire, rythme, statut `remise` / `echec` / `planifiee`). Exemple :
  dans `testdemandeur_…prestataire,_données_client_reçues`, seq 2, la
  notification remise au DPO est invisible.
- **Approbations.** Les 4 entrées `approbation` (acceptée, ou refusée avec
  motif : rôle échu, valideur sans le rôle, valideur = demandeur) ne sont
  pas affichées. `appel.approbation` non plus : dans
  `testtopmanagement_reprisebonvalideur`, seq 2 est transmis après une
  approbation acceptée, mais l'écran ne montre que le badge « En attente de
  validation » (l'effet de `Decide`), un statut `erreur` et
  `transmis: true`, sans dire qui a validé ni quand. Le rôle requis
  (`validation.roles`) est bien affiché, mais pas `quatre_yeux`.
- **Origine `notification` des refus.** L'API porte `etapes[].origine =
  "notification"` (`testtopmanagement_sansecranrefus`), mais aucun JS ne lit
  `origine` : l'écran montre « Refusé » et le motif en texte (« la
  notification de T-001 au rôle juridique n'a pas pu être remise »), sans
  dire que l'origine est une notification. La carte de directive affiche en
  même temps T-001 « appliquée (en attente de validation) » : c'est
  contradictoire en apparence avec le badge « Refusé ». Les origines
  `registre` et `engine` ne sont pas distinguées non plus.
- **Autres champs v2 absents :** `version_registre` (31 appels), `sujets`
  (empreinte et groupes, 5 appels) et `sujets_non_resolus` (1 appel), les
  entrées `alerte` (horloge simulée, 4 journaux week-end), les `marques`,
  les décisions d'évènements (aucune dans ces journaux, et l'API n'a aucun
  champ pour elles).

**Écran 2 — Poste conseiller**
- Demandeur : seule l'identité est affichée (« Connecté : … »), pas les
  groupes.
- Notifications, approbation, origine : absentes, comme à l'écran 1. La
  réponse finale de `testtopmanagement_reprisebonvalideur` reste fondée
  sur l'attente, pas sur l'approbation.
- **Temps ajouté par l'engine** : toujours estimé en rejouant la décision
  (`cout.evaluation_moyenne_ms`). La mesure `decision.duree_us`, présente
  sur chaque appel, n'est pas lue : c'est la question 2 du rapport ui, à
  laquelle gate-v2 a répondu.

**Écran 3 — Règles**
- **Un seul jeu affiché** : `EnVigueur` prend le jeu cité par l'appel le
  plus récent. Ici c'est `weekend-demo`, parce que l'horloge simulée des
  journaux week-end est dans le futur. Les directives de `rh.json`,
  `top-management.json`, `rapport.json` et `credit-demo-v2.json` ne sont
  pas consultables à l'écran 3 (`/api/regles?directive=T-001` rend encore
  W-001).
- **Relecture v2 tronquée** : `regleEnFrancais` garde la 3ᵉ ligne de la
  relecture, ce qui convient au gabarit v1. Pour une directive v2, cette
  ligne est « Pour tous les appelants. ». Les blocs « Quand / Si / Alors »
  et la règle de l'indéterminé n'apparaissent pas.
- « Accès » (`acces` du déclencheur v2) : toujours « non disponible »
  (écart déjà connu du rapport ui).

**Écran 4 — Avant / après** (paramètre `directive=ID#version`)
- Fonctionne sur les journaux v2. Exemples : `testbaseperimee` + D-005
  donne « autorisé → en attente » ; `testtopmanagement_reprisebonvalideur`
  + D-002 v1 donne 2 changements.
- Seuls l'effet et le motif sont comparés. Un changement qui ne porte que
  sur les notifications, les marques, les rôles requis ou `quatre_yeux`
  s'affiche « aucun appel ne change d'effet ».
- La colonne « avant » est le rejeu de `Decide` : un appel approuvé puis
  transmis reste « en attente ».
- La liste des directives proposées réunit tous les jeux chargés, sans
  rapport avec le domaine du journal : par exemple R-001 (RH) pour une
  session de crédit.

**Hors écrans 1 à 4, constaté au passage (écran 5, file du superviseur)**
- Les appels en attente qui ont ensuite été approuvés et transmis restent
  dans la file (`testtopmanagement_reprisebonvalideur` seq 1). Les
  tentatives refusées y figurent aussi chacune comme une attente. Les
  contrôles de l'engine sont toujours « non disponible », alors que les
  entrées `approbation` les portent.
- Les noms des sous-répertoires de `MCP_CONSERVER_JOURNAUX` contiennent
  des espaces, des virgules, des « : » et des accents (noms des
  sous-tests). La console les gère (ids encodés dans l'URL), mais ils sont
  peu pratiques en ligne de commande. `TestJetonConsomme` lance deux gates
  dans le même test : le second journal écrase le premier dans la copie.
  L'écran de notifications (`notifications.jsonl`) n'est pas copié.

## Décisions prises seul

1. **`-gels servers/gels-avoirs/enregistrements`** (registre des gels
   synthétique versionné) pour `registre-demo` : le registre complet
   (`enregistrements_locaux`) n'existe pas sur la machine et le préparer
   demanderait le réseau. Le catalogue n'en dépend pas ; seuls
   `demo/registre*.json` (non versionnés) en portent l'empreinte.

2. **Variante W-003 de la relecture** passée en `si_echec: continuer`
   plutôt que déplacée vers `ouverture_session` : changement minimal, le
   chargeur fusionné fait foi.
3. **Fusion 2, `banc.directives` abandonné au profit de `banc.jeu`** : même
   donnée sous deux noms ; garder les deux aurait laissé un champ mort.
4. **Contrôle permanent de `duree_us` dans `test/integration`** (`546450c`) :
   le brief ne demande qu'une vérification sur les journaux régénérés ;
   c'est un test de cohérence, sans nouvelle fonctionnalité, qui garde la
   précision de Manu vraie pour la suite.
5. **`TestTousLesJeuxDemo` essaie aussi `demo/registre.json` s'il existe**
   (fichier non versionné, régénéré) : c'est le registre que le brief
   nomme ; absent, le test garde le registre des schémas versionnés et le
   dit dans son journal de test, plutôt que d'échouer sur une machine
   neuve.
6. **Inventaire de la console par l'API JSON et la lecture du JS** : la
   page HTML n'est qu'une coque (rendu côté client), `curl` ne peut pas en
   montrer davantage ; aucun navigateur lancé.

## Écarts relevés pour la session suivante

1. **Relecture de `$racine/…`** (`internal/relecture/relecture_v2.go`) :
   sous un quantificateur, relu « le champ $racine/x de l'élément » ; doit
   dire « l'argument /x de l'appel ».
2. **Relecture des évènements** : la mention « évènement réservé, pas
   encore évalué par l'engine » est obsolète depuis gate-v2. La ligne
   d'indéterminé d'une directive d'évènement parle aussi d'« appel ».
3. **`python -m compilateur.scenarios_v2 <nom>`** (rejeu, catalogue généré)
   lève désormais `EnregistrementAbsent` (vérifié sur `weekend`) : seuls
   les tests rejouent sur le catalogue gelé. Même chose pour la démo texte
   v2 rejouée sur le catalogue généré. À réenregistrer avec le catalogue
   courant (appels Mistral réels, autant que de requêtes enregistrées), ou
   à doter d'une option `--catalogue`.
4. **Vocabulaire du compilateur** (`agents/compilateur/exemples/vocabulaire-demo.json`)
   toujours écrit à la main ; `demo/etiquettes.json` et l'annuaire
   existent maintenant dans la même branche (question 2 de compilateur-v2 :
   à générer après la fusion).
5. **Console** : voir la section dédiée ci-dessus (notifications,
   approbations, étiquettes, origine, relecture v2 tronquée, jeu unique à
   l'écran 3, `duree_us` non lue).
6. **Commit de fusion rouge `3315062`** : à garder en tête pour un
   `git bisect` (les commits `07b423c` et `d40da1d` le corrigent).

## Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-fusion
git log --oneline --first-parent 782ff37..HEAD
go vet ./... && go test -race -count=1 ./...
(cd agents && python3 -m unittest discover -s tests -t .)      # 192 OK
go test -count=3 ./test/integration/
go test -count=1 -v -run TestTousLesJeuxDemo ./internal/registredemo/

# Journaux v2, vérifiés et rejoués
export MCP_GATE_CLE_JOURNAL=$(printf 'integ-integ-integ-integ-integ-integ-' | xxd -p | tr -d '\n')
MCP_CONSERVER_JOURNAUX=/tmp/j go test -count=1 ./test/integration/ -run 'WeekEnd|TopManagement|RH_|Demandeur_Rapport|BasePerimee'
go run ./cmd/journal verifier /tmp/j/testtopmanagement_reprisebonvalideur/journal.jsonl
go run ./cmd/journal rejouer /tmp/j/testtopmanagement_reprisebonvalideur/journal.jsonl \
    -directives demo/directives/top-management.json -coffre /tmp/j/testtopmanagement_reprisebonvalideur/coffre.jsonl
go run ./cmd/console -donnees /tmp/j -jeux demo/directives      # http://127.0.0.1:8088/
```
