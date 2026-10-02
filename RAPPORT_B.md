# Rapport — session B « compilateur » : fiable sur scène, branché sur la console

Branche `feat/compilateur-final`, worktree `~/cybercar/mcp_rogue-compilateur-final`.
Rien n'est poussé ni fusionné ; `main` n'est pas touchée.

**Compteur d'appels Mistral réels : 0 / 25.**

## 1. Ce qui est fait

| Étape | Commit | Objet |
|---|---|---|
| — | `3e9149c` | `demo/directives/demo.json` et `demo/annuaire-source.json` repris **à l'identique** de `feat/demo-reelle` (`ca3bbdd`), voir décision 1 |
| 1 | `2ac033d` | Vocabulaire généré : `go run ./cmd/registre-demo -vocabulaire …` ; tests de divergence Go et Python ; ancien vocabulaire gelé pour les enregistrements existants |
| 3 | `7c53da0` | `directive relire --directives D-009,…` : relecture des seules directives ajoutées, jugées dans le jeu entier |
| 3, 5, 4, 2 bis | (ce commit) | `python -m compilateur.service` (contrat §4) ; ajout à un jeu existant (version n+1, numérotation à la suite) ; correctif demandeur **et** sujet ; `scenarios_v2 --catalogue --vocabulaire --jeu-existant --modele` ; `demo --jeu-existant` |

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
