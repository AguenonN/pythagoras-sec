# Rapport — session B « compilateur » : fiable sur scène, branché sur la console

Branche `feat/compilateur-final`, worktree `~/cybercar/mcp_rogue-compilateur-final`.
Rien n'est poussé ni fusionné ; `main` n'est pas touchée.

**Compteur d'appels Mistral réels : 10 / 25** (étape 6 : 8 ; étape 8 : 2). Aucune génération interrompue.

## 1. Ce qui est fait

| Étape | Commit | Objet |
|---|---|---|
| — | `3e9149c` | `demo/directives/demo.json` et `demo/annuaire-source.json` repris **à l'identique** de `feat/demo-reelle` (`ca3bbdd`), voir décision 1 |
| 1 | `2ac033d` | Vocabulaire généré : `go run ./cmd/registre-demo -vocabulaire …` ; tests de divergence Go et Python ; ancien vocabulaire gelé pour les enregistrements existants |
| 3 | `7c53da0` | `directive relire --directives D-009,…` : relecture des seules directives ajoutées, jugées dans le jeu entier |
| 3, 5, 4, 2 bis | `120e504` | `python -m compilateur.service` (contrat §4) ; ajout à un jeu existant (version n+1, numérotation à la suite) ; correctif demandeur **et** sujet ; `scenarios_v2 --catalogue --vocabulaire --jeu-existant --modele` ; `demo --jeu-existant` |
| — | `d191eb0` | Un outil de lecture est désigné par ses propres mots (« base interne des taux » → `get_taux_interne`) ; une action ne l'est jamais |
| 6, 8 | `c7f61e3` | Choix du modèle sur mesures (**`mistral-medium-latest`** par défaut du compilateur) ; parcours de scène week-end et base interne (D-009) enregistrés par le service et rejoués sans réseau ; D-010 après D-009 |
| 2 | (ce commit) | Relecture des formes de gate-v2 : `$racine/…` sous un quantificateur, directives d'évènement ; fichier attendu `testdata/v2/gate-v2.txt`, trois fuseaux machine |

### Étape 2 — relecture des formes ajoutées par gate-v2

Nouveau jeu `internal/relecture/testdata/gate-v2.json` (attendu
`testdata/v2/gate-v2.txt`, comparé octet pour octet, `time.Local` changé et
sous-processus `TZ=UTC`, `Asia/Tokyo`, `America/Los_Angeles`) :

- **`$racine/x`** (feuille `argument`, `champ` d'un quantificateur imbriqué,
  `champ` d'une feuille `sujet`) est relu « l'argument /x de l'appel (lu
  dans les arguments, et non dans l'élément courant) », et non plus « le
  champ $racine/x de l'élément ».
- **Évènements** : « à l'ouverture de chaque session », « à chaque
  attestation en échec [du serveur X] (connexion ou changement) », « à
  chaque purge du coffre » ; la mention « pas encore évalué par l'engine »
  est retirée.
- Corrigé en plus, trouvé en écrivant l'attendu : pour une directive
  d'évènement, `temps` dit « l'évènement a lieu » (et non « l'appel ») ;
  `refuser` à l'ouverture se lit « la session n'est pas ouverte » (y compris
  après une notification `si_echec: refuser`) ; la ligne de l'indéterminé
  suit `DecideEvenement` (ouverture : la session n'est pas ouverte ; autres
  évènements : directive tenue pour déclenchée, l'évènement suit son cours,
  notifications et marques appliquées) ; « Applicable aux évènements ».
- Attendus existants modifiés : seules les trois lignes « Quand » des
  évènements de `complet.txt` et `variantes.txt`.

### Étape 6 — choix du modèle, sur mesures

Même texte, même fondement, mêmes réponses du juriste, `--jeu-existant
demo/directives/demo.json`, par le service. Le budget (8 appels) ne
permettait **qu'un passage** par modèle et par texte (température 0 et
graine fixe : un second passage identique serait servi par le cache).
Les deux parcours medium ont été terminés sur le budget de l'étape 8 (2
appels), dont ils sont les enregistrements.

| Modèle | Texte | Appels | Questions obtenues | Jeu accepté du premier coup | Jeu fidèle au texte | Générations interrompues |
|---|---|---|---|---|---|---|
| small | week-end | 3 | 1 : fuseau (texte libre, demande aussi des heures) | non : article du fondement omis, corrigé | **non : condition inversée** (`non(samedi, dimanche)` : refus **en semaine**), acceptée par le code, visible à la relecture (« Si la condition suivante est fausse ») | 0 |
| small | base interne | 3 | 2 : « durée d'un trimestre » (hors sujet : c'est la raison de la règle), validation ou notification | non : nœud inventé (`attendre_validation` comme feuille), puis groupe `analystes-credit` inventé en feuille `sujet` | **refusé** par le code après la correction | 0 |
| medium | week-end | 2 | 1 : fuseau, options = liste fermée des fuseaux | **oui** | **oui** : `temps(samedi, dimanche, Europe/Paris)` → refuser, fondement art. 12 | 0 |
| medium | base interne | 2 | 3 : cible (attribuer_pret seul ?), validation ou refus, rôle « analyste des risques » = `analyste-risques` (option) | **oui** | **oui** : identique à D-009 des contrats (`a_consulte get_taux_interne` → attendre_validation `analyste-risques`), critère `donnees_recues` | 0 |

**Retenu : `mistral-medium-latest`** (`mistral.MODELE_COMPILATEUR`, défaut de
`service`, `demo` et `scenarios_v2` ; `--modele` ou `COMPILATEUR_MODELE`
pour en changer). Le client Mistral garde `mistral-small-latest` par
défaut : la boucle et l'orchestrateur de la session A l'utilisent. Les
mesures sont rejouées par `tests/test_service_scene.py` (`TestChoixDuModele`).

### Étape 8 — textes de la scène, enregistrés pour le secours

Parcours (`agents/compilateur/exemples/service/weekend.json` et
`base-interne.json`) : la requête du contrat §4 et les réponses du juriste.
Rejoués sans réseau par `tests/test_service_scene.py` :

- **week-end** : question fuseau (Mistral) → `si_indetermine` (code) → jeu
  candidat `demo.v2.json`, D-009 en brouillon (refus de tout appel d'outil
  le samedi et le dimanche, heure de Paris), accepté par `directive verifier`,
  relecture attendue `tests/attendus/service/weekend.txt` ;
- **base interne** : trois questions (Mistral) → `si_indetermine` → D-009
  identique à celle des contrats, relecture attendue
  `tests/attendus/service/base-interne.txt` ;
- **D-010 après D-009** : le week-end ajouté au jeu qui contient déjà D-009
  (activée) rend D-010, version 3, sans nouvel enregistrement ;
- le parcours week-end est aussi déroulé **par la ligne de commande**, une
  requête JSON par appel, comme la console l'appellera : 1, 1, puis 0 appel
  nouveau au dernier rappel.

Sur scène, en secours : `python -m compilateur.service` (mode `rejeu` par
défaut) avec exactement ces textes, ce fondement et ces réponses. En mode
`reel`, ces textes sont rejoués sans appel ; un autre texte part vers
Mistral.

### Le service (`agents/compilateur/service.py`)

- Une requête JSON sur l'entrée standard, une réponse JSON sur la sortie,
  clés exactement celles du contrat §4, dans cet ordre.
- État dans `demo/execution/compilation/<id>/` : `conversation.json`
  (requête initiale, questions posées et leurs identifiants, réponses,
  tours) et `appels/` (chaque réponse de Mistral obtenue pour la
  conversation). Une requête déjà posée dans la conversation est servie
  depuis `appels/` : le rappel ne refait **aucun** appel, quel que soit le
  mode (testé avec un client qui échoue s'il est appelé).
- Identifiants de question : le champ pour une question du code
  (`si_indetermine`, `designation_personne`, `rythme:D-009:<rôle>`…),
  `m-<empreinte du texte>` pour une question de Mistral.
- `jeu_candidat` : `demo/execution/compilation/<id>/demo.v<n+1>.json`,
  directives existantes inchangées puis les ajoutées en `brouillon`,
  accepté par `directive verifier` ; `relecture` : les seules ajoutées.
- `python -m compilateur.service --parcours exemples/service/<nom>.json`
  déroule un parcours scripté (réponses par texte de question ou par
  identifiant) : c'est ainsi que les parcours de scène sont enregistrés et
  rejoués.

### Correctif demandeur et sujet (étape 4)

La réponse à « l'utilisateur : demandeur ou personne concernée ? » dit
seulement à quelle feuille le mot correspond. Le jeu n'est refusé que si
**aucune** feuille du type désigné n'y figure ; une règle qui porte les deux
feuilles est acceptée dans les deux sens (test scripté), y compris avec un
jeu existant.

## 2. Ce qui n'est pas fait

(en cours)

## 3. Décisions prises seul

1. **Copie des fichiers de la session A.** `demo/directives/demo.json` n'existait
   pas sur ma branche et l'annuaire n'y portait ni `analyste-risques`, ni
   `responsable-rh`, ni `direction-juridique`. Or le critère de réussite passe
   par `--jeu-existant demo/directives/demo.json`, et chaque requête Mistral
   enregistrée embarque le vocabulaire tiré de l'annuaire : enregistrer sur
   l'ancien annuaire rendrait les enregistrements caducs dès la fusion. J'ai
   donc repris ces deux fichiers **octet pour octet** depuis `feat/demo-reelle`
   (`ca3bbdd`, dernier commit de A). Je ne les modifie pas. Mêmes contenus des
   deux côtés : la fusion ne crée pas de conflit. Si A les change encore, il
   faudra régénérer le vocabulaire et réenregistrer (voir §5).
2. **Groupes du vocabulaire** : ceux de l'annuaire (feuille `demandeur`) **et**
   ceux du référentiel RH (`servers/referentiel-rh/groupes_fictifs.json`,
   feuille `sujet`). Le brief ne cite que l'annuaire, mais sans le référentiel
   RH le compilateur ne pourrait plus nommer `salaries-proteges` ni
   `contentieux-en-cours` (D-006, D-007, scénario RH). La description de
   chaque groupe dit de quelle feuille il relève.
3. **`-vocabulaire` écrit seulement le vocabulaire**, sans lire le manifeste
   des binaires : il ne dépend que de fichiers versionnés, il se régénère
   partout.
4. **Tous les rôles de l'annuaire** entrent au vocabulaire, `emetteur-identite`
   compris : aucun tri à la main.
5. **Enveloppe du jeu candidat** : `id` et `version` n+1 du jeu existant,
   `date_effet` et `auteur` ceux de la requête (le juriste qui compile).
   L'`empreinte` et la `signature` de la version n ne sont pas reprises :
   elles ne valent pas pour n+1.
6. **« Inchangées octet pour octet »** : chaque directive existante a le même
   JSON canonique (celui de l'empreinte) et le même ordre de clés que dans
   `demo.json` ; la mise en page à la main du fichier n'est pas recopiée
   (le candidat est réécrit, indenté). Testé.
7. **Le jeu existant n'entre pas dans la requête à Mistral** : un
   enregistrement reste valable quand on active ou retire une directive.
   Seuls les identifiants des directives ajoutées en dépendent, via les
   messages de correction.
8. **Le fondement de la requête** entre dans le message à Mistral (bloc
   « Fondement donné par le juriste ») et dans la source des vérifications ;
   absent, la requête est celle d'avant (enregistrements existants
   inchangés). `texte_fr` reste le seul texte du juriste.
9. **Mode par défaut du service : `rejeu`** (jamais de réseau sans le
   demander). La console passe `--mode reel` ou `COMPILATEUR_MODE=reel`
   pour compiler en direct ; en `reel`, les textes enregistrés sont rejoués
   sans appel.
10. **Codes de sortie** : 0 pour toute réponse du contrat, y compris
    `etat: erreurs` quand la compilation n'aboutit pas (Mistral
    injoignable…) ; 2 pour une requête invalide (la réponse JSON est écrite
    quand même).
11. **Rappel avec un autre texte** (ou autre fondement, auteur, jeu, date) :
    refusé, « ouvrir une nouvelle conversation ». Le même texte renvoyé par
    la console est admis.
12. **Correctif demandeur/sujet jugé sur le jeu entier** : il suffit qu'une
    feuille du type désigné figure dans une des directives proposées.
13. **Fondements des textes de scène** : le brief ne les donne pas. Week-end :
    « Règlement intérieur, article 12 » (celui des jeux de référence v2) ;
    base interne : « Code monétaire et financier, article L313-5-1 » (celui de
    la D-009 écrite par la session A). Auteur `julie.juridique@exemple.test`,
    date d'effet celle de l'exemple du contrat §4. Seuls texte, fondement et
    réponses entrent dans les requêtes : un autre auteur ou une autre date
    se rejoue aussi.
14. **Réponses du juriste aux questions de Mistral** : écrites par moi, au plus
    près du texte. Base interne : « retard d'un trimestre » est la raison de
    la règle, pas une condition ; la cible est `attribuer_pret` seul.
15. **Un passage par modèle et par texte** au lieu de trois : le budget de 8
    appels n'en permettait pas plus (voir le tableau).
16. **La version medium du week-end bloque aussi la lecture** : « aucune
    exécution de workflow agentique » ne distingue pas lecture et action, et
    medium n'a pas posé la question. C'est fidèle au texte ; la relecture le
    dit (« tout appel d'outil, sur tout serveur »). Si la scène veut « pas
    d'**action** le week-end » (titre de D-010 aux contrats), le juriste doit
    l'écrire, et ce texte-là n'est pas enregistré.

## 4. Blocages

Aucun pour l'instant.

## 5. Questions pour Manu

(aucune pour l'instant)

## 6. Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-compilateur-final
go vet ./... && go test -race -count=1 ./cmd/registre-demo ./cmd/directive ./internal/relecture
(cd agents && python3 -m unittest discover -s tests -t .)
go run ./cmd/registre-demo -vocabulaire agents/compilateur/exemples/vocabulaire-demo.json && git diff --stat
```
