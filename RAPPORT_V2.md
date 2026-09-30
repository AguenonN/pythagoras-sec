# Rapport : schéma de directive v2 et `Decide` v2

Branche `feat/v2`, worktree `~/cybercar/mcp_rogue-v2`, partie de `main` à
`af46f0e`.

**schéma figé : 5781dca**

(`docs/schema-directive-v2.md`, `internal/directive/schema_v2.go`,
`directive_json.go`, `charger_v2.go`, `convertir.go`, `fuseaux.go`,
`decimal.go` ; champs `omitempty` dans `internal/journal`.)

**Critère de réussite atteint** : l'étape 1 est commitée et signalée. L'étape 2
est verte avec les cinq tests et les scénarios. Aucun test existant n'est
cassé (`go vet ./...`, `go test -race -count=1 ./...` et les 113 tests
Python passent).

## 1. Fait

| Commit | Contenu |
|---|---|
| 5781dca | Étape 1 : schéma v2 figé (doc + types), chargeur strict, conversion v1→v2 (`ConvertirV1`), base de fuseaux IANA 2025b embarquée (21 fuseaux), décimaux exacts, champs `omitempty` du journal. Empreintes des trois jeux v1 du dépôt inchangées, vérifiées contre le code d'avant. |
| dd18223 | Étape 2 : `Decide` v2 (évaluateur Kleene, dix feuilles, liste d'effets, précédence, notifications, marques, validation), cinq tests, scénarios, extension du test des motifs. |

Les cinq tests :

1. **Garde de pureté** (`purete_test.go`) : elle parcourt tout
   `internal/directive`, sous-répertoires compris, et interdit en plus l'aléa,
   `regexp`, `reflect`, `runtime`, `time/tzdata`, `time.LoadLocation`,
   `time.Local` et toute lecture de l'horloge. S'y ajoute un test qui change
   `time.Local` en cours d'exécution.
2. **Répétition** (`TestDecide_Repetition1000`) : 1 000 évaluations sur un
   jeu qui réunit toutes les directives v2 et v1, identiques en `DeepEqual`
   et une fois sérialisées.
3. **Arbres aléatoires** (`TestArbresAleatoires_SansPaniqueNiDivergence`) :
   3 000 jeux générés à graine fixe, y compris des nœuds invalides que le
   chargeur refuserait. Aucune panique, aucune divergence, aucune sentinelle
   d'argument dans un motif. Ce test a trouvé une vraie panique (feuille
   `demandeur` vide, construite hors chargeur), corrigée avant le commit.
4. **Table de vérité** (`TestKleene_*`) : les 3 cas de `non` et les 9 de
   `et` et de `ou`, sur les fonctions et sur des arbres réels. S'y ajoutent
   les quantificateurs, tableau vide compris, et `si_indetermine` appliqué
   à la racine seulement.
5. **Rejeu sans écart** :
   - `TestRejeu_V1IdentiqueSousV2` compare une copie figée du `Decide` v1
     (`decide_v1_ref_test.go`, gardée comme oracle) au `Decide` v2. La
     comparaison porte sur 20 000 décisions, tirées des trois jeux v1 du
     dépôt et de jeux v1 générés qui couvrent les six verrous, l'observation,
     le périmètre, les échéances et `si_indetermine`. Tout est identique :
     effet, arguments, empreintes, applications, motifs, alertes.
   - Les tests du gate, du rejeu et de l'intégration (`test/gate`,
     `test/integration`, `internal/rejeu`, `internal/gate`) produisent,
     vérifient et rejouent leurs journaux sous `Decide` v2 : tous sont verts.
   - **Note** : le brief annonçait un worktree sans `feat/integration`. En
     réalité, `feat/v2` contient la fusion `af46f0e` (vérifié par
     `git merge-base --is-ancestor feat/integration HEAD`). Les journaux et
     jeux de l'intégration sont donc **déjà rejoués ici**. Si une autre
     version de l'intégration est fusionnée plus tard, il faudra la rejouer
     après fusion avec la même commande.
   - Aucun journal n'est versionné dans le dépôt : tous sont produits par
     les tests. Les enregistrements du compilateur
     (`agents/compilateur/enregistrements`) ne contiennent que des
     brouillons de directive sans enveloppe. Ils ne sont pas des jeux et
     n'ont pas été rejoués.

Scénarios (`scenarios_v2_test.go`, jeux dans `internal/directive/testdata/v2/`) :

- **Crédit** : `non(a_consulte gels-avoirs)` refuse, montant > 500 000
  refuse, `a_consulte taux-officiel` met en attente, caviardage de `/siren`
  vers `recherche-entreprises`. S'y ajoutent le décimal exact
  (500 000,000…01), le montant illisible (refus par défaut) et la
  précédence.
- **Week-end** : les six horodatages du brief donnent les résultats
  attendus. Les résultats sont identiques en sous-processus avec `TZ=UTC`
  et `TZ=Asia/Tokyo`, et avec `time.Local` modifié.
- **Top management** : attente, rôle `directeur-risques`, quatre yeux,
  notification au juridique.
- **RH** : un salarié en contentieux refuse, un salarié protégé en action
  met en attente, les deux ensemble refusent. Aucun identifiant de salarié
  n'apparaît dans un motif.
- **Demandeur** : groupe X, données reçues d'un serveur étiqueté
  `donnees_client`, cible `production_rapport` : refus et notification.

## 2. Pas fait, et pourquoi

- **Évènements `ouverture_session`, `attestation_echouee`, `purge`** : ils
  sont définis par le schéma, mais aucun évaluateur n'existe, car le gate
  est hors périmètre. Le chargeur ne les accepte qu'au statut `brouillon`.
- **Branchement dans le gate, le rejeu, le compilateur et la relecture** :
  hors périmètre (brief). Voir les questions 1 et 2.
- **Conversions du journal** (`DepuisDecision`, `VersConsultations`…) :
  elles ne sont pas mises à jour, parce que le brief n'autorise dans
  `internal/journal` que l'ajout de champs `omitempty`. Les nouveaux champs
  existent mais ne sont pas encore remplis.

## 3. Décisions prises seul

1. **Discriminant par directive** (clé `declencheur`) plutôt que par jeu.
   Un jeu peut mêler v1 et v2, et un jeu v1 reste accepté octet pour octet.
   Porter à la fois `verrou` et `declencheur` est refusé.
2. **Ordre des directives = ordre du jeu**, et non un tri par identifiant.
   Cet ordre est déjà canonique (fixé par le document haché) ; le contrat
   de l'engine §6 cite « la première directive qui refuse, dans l'ordre du
   jeu » ; les motifs v1 en dépendent. Les notifications, marques et rôles
   sont triés, et aucune `map` n'est parcourue sans tri.
3. **Entrée du registre absente, donc indéterminé.** Si
   `Appel.VersionRegistre` est vide, une cible ou une feuille qui porte sur
   `acces` ou `etiquette` est indéterminée, et `si_indetermine` (refuser par
   défaut) tranche. C'est l'option fermée par défaut. Même règle pour les
   étiquettes d'une consultation sans version de registre.
4. **Personnes concernées par empreinte** : `Appel.Sujets` est indexé par
   `sha256(id)` (`EmpreinteSujet`). Le journal ne porte jamais
   l'identifiant en clair, conformément au principe « le journal ne porte
   que des empreintes ».
5. **Quantificateurs** : sous `existe` ou `tous`, les pointeurs sont
   relatifs à l'élément, et `""` désigne l'élément lui-même. Les arguments
   racine ne sont pas accessibles depuis l'intérieur (il faut les combiner
   par `et` à l'extérieur). Au-delà de 10 000 éléments, le résultat est
   indéterminé.
6. **Nombres** : décimal exact, y compris pour les jeux v1 traduits. C'est
   la seule différence voulue avec la v1, et elle ne joue que sur des
   valeurs non représentables en `float64`. Un `float64` reçu est lu par sa
   représentation décimale la plus courte (0,1 = 0,1). L'exposant est borné
   à ±1 000 et la mantisse à 1 000 chiffres.
7. **Effets** : `refuser` ne se combine ni avec `attendre_validation` ni
   avec `caviarder`, qui seraient sans effet. Un caviardage qui ne retire
   rien ne déclenche rien, comme en v1. `notifier` et `marquer` sont émis
   aussi quand un indéterminé force le refus, mais jamais en observation.
8. **Conditions et caviardage** : les conditions voient les arguments déjà
   caviardés par les directives précédentes, comme en v1 : ce qui est
   évalué est ce qui serait transmis.
9. **`Validation.Roles`** : c'est l'union des `par_role` des directives
   appliquées en attente, et chaque rôle listé doit valider. Précision
   ajoutée au schéma (§8.3), sans changement de forme.
10. **Fuseaux** : copie figée de 21 fuseaux (IANA 2025b, prise dans le
    `zoneinfo.zip` de Go 1.25.0, pas dans le système), avec un manifeste
    sha256 vérifié par test. La liste est fermée : les fuseaux européens,
    ultramarins français et quelques places financières, plus UTC.
11. **`sous_agent_declare` absent = faux** (pas indéterminé) : c'est une
    déclaration, qui ne peut que resserrer.
12. **État intermédiaire après 5781dca** : le `Decide` v1 refusait tout appel
    ciblé par une directive v2 (fermé par défaut). Ce n'est plus le cas
    depuis dd18223.
13. Pas d'environnement virtuel Python créé : les tests Python n'ont besoin
    que de `unittest`, qui fait partie de la bibliothèque standard.

## 4. Blocages

Aucun. Aucun appel réseau, aucun appel Mistral.

## 5. Questions pour Manu (par urgence)

1. **Urgent, pour la session gate** : le gate ne passe encore ni
   `VersionRegistre`, ni `Acces`, ni `EtiquettesCible`, ni `Demandeur`, ni
   `Sujets`, ni les étiquettes et la `Source` des consultations. Tant que ce
   n'est pas fait, une directive v2 ciblant une étiquette ou un accès
   **refuse par défaut** (décision 3). Il faut aussi que le gate écrive les
   nouveaux champs du journal.
2. **Urgent, pour la session rejeu** : `journal.VersConsultations`,
   `DepuisDecision` et la reconstitution de l'`Appel` dans
   `rejeu.RejouerEntree` doivent recopier les champs v2. Sinon, une
   directive v2 qui lit ces entrées divergera au rejeu. Je ne les ai pas
   touchés : le brief n'autorise que l'ajout de champs dans `journal` et
   exclut `rejeu`.
3. L'ordre du jeu vous convient-il comme « ordre canonique trié »
   (décision 2) ?
4. La liste de 21 fuseaux suffit-elle ?
5. Faut-il permettre de lire les arguments racine depuis un quantificateur
   (décision 5) ?
6. Qui écrit les évaluateurs des évènements `ouverture_session`,
   `attestation_echouee` et `purge` ?

## 6. Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-v2
git log --oneline -3                       # dd18223, 5781dca, af46f0e
go vet ./... && go test -race -count=1 ./...
go test ./internal/directive -count=1 -v -run 'Scenario|Kleene|Repetition|Aleatoires|Rejeu_V1|WeekEnd_Fuseau|Purete|EmpreintesInchangees' 2>&1 | grep -E '^(---|ok|FAIL)|TZ='
(cd agents && python3 -m unittest discover -s tests -t .)
```

À lire : `docs/schema-directive-v2.md` (contrat), puis
`internal/directive/decide_v2.go` et `evaluer.go`.
