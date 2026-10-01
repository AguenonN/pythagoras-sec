# Rapport — session « v2-seul » : une seule forme de directive

Branche `feat/v2-seul`, worktree `mcp_rogue-v2-seul`. Rapport tenu à jour
après chaque commit (nom imposé par le brief, à la place de
`RAPPORT_NUIT.md` des règles communes).

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

(mis à jour après chaque commit)

## 3. Décisions prises seul

1. **Nom du rapport** : `RAPPORT_V2_SEUL.md` (brief) plutôt que
   `RAPPORT_NUIT.md` (règles communes) ; le brief est plus précis.

## 4. Blocages

## 5. Questions pour Manu

## 6. Vérifier en 5 minutes
