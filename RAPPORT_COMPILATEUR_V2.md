# Rapport — session « compilateur-v2 » : du texte du juriste au jeu v2

Branche `feat/compilateur-v2`, worktree `~/cybercar/mcp_rogue-compilateur-v2`.
Rien n'est poussé ni fusionné ; `main` n'est pas touchée ; `internal/directive`
et le gate ne sont pas modifiés.

État : **en cours**.

## 1. Fait

| Étape | Commit | Objet |
|---|---|---|
| 1 | `cd8cf89` | `internal/relecture` : relecture v2 déterministe (déclencheur, arbre, feuilles, effets, fuseau, indéterminé), fichiers attendus octet pour octet, trois fuseaux machine |
| 2 | `cd8917a` | `cmd/directive` : `verifier` et `relire` sur jeux v2, `--exiger-brouillon` conservé, tests |
| 3 | `dafdb6e` | Schéma de function calling sans récursion : arbre à plat reconstruit par le code (`arbre.py`), énumérations tirées du vocabulaire (`vocabulaire.py`, `outils_v2.py`), `directive fuseaux` |
| 4, 5 | `1a37a2b` | Un texte → un jeu v2 complet en brouillon ; refus motivés (`signaler_non_observable`) ; vérifications v2 ; questions fermées du code |
| 6a | (ce commit) | Scénarios enregistrés crédit et week-end, rejoués ; durcissements trouvés en mode réel |

### Étape 1 — relecture v2

- `relecture.Directive` rend une directive v2 par `directiveV2` ; la relecture
  v1 est inchangée octet pour octet (les attendus v1 et `agents/tests/attendus/d001.txt`
  passent sans modification). Un jeu qui contient au moins une directive v2
  porte une ligne de plus sur le cumul des notifications et marques.
- Forme : identité et fondements (le premier est dit « cité par les motifs »),
  validité, périmètre, `Quand :` (déclencheur), `Si …` (arbre indenté, une
  condition par tiret, `et`/`ou` annoncés, `non` en phrase niée quand la
  feuille le permet, quantificateurs avec leur valeur sur liste vide),
  `Alors :` (un effet par tiret), puis la règle de l'indéterminé à la racine.
- Le fuseau d'une feuille `temps` est toujours nommé ; les dates de
  l'enveloppe et de `source.date` sont écrites en UTC ; les dates de feuille
  `temps` sont réécrites sans passer par une heure.
- Tests : 7 jeux (les 6 scénarios v2 de `internal/directive/testdata/v2`, lus
  sans être modifiés, et `testdata/variantes-v2.json`) comparés à
  `testdata/v2/*.txt` ; changement de `time.Local` en cours d'exécution ;
  sous-processus avec `TZ=UTC`, `Asia/Tokyo`, `America/Los_Angeles` ; test de
  couverture (chaque nœud, feuille, opérateur, effet et évènement employé par
  au moins un jeu relu) ; aucun gabarit de repli sur un jeu accepté.

### Étape 2 — `cmd/directive` v2

- `verifier` et `relire` passaient déjà par `directive.Charger`, qui accepte
  v1 et v2 ; seule la relecture manquait (étape 1). `--exiger-brouillon` est
  conservé et vaut pour les directives v2.
- Tests (`main_v2_test.go`) : les six jeux v2 des scénarios acceptés et relus
  octet pour octet comme `internal/relecture/testdata/v2/*.txt` ;
  `--exiger-brouillon` sur un jeu v2 actif (une erreur par directive) ; erreurs
  v2 rendues en bloc avec le chemin du nœud (`condition.et[0].temps`), statuts
  contrôlés même quand le chargeur refuse.

### Étape 3 — function calling sans récursion

- `agents/compilateur/arbre.py` : la condition arrive **à plat** (`noeuds` :
  liste de `{id, type, enfants, paramètres}`, `racine` : identifiant). Le code
  reconstruit l'arbre v2 et refuse, avant le chargeur Go : identifiant absent
  ou en double, enfant ou racine inconnus, cycle, nœud à deux parents, nœud
  non rattaché, mauvais nombre d'enfants, enfant d'une feuille, paramètre non
  admis pour le type ou manquant, profondeur > 6, plus de 64 nœuds. Chaque
  nœud n'ayant qu'un parent, la reconstruction ne peut pas boucler.
- `agents/compilateur/outils_v2.py` : trois outils (`proposer_jeu`,
  `poser_question`, `signaler_non_observable`) et la consigne. Aucun `$ref`,
  `anyOf`, ni schéma récursif. Énumérations : types de nœuds (les 15 du
  schéma), opérateurs, effets, jours, modes d'attestation, serveurs, outils
  (simples et qualifiés), arguments, étiquettes, rôles, groupes, fuseaux.
  Absents des outils : statut, version, identifiants, `texte_fr`,
  `date_effet`, `si_indetermine`, `rythme`, `si_echec`, `evenement`.
- `agents/compilateur/vocabulaire.py` : réunit le catalogue généré
  (`catalogue-demo.json`, inchangé), un complément écrit à la main
  (`exemples/vocabulaire-demo.json` : étiquettes, rôles, groupes) et la liste
  des fuseaux **du chargeur Go**, obtenue par la nouvelle sous-commande
  `directive fuseaux` (pas de copie tenue à la main).
- Tests : `agents/tests/test_arbre.py` (14 : chaque type accepté par le
  chargeur une fois reconstruit ; cycles, deux parents, orphelins,
  profondeur 7, 65 nœuds, arité, paramètres) et `test_outils_v2.py` (8 :
  aucune récursion, énumérations égales au vocabulaire et aux fuseaux Go, ce
  que Mistral ne fixe jamais) ; `cmd/directive` : `TestFuseaux`.

### Étape 4 — un texte → un jeu complet (et mécanique de l'étape 5)

- `agents/compilateur/compilateur_v2.py`, `compiler_jeu` : un appel
  `proposer_jeu` porte **plusieurs directives** ; le code les numérote
  (`D-001`, `D-002`…), les met toutes en `brouillon`, fixe `date_effet`,
  et prend pour `texte_fr` l'extrait que Mistral a recopié **s'il figure mot
  pour mot** dans le texte, sinon le texte entier. Le tout est réuni dans un
  seul jeu, vérifié par `directive verifier --exiger-brouillon`, relu par
  `directive relire`.
- États : `questions`, `non_observable` (refus motivé : extrait, raison, et
  une question pour un critère observable), `jeu`, `refuse`, `erreurs`.
  Le refus motivé l'emporte sur une question, qui l'emporte sur une
  proposition. Une seule correction renvoyée à Mistral, puis refus.
- Questions **du code** (fermées, consignées, jamais envoyées à Mistral) :
  `si_indetermine` (une question pour le jeu, qui liste les directives
  concernées) ; pour chaque notification, `rythme` et `si_echec`. Le
  chargeur contrôle d'abord le jeu avec des valeurs provisoires, qui ne sont
  jamais rendues.
- `agents/compilateur/verifications_v2.py` (rien d'inventé) : montants,
  dates, heures, jours (« week-end » justifie samedi et dimanche), fuseau
  (nom IANA ou ville écrite), étiquettes, rôles et groupes **sous leur forme
  exacte** (« données client » ne justifie pas `donnees_client` : il faut le
  choisir), quatre yeux, fondements, identités ; une condition sur un
  argument doit porter sur un argument de **chaque** outil visé (sinon elle
  serait indéterminée, donc refusée, sur les autres).
- Tests : `agents/tests/test_compilateur_v2.py` (client scripté).

### Étape 6 — scénarios enregistrés (crédit, week-end)

- `agents/compilateur/scenarios_v2.py` déroule un scénario
  (`exemples/scenarios-v2/<nom>.json` : texte, réponses du juriste) tour par
  tour ; les réponses aux questions de Mistral sont retrouvées par le texte
  de la question, celles du code par leur champ. Les réponses ont été écrites
  en mode réel au fil des questions, puis sont rejouées par les tests sans
  appel. `python -m compilateur.scenarios_v2 <nom>` affiche le déroulé.
- **Crédit** (un texte → 4 directives `C-001`…`C-004`) : questions de
  Mistral (seuil, taux officiel, recherche d'entreprises), puis question du
  code `si_indetermine`, puis jeu : `non(a_consulte gels-avoirs/verifier_gel,
  donnees_recues)` → refus ; `/montant > 500000` → refus ;
  `a_consulte taux-officiel` → attente ; caviardage `/siren` vers
  `recherche-entreprises`. Fondements avec articles. Chaque `texte_fr` est un
  extrait mot pour mot du paragraphe.
- **Week-end** (« Désormais aucune exécution de workflow agentique le
  week-end. ») : question consultation/production ; la première proposition
  (sans fuseau ni fondement écrits) est refusée par le code, et la correction
  pose les questions **fuseau** et **fondement** ; puis `si_indetermine` ;
  jeu identique (déclencheur, condition, effets, fondements) à
  `internal/directive/testdata/v2/weekend.json`.
- Tests : `agents/tests/test_scenarios_v2.py` (questions attendues tour par
  tour, jeu accepté par le chargeur, relecture égale à
  `agents/tests/attendus/v2/<nom>.txt` sur trois déroulés).
- Durcissements trouvés en mode réel (chacun testé) : article de fondement
  omis alors qu'écrit ; `acces` qui ne restreint rien (refusé : il ajouterait
  un indéterminé) ; `critere: transmis` non écrit par le juriste (« consulter »
  = `donnees_recues`, le plus fermé) ; racine sans nœuds ; réponses
  dédoublonnées et déclarées définitives ; question déjà répondue écartée ;
  `finish_reason: error` de Mistral signalé.
- Les enregistrements devenus inutiles après chaque changement de consigne
  (8 fichiers non versionnés) ont été déplacés hors du dépôt, pas commités.

## 2. Pas fait (encore)

Étape 6 : top management, RH, demandeur (et un refus « ne pas discriminer ») ; étape 7 (démo).

## 3. Décisions prises seul

1. **Relecture v1 inchangée** : un jeu purement v1 est relu exactement comme
   avant (d'autres tests et le compilateur v1 en dépendent). La phrase sur le
   cumul n'apparaît que si le jeu contient une directive v2.
2. **Règle de l'indéterminé** écrite seulement si quelque chose peut être
   indéterminé (condition présente, ou cible portant sur `etiquette`/`acces`) ;
   la mention « les notifications et marques s'appliquent aussi » seulement si
   la directive en a.
3. **Étiquettes, rôles, groupes : un complément écrit à la main.** Le
   registre de démo et le catalogue généré ne portent ni étiquettes, ni
   rôles, ni groupes, et je n'ai pas le droit de modifier `cmd/registre-demo`.
   Je les tiens dans `agents/compilateur/exemples/vocabulaire-demo.json`,
   marqué « écrit à la main, à générer dès que le registre les portera ».
   Les groupes RH sont ceux de `servers/referentiel-rh` ; les autres noms
   (`donnees_client`, `production_rapport`, `prestataires-externes`,
   `directeur-risques`, `juridique`…) reprennent ceux des scénarios v2.
4. **Fuseaux tirés du Go** par une sous-commande `directive fuseaux` ajoutée
   à `cmd/directive` (périmètre autorisé), plutôt qu'une copie en Python.
5. **Seul l'évènement `appel_outil` est offert à Mistral** : les autres ne
   sont pas évalués par l'engine ; une règle qui en relèverait doit être
   signalée, pas compilée en une directive qui aurait l'air de compter.
6. **`texte_fr` d'une directive issue d'un paragraphe** : l'extrait recopié
   par Mistral s'il figure mot pour mot dans le texte, sinon le paragraphe
   entier. Jamais une reformulation.
7. **`si_indetermine` : une question pour tout le jeu** (et non une par
   directive), pour ne pas noyer le juriste ; la réponse vaut pour chaque
   directive concernée, et `ignorer` nomme le juriste dans chacune.
8. **Rythme et conduite en cas d'échec d'une notification : questions du
   code**, pas de Mistral : ce sont des paramètres obligatoires que le texte
   ne fixe presque jamais, et Mistral ne doit pas les choisir.
9. **Nombre écrit en chaîne** (`"500000"`) relu comme nombre, et flottant
   entier (`2000000.0`) comme entier : même valeur, autre écriture.

## 4. Blocages

Aucun bloquant. Budget Mistral : **15 appels réels sur 30** après crédit et
week-end (compteur `agents/.etat/compilateur-v2/budget.json`). Un appel a
rendu `finish_reason: error` sans contenu (côté Mistral).

## 5. Questions pour Manu

(à venir)

## 6. Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-compilateur-v2
go vet ./... && go test -race -count=1 ./internal/relecture ./cmd/directive
go run ./cmd/directive relire < internal/directive/testdata/v2/credit.json
```
