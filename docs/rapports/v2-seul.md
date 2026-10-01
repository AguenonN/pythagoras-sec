# Rapport — session « v2-seul » : une seule forme de directive

Branche `feat/v2-seul`, worktree `mcp_rogue-v2-seul`. Rapport tenu à jour
après chaque commit sous `RAPPORT_V2_SEUL.md` (nom imposé par le brief, à
la place de `RAPPORT_NUIT.md` des règles communes), versé ici,
`docs/rapports/v2-seul.md`, au dernier commit de la session (commit de
docs seules : l'étape 3 reste dans l'arbre de travail, §2).

État de départ (9a74714) : `go vet ./...` vert, `go test -race -count=1 ./...`
vert, suite Python `agents/` verte (192 tests, 1 ignoré).

## 1. Inventaire de ce qui dépend du format v1 (avant tout changement)

Légende : **S** supprimé, **R** réécrit en v2, **G** régénéré par les
tests, **C** conservé (justification), **↗** hors périmètre, signalé.

### 1.1 Code de `internal/directive`

| Élément | Où | Sort |
|---|---|---|
| Types des six verrous (`Verrou`, `Interdiction`, `SeuilArgument`, `ProvenanceSession`, `ApprobationHumaine`, `ChampsAutorises`, `SourceAttestee`) | `schema.go` | S de la logique ; voir §3 (identifiants gardés inertes le temps que B/C/D retirent leurs usages) |
| Champs v1 de `Directive` : `Fondement` (singulier), `Verrou`, `Cible`, `Condition []Condition`, `Provenance`, `Champs`, `Effet`, `verrouV1` | `schema.go` | S de la logique (jamais remplis) ; voir §3 |
| Types `Cible`, `Provenance`, `Champs` | `schema.go` | idem |
| `OpDans` (`"in"`, opérateur v1) | `schema.go`, `evaluer.go`, `charger_v2.go` | S de l'évaluation et du chargeur (« in » refusé comme opérateur inconnu) ; identifiant gardé pour `internal/relecture` |
| `directiveV1`, branche v1 de `MarshalJSON` / `UnmarshalJSON`, discriminant `verrou` | `directive_json.go` | S ; une directive sans `declencheur`, ou un jeu qui porte `verrou`/`fondement`, est refusé avec « format de directive v1 non pris en charge : réécrire en v2 » |
| `validerDirective`, `validerCondition`, `parVerrou`, `exigences` | `charger.go` | S |
| `ConvertirV1`, `VerrouV1` | `convertir.go` | S (fichier supprimé) |
| `notesV1`, `provenanceCorrespond`, `nombre`, branche `v1` de `decider`, `Application.Verrou`, citation du verrou dans `motif` | `decide.go`, `decide_v2.go` | S |
| `EstV2` | `schema.go` | C (toujours vrai après chargement ; appelé par `internal/gate`, `internal/relecture`) — voir §3 |
| Textes « celui de la v1 » des feuilles `argument` et `attestation` | `evaluer.go` | C (les textes restent, seul le commentaire change) |

### 1.2 Tests de `internal/directive`

| Test / fichier | Sort |
|---|---|
| `decide_v1_ref_test.go` (copie figée du `Decide` v1, `DecideV1Ref`) | S |
| `TestRejeu_V1IdentiqueSousV2` (test d'équivalence v1/v2), `sansChampsV2`, `directivesV1Aleatoires`, `jeuxV1DuDepot` | S (remplacé par le test 5 de déterminisme sur journaux régénérés, §6) |
| `TestConvertirV1_SixVerrous` | S |
| `TestChargerV1_EmpreintesInchangees` | S (remplacé par un test de refus du v1) |
| `TestCharger_Refus`, `TestCharger_Accepte`, `TestAlertes` (jeux v1 en ligne) | R (cas v1 propres à un verrou : retirés, la règle équivalente v2 est déjà dans `TestChargerV2_Refus` ; cas d'enveloppe : réécrits en v2) |
| `TestDecide_Table`, `TestDecide_ScenarioDemo`, aides `dir`/`seuil`/`champs`/`provenance` | R, mêmes attentes (effet, directives appliquées, arguments, motif sans le nom du verrou) |
| `TestMotifs_SansValeurDArgument` | R |
| `TestChargerV2_Accepte` (directive v1 dans `complet.json`) | R |
| `TestDecide_Repetition1000` (`toutesLesDirectives` y mêle `credit-pme.json` v1) | R (jeu v2) |
| `TestArbresAleatoires_SansPaniqueNiDivergence` (génère l'opérateur `in`) | C, `in` reste généré : il doit valoir indéterminé sans panique |
| `testdata/credit-pme.json` (v1) | R en v2 (même chemin, mêmes identifiants) |
| `testdata/v2/complet.json`, directive X-003 v1 | R en v2 |

### 1.3 Journal, rejeu

| Élément | Sort |
|---|---|
| `journal.Application.Verrou` (`json:"verrou"`, non `omitempty`) | S du journal (plus jamais écrit) ; champ Go gardé inerte pour `internal/console` (§3) |
| `DepuisDecision` / `VersDirective` : recopie de `Verrou` | S |
| `trivalent` : « un journal v1 n'en porte pas » | R (commentaire ; un résultat absent reste faux) |
| `journal/v2.go` : « un journal v1 se relit à l'identique » | R (commentaire) |
| `journal_test.go`, `v2_test.go` (`Verrou: directive.ApprobationHumaine`, application « v1 ») | R |
| `rejeu_test.go` (scénario sur `credit-pme.json` v1) | R (jeu v2), journaux G |
| `rejeu/v2_test.go` (« un journal v1 ne porte aucun champ v2 ») | R (un appel sans compléments) |
| Rejeu : chargement des jeux du catalogue | passe par `directive.Charger`/`Valider` : un jeu v1 est refusé |

### 1.4 Gate (tests seulement), `test/gate`, `test/integration`, `internal/registredemo`

| Fichier | Dépendance v1 | Sort |
|---|---|---|
| `internal/gate/gate_test.go` | `jeuTest` (4 directives v1) | R, journaux G |
| `internal/gate/rechargement_test.go` | `jeuTestV2` (+ D-009 v1) | R |
| `internal/gate/cibles_test.go` | `jeuCible`, `jeuProvenance` (v1) | R (la provenance devient `session.a_consulte` ; message « dans sa condition ») |
| `internal/gate/reponses_test.go`, `notification_test.go` | via `jeuTest` | R (attentes de motif) |
| `test/gate/gate_e2e_test.go`, `imposteur_test.go`, `rechargement_test.go`, `robustesse_test.go`, `alteration_test.go` | jeux v1 en ligne, motifs « provenance_session », « source_attestee » | R, journaux G |
| `test/integration/integration_test.go` | `demo/directives/credit-demo.json` (v1), motifs « seuil_argument », « source_attestee » | R ; jeu déplacé en `test/integration/testdata/credit-demo.json` (v2) |
| `test/integration/base_perimee_v2_test.go` | `credit-demo.json` et `credit-demo-v2.json` (v1) | R ; jeu déplacé en `test/integration/testdata/credit-demo-v2.json` (v2) |
| `test/integration/v2_test.go` | jeux `weekend`, `top-management`, `rh`, `rapport` (déjà v2) | C |
| `internal/registredemo/directives_test.go` | `credit-demo.json`, `directive.Verrou` | R |

### 1.5 Jeux de `demo/directives/` du périmètre

| Fichier | Format | Sort |
|---|---|---|
| `credit-demo.json` | v1 | R en v2, déplacé dans `test/integration/testdata/` |
| `credit-demo-v2.json` | v1 (le « v2 » est la version 2 du jeu) | R en v2, déplacé dans `test/integration/testdata/` |
| `weekend.json`, `top-management.json`, `rh.json`, `rapport.json` | déjà v2 | C (rien à réécrire) |

### 1.6 Journaux et coffres

Aucun journal ni coffre n'est versionné (`git ls-files` : aucun `.jsonl`).
Tous sont produits par les tests dans `t.TempDir()` (ou
`MCP_CONSERVER_JOURNAUX`) : ils sont **régénérés** à chaque exécution,
sous les jeux v2.

### 1.7 Hors périmètre, dépendant du v1 (↗, non modifié)

| Session | Fichier | Dépendance |
|---|---|---|
| B | `internal/relecture/relecture.go` | gabarit v1 (`d.Verrou`, `d.Cible`, `d.Provenance`, `d.Champs`, `OpDans`…) |
| B | `internal/relecture/relecture_test.go`, `relecture_v2_test.go`, `testdata/variantes.json` | lisent `internal/directive/testdata/credit-pme.json` et `variantes.json` (v1), goldens v1 |
| B | `cmd/directive/main_test.go`, `main_v2_test.go` | lisent `credit-pme.json` (v1), `complet.json` (mêlé) |
| B | `agents/` (`compilateur.py`, `outils.py`, enregistrements, `test_demo.py --v1`, `test_compilateur.py`, `test_scenarios.py`) | parcours v1 |
| C | `internal/console/vues.go`, `vues2.go` | `a.Verrou`, `d.Provenance`, `d.Fondement`, `directive.SourceAttestee`, `directive.ProvenanceSession` |
| C | `internal/console/exemples/credit-demo-v1.json`, `credit-demo-v2-strict.json` | jeux v1 embarqués |
| D | `internal/gate/cibles.go` | branche v1 de `verifierCibles` (`d.Provenance`, `d.Cible`) |
| D | `internal/gate/gate.go` | `d.EstV2()` |
| A | `scripts/lancer-demo.sh` | défaut `credit-demo` dans `demo/directives/` |

## 2. Ce qui est fait

| Commit | Contenu | Contrôle |
|---|---|---|
| 713bc7a | Inventaire (§1) | docs seules ; base verte |
| dc2e65f | Étape 2 : jeux v1 des tests réécrits en v2, **mêmes attentes** : `jeuTest`/`jeuTestV2`/`jeuCible`/`jeuProvenance` (gate), `jeuProvenance`/`jeuSansProvenance`/`jeuImposteur`/`jeuProvenanceV2` (`test/gate`), aides de `TestDecide_Table` et `TestMotifs_SansValeurDArgument`, `TestAlertes`. `demo/directives/credit-demo.json` et `credit-demo-v2.json` réécrits en v2 et déplacés dans `test/integration/testdata/` ; intégration et `internal/registredemo` les lisent là. | vet, `go test -race -count=1 ./...`, Python : verts |
| efc0ddf | Étape 4 : `journal.Application.Verrou` n'est plus écrit ni lu (`json:"-"`) ; `DepuisDecision`/`VersDirective` ne le recopient plus ; le rejeu (`sorties`, ex-`sortiesV2`) ne dispense plus de comparer un résultat d'application absent (c'était la branche « journal antérieur à la v2 ») : l'effacer est une altération (nouveau cas dans `TestV2_AlterationsDetectees`). | vet, race, Python : verts |
| 02e3f59 | Étape 6 : `test/integration` vérifie et rejoue **tout** journal régénéré (`verifier`, ou au nettoyage du test pour un banc sans appel), et `TestMain` échoue si un seul y échappe (« rejeu : 26 journaux régénérés, tous vérifiés et rejoués sans écart »). Le `TZ` de la machine passe désormais au gate lancé par le test, pour que `TZ=Asia/Tokyo go test` éprouve vraiment le gate. | vet, race, Python : verts |

Les commits de rapport (0a5f512, b8de5a4, et le versement de ce rapport)
ne contiennent que de la documentation. L'état commité final a été
recontrôlé sur une copie (`git archive HEAD`) : `go vet ./...`,
`go test -race -count=1 ./...` et la suite Python, tous verts.

**Preuve du « même comportement »** (étape 2) :

- Les tests réécrits gardent leurs attentes d'origine : effet, directives
  appliquées, arguments transmis, champs retirés, empreintes, directive
  citée et fondement. Seule différence voulue : le motif ne cite plus le
  verrou (`refusé par D-001 v1 (RGPD art. 22)` au lieu de
  `refusé par D-001 v1 (provenance_session, RGPD art. 22)`), comme le
  schéma v2 le prévoit (§8.3) ; les tests vérifient désormais la phrase
  exacte au lieu de trois fragments.
- Contrôle croisé ponctuel, avant suppression de l'oracle v1 : sur
  80 000 décisions aléatoires (graine fixe), `DecideV1Ref` sur chaque jeu
  v1 (`credit-pme`, `credit-demo` v1 et v2) et `Decide` sur sa réécriture
  v2 rendent le même effet, les mêmes arguments, la même empreinte
  transmise, les mêmes applications (id, version, déclenchée, appliquée,
  effet, indéterminée, champs retirés, fondement, statut, échéance) et le
  même motif au nom du verrou près. Toutes les directives s'y déclenchent
  (D-001 : 129 à 145 fois, D-005 : 43 fois, refus, caviardages, attentes).
  Ce test dépendait de l'oracle v1 qui disparaît ; il n'est pas versionné.
  La conversion suit la table §9 du schéma (traduction mécanique).

### Étape 3 (et fin des étapes 1, 5) : prête, **non commitée**

Le retrait du v1 dans `internal/directive` est fait dans l'arbre de
travail du worktree, vérifié, et **arrêté avant commit** parce qu'il rend
rouges des tests qui ne sont pas à moi (§4). Le même changement est
enregistré dans `nuit/v2-seul-etape-finale.patch` (s'applique sur
`02e3f59`, contrôlé par `git apply --check`).

Contenu :

- `convertir.go` (`ConvertirV1`, `VerrouV1`) et `decide_v1_ref_test.go`
  (copie figée du `Decide` v1) supprimés ; `TestRejeu_V1IdentiqueSousV2`,
  `TestConvertirV1_SixVerrous`, `TestChargerV1_EmpreintesInchangees`
  supprimés.
- Chargeur : `validerDirective`, `validerCondition`, `parVerrou`
  supprimés ; `directiveV1` et la branche v1 de `MarshalJSON` /
  `UnmarshalJSON` supprimées. Un document dont une directive porte
  `verrou` ou n'a pas de `declencheur` est refusé :
  `directive: directive 2: format de directive v1 non pris en charge :
  réécrire en v2`, une ligne par directive fautive, par **position**,
  sans rien citer du document (ni id, ni texte, ni verrou) ;
  `errors.Is(err, directive.ErrFormatV1)`. `Valider` refuse de même un
  jeu construit à la main. Nouveau test `TestCharger_RefuseV1`.
- `Decide` : plus de traduction, plus de `notesV1`, plus de citation du
  verrou dans le motif ; `Application.Verrou` supprimé ; l'opérateur `in`
  n'est plus évalué (indéterminé) et le chargeur le refuse (« écrire
  "dans" »). Une directive sans déclencheur passée à la main à `Decide`
  est un refus (fermé par défaut, avant : `ConvertirV1` faisait de même
  pour un verrou inconnu).
- `testdata/credit-pme.json` réécrit en v2 (mêmes identifiants, même
  chemin) ; directive X-003 de `testdata/v2/complet.json` réécrite en v2.
  `TestCharger_Refus` / `TestCharger_Accepte` réécrits en v2 (les cas
  propres à un verrou disparaissent : leurs règles v2 sont dans
  `TestChargerV2_Refus`).
- `docs/schema-directive-v2.md` : §9 remplacé par une ligne
  (« Retiré : un document v1 est refusé au chargement… ») ; les phrases
  devenues fausses (§1, §2 discriminant, §8.3 motif v1, §8.5 journal v1)
  corrigées a minima et marquées.

Contrôle sur cet arbre : `go vet ./...` vert ; `go test -race -count=1`
vert sur **tout mon périmètre** (`internal/directive`, `internal/journal`,
`internal/rejeu`, `internal/gate`, `internal/registredemo`, `test`,
`test/gate`, `test/integration`) ; les cinq tests de déterminisme verts :
`TestPurete_*`, `TestDecide_Repetition1000`,
`TestArbresAleatoires_SansPaniqueNiDivergence`, `TestKleene_*`, et le
rejeu de tous les journaux régénérés (26 en intégration, plus le
`rejouer` de chaque banc de `internal/gate` et `test/gate`) ; week-end
identique sous `TZ=UTC` et `TZ=Asia/Tokyo` (`TestWeekEnd_FuseauMachine`
et `test/integration` `TestWeekEnd`).

`git grep -n -i "verrou\|ConvertirV1\|v1"` sur le périmètre, après
l'étape 3 : 85 lignes, toutes justifiées :

1. **Le refus lui-même** : `ErrFormatV1`, `estV1`, `refuserV1`, leurs
   commentaires, `TestCharger_RefuseV1`, le motif « directive sans
   déclencheur (format v1 non pris en charge) ».
2. **Numéros de version, pas de schéma** : `D-001 v1`, `gate-test v1`,
   `registre test v1`, `retour arrière de v2 à v1`, variables `v1, v2`
   des tests de rechargement (versions 1 et 2 d'un jeu) — c'est le
   format des motifs (`id vN`).
3. **Le verrou de session** (autre sens) : `TestPipeline_VerrouRendu`,
   commentaires de `gate_test.go` et `robustesse_test.go`.
4. **Identifiants gardés inertes pour B/C/D** (décision 2) :
   `obsolete.go`, champs dépréciés de `Directive` (`schema.go`),
   `journal.Application.Verrou` (`json:"-"`), et leurs commentaires.
5. Commentaire historique de `internal/gate/v2_test.go` (« n'est plus
   pris pour un journal d'avant la v2 »).

## 3. Décisions prises seul

1. **Nom du rapport** : `RAPPORT_V2_SEUL.md` (brief) plutôt que
   `RAPPORT_NUIT.md` (règles communes) ; le brief est plus précis. Versé
   dans `docs/rapports/v2-seul.md` au dernier commit, comme demandé.
2. **Types v1 gardés inertes, au lieu d'être supprimés.** Le brief dit
   « suppression des types des six verrous ». Les supprimer casse la
   **compilation** de paquets qui ne sont pas à moi :
   `internal/relecture/relecture.go` (B), `internal/console/vues.go`,
   `vues2.go` (C), `internal/gate/cibles.go` (D) — et, par
   `internal/gate`, mes propres tests du gate, de `test/gate` et de
   l'intégration. J'ai donc supprimé tout **chemin** v1 (chargement,
   conversion, évaluation, motifs, journal, rejeu) et gardé les seuls
   **noms** que ces paquets citent, regroupés et marqués `Deprecated:` :
   `Verrou` et ses six constantes, `Cible`, `Provenance`, `Champs`,
   `OpDans` (`internal/directive/obsolete.go`), les champs `Fondement`,
   `Verrou`, `Cible`, `Condition`, `Provenance`, `Champs`, `Effet` de
   `Directive` (jamais remplis, jamais lus), `EstV2` (toujours vrai pour
   une directive chargée), `journal.Application.Verrou` (`json:"-"`).
   À supprimer dès que B, C et D ont retiré leurs usages : c'est un
   commit de quelques lignes, sans effet sur le comportement.
   **Écart signalé au brief, contrats inchangés.**
3. **Étape 3 non commitée** (§4) : la consigne « arrête-toi sur tout test
   rouge avant de commiter » prime ; le travail est dans l'arbre et dans
   `nuit/v2-seul-etape-finale.patch`.
4. **Motifs** : un refus cite `D-00n vN (fondement)`, sans le verrou ; les
   tests réécrits vérifient la phrase exacte (`refusé par D-001 v1 (RGPD
   art. 22)`). C'est le motif v2 du schéma (§8.3), pas une nouveauté.
5. **Conversion des jeux** : mécanique, table §9 du schéma (script
   `nuit/v1v2.py`), une feuille seule sans `et`/`ou` d'un élément
   (équivalent en logique de Kleene) ; contrôlée par 80 000 décisions
   croisées contre l'oracle v1 avant sa suppression (§2 ; test et sortie
   dans `nuit/controle-croise-*`).
6. **`credit-demo-v2.json`** garde son nom (imposé par le brief) ; le
   « v2 » y est la version 2 du jeu. Dans `base_perimee_test.go` (renommé
   depuis `base_perimee_v2_test.go`), variables et messages disent
   « version 2 » pour lever l'ambiguïté.
7. **Rejeu** : l'absence de `resultat` sur une application n'est plus
   tolérée (c'était la dispense « journal antérieur à la v2 ») ; c'est le
   seul changement de comportement du rejeu, plus strict.
8. **Journaux** : aucun n'était versionné ; tous sont produits par les
   tests (`t.TempDir`, ou `MCP_CONSERVER_JOURNAUX` pour la console).
   « Régénérer » = les tests les produisent sous les jeux v2 à chaque
   exécution, et l'intégration exige que chacun se vérifie et se rejoue.
9. **`docs/schema-directive-v2.md`** (« figé ») : je n'ai changé que ce que
   le brief ordonne (§9) et les phrases devenues fausses, chaque retouche
   étant marquée « retrait du v1 (session v2-seul) ».

## 4. Blocages

**Le critère « un document v1 est refusé au chargement » et le critère
« toute la suite Go verte » sont incompatibles dans cette branche seule.**
Dès que le chargeur refuse le v1, ces tests, hors de mon périmètre,
deviennent rouges (ils lisent des jeux v1 ou comparent à une relecture
v1). Je ne les ai pas modifiés.

Session **B** (compilateur, relecture) :

| Paquet / fichier | Tests rouges | Cause |
|---|---|---|
| `cmd/directive/main_test.go` | `TestVerifier_ToutesLesErreurs`, `TestRelire`, `TestVerifier_ExigerBrouillon_JeuInvalide` | jeux v1 en ligne ; `internal/directive/testdata/credit-pme.json` (désormais v2) comparé à `internal/relecture/testdata/credit-pme.txt` (relecture v1) |
| `cmd/directive/main_v2_test.go` | `TestV2_VerifierEtRelire/complet` | X-003 de `complet.json` désormais v2 : golden `internal/relecture/testdata/v2/complet.txt` à régénérer |
| `internal/relecture/relecture_test.go` | `TestJeu_Attendu` (credit-pme, variantes), `TestDirective_CreditPME`, `TestRegle_Variantes` | gabarit v1 ; `testdata/variantes.json` (v1, à B) refusé |
| `internal/relecture/relecture_v2_test.go` | `TestJeuV2_Attendu/complet`, `TestJeuV2_FuseauxMachine`, `TestJeuV1_Inchange` | idem ; `TestJeuV1_Inchange` teste le v1 lui-même |
| `agents/tests/` (Python) | `test_cli` : `test_jeu_valide_non_marque` ; `test_compilateur` : `test_correction_unique_reussie`, `test_deux_propositions`, `test_directive_valide`, `test_ignorer_hors_question_du_code_sans_effet`, `test_outils_bornes_par_le_catalogue`, `test_second_echec_rendu_au_juriste`, `test_si_indetermine_question_du_code`, `test_si_indetermine_refuser_ou_ignorer`, `test_si_indetermine_reponse_invalide_redemandee` ; `test_demo` : `test_parcours_complet`, `test_question_du_code_fermee_et_consignee`, `test_reponse_vide_redemandee` ; `test_directive_go` : `test_brouillon_accepte`, `test_hors_brouillon_refuse_par_le_compilateur` ; `test_scenarios` : `test_champs_autorises_siren`, `test_completee_directive_acceptee_et_relecture_stable`, `test_montant_donne_par_le_juriste`, `test_erreurs_renvoyees_une_fois_puis_acceptee` (19) | parcours v1 du compilateur, vérifié par `directive verifier` qui refuse désormais le v1 |

Session **C** (console) :

| Paquet / fichier | Tests rouges | Cause |
|---|---|---|
| `internal/console` | `TestChaineEtProvenance`, `TestChaineRompueEnRouge`, `TestSansCleNonVerifiable`, `TestLigneEnCoursDEcritureIgnoree`, `TestPosteEtCout`, `TestRegles`, `TestAvantApres`, `TestFile`, `TestRelire` | jeux intégrés `internal/console/exemples/credit-demo-v1.json` et `credit-demo-v2-strict.json` au format v1, refusés au chargement (la console démarre alors sans jeu intégré) |

Également à signaler, sans test rouge :

- **A** : `scripts/lancer-demo.sh` prend par défaut `credit-demo` dans
  `demo/directives/` ; ce fichier est devenu un jeu de test
  (`test/integration/testdata/`, commit dc2e65f, demandé par le brief).
  Le jeu de démo est `demo.json` (session A).
- **C** : `internal/console/web.go` dit que `credit-demo-v1.json` est une
  copie de `demo/directives/credit-demo.json` ; la console lit encore
  `a.Verrou` (toujours vide désormais), `d.Provenance`, `d.Fondement`,
  `directive.SourceAttestee`, `directive.ProvenanceSession`.
- **D** : `internal/gate/cibles.go`, branche v1 de `verifierCibles`
  (`d.Provenance`, `d.Cible`), désormais morte ; `gate.go` appelle
  `d.EstV2()` (toujours vrai). Le message « attend dans sa provenance »
  ne sort plus : c'est « attend dans sa condition » (v2) ; tests du gate
  adaptés.

## 5. Questions pour Manu

1. **(Urgent, avant les fusions)** Ordre de fusion : l'étape 3 (patch ou
   commit) ne peut passer au vert qu'avec, ou après, les retraits du v1 de
   B (relecture, `cmd/directive`, `agents/`) et de C (exemples de la
   console). Préférez-vous (a) fusionner B et C d'abord, puis appliquer
   `nuit/v2-seul-etape-finale.patch` ; ou (b) appliquer le patch ici
   (`git apply` ou commit de l'arbre tel quel) et laisser B/C se mettre
   au vert dessus ?
2. **(Après les fusions)** Supprimer les identifiants inertes
   (`obsolete.go`, champs dépréciés de `Directive`,
   `journal.Application.Verrou`) une fois que plus personne ne les nomme
   (`go build ./...` le dira) — à me confier ou à la dernière session.
3. **(Faible)** `docs/schema-directive-v2.md` se dit « figé » : les
   retouches du retrait du v1 vous conviennent-elles telles quelles, ou
   voulez-vous un document « v2.1 » ?

## 6. Vérifier en 5 minutes

```sh
cd mcp_rogue-v2-seul
git log --oneline 9a74714..HEAD          # commits de la session
git status --short                        # étape 3 en cours (non commitée)
go vet ./...
# Mon périmètre, avec l'étape 3 :
go test -race -count=1 ./internal/directive ./internal/journal ./internal/rejeu \
  ./internal/gate ./internal/registredemo ./test/...
# Refus du v1 :
go test -count=1 -run 'TestCharger_RefuseV1' -v ./internal/directive
# Déterminisme (5) et week-end sous deux fuseaux :
go test -count=1 -v -run 'TestPurete_|TestDecide_Repetition1000|TestArbresAleatoires|TestKleene_|TestWeekEnd_FuseauMachine' ./internal/directive
go test -count=1 -v ./test/integration | grep '^rejeu'   # 26 journaux, tous rejoués
for tz in UTC Asia/Tokyo; do TZ=$tz go test -count=1 -run TestWeekEnd ./test/integration; done
# Les rouges attendus hors périmètre (§4) :
go test -count=1 ./cmd/directive ./internal/relecture ./internal/console
# Sans l'étape 3 (état commité, tout vert), sur une copie hors du worktree :
#   d=$(mktemp -d) && git archive HEAD | tar -x -C "$d" && (cd "$d" && go test -race -count=1 ./...)
```
