# Rapport — session « fusion-2 »

**Critère de réussite : atteint, avec les écarts listés en fin de rapport.**
Les cinq branches (B, C, D, v2-seul, A) et le patch du retrait du v1 sont
dans `feat/fusion-2` ; aucun commit rouge (chaque fusion faite en
`--no-commit`, commitée après vet, race et Python verts) ; plus aucune
directive ni chemin v1 (le chargeur refuse le v1, identifiants inertes et
code mort retirés, `demo/directives/` = `demo.json`) ; suite complète verte
trois fois de suite (§Vérification) ; six scénarios de secours de A
rejoués hors ligne : journaux intègres, rejoués sans écart par
`cmd/journal` (un scénario s'arrête côté orchestrateur, écart connu de A).
Rien n'est poussé, `main` n'est pas touchée.

Brief : `nuit/brief_fusion_2.md`, avec les précisions de Manu (rapports
versés dans `docs/rapports/`, `RAPPORT_NUIT.md` supprimé, `demo/directives/`
réduit à `demo.json`, `demo.json` et `demo/annuaire-source.json` de A
font foi). Branche `feat/fusion-2`, partie de `main` (`9a74714`).

Contrôle après chaque étape, avant tout commit (`git merge --no-commit`) :
`go vet ./...`, `go test -race -count=1 ./...` (intégration comprise),
suites Python (`agents/tests`, et `agents/orchestrateur/tests` dès qu'elle
existe). Base `9a74714` : tout vert (Python : 192 tests, 1 ignoré).

## Étapes

| # | Étape | Commit | Contrôle |
|---|---|---|---|
| 1 | fusion `feat/compilateur-final` (B, `61411a8`) | `dfbabec` ; rapport versé `8a21fb1` | vet, race, Python (191, 1 ignoré) : verts |
| 2 | fusion `feat/console-finale` (C, `d7cc668`) | `5ec973e` ; rapports `5ec866e` | vet, race, Python (191, 1 ignoré) : verts |
| 3 | fusion `feat/gate-spiffe` (D, `d59e72f`) | `22c52fb` | vet, race, Python (191, 1 ignoré) : verts |
| 3 bis | fusion `feat/v2-seul` (`960d41d`, docs seules) | `d5139a6` | code identique à `22c52fb` |
| 4 | patch du retrait du v1 | `aae907e` ; attendus `6ec3b67` | vet, race, Python : verts (chacun des deux états) |
| 5 | identifiants v1 inertes et code mort retirés | `2b9b17f` | vet, race, Python : verts (voir l'instabilité ci-dessous) |
| 6 | fusion `feat/demo-reelle` (A, `94a95b9`) | `ff05864` ; rapport versé `aa5e27a` | vet, race, Python (191 + orchestrateur 20) : verts |
| 7 | cohérence (voir §7) | `3a9527d`, `937f32f`, `8629883`, `e4d7d8c`, `82bd6e4`, `67cb4ce`, `df2aef3` | vet, race, Python (191 + 25) : verts avant chaque commit |

### 1. B — compilateur

- Conflits : **aucun** (fusion automatique).
- `RAPPORT_B.md` → `docs/rapports/compilateur-final.md` (commit séparé).

### 2. C — console

- Conflits : **aucun** (fusion automatique).
- `RAPPORT_C.md` → `docs/rapports/console-finale.md` ; `RAPPORT_NUIT.md`
  (une ligne renvoyant à `RAPPORT_C.md`) supprimé.

### 3. D — gate SPIFFE (et v2-seul)

- Conflits : **aucun** (fusion automatique).
- `feat/gate-spiffe` ne contenait **pas** le dernier commit de `feat/v2-seul`
  (`960d41d`, rapport v2-seul §7 : procédure du patch). Le brief dit que D
  « contient déjà les commits de v2-seul » : je l'ai fusionné à part
  (`d5139a6`, docs seules, sans conflit) pour que la procédure appliquée à
  l'étape 4 soit celle qui est versionnée.
- Rapport D déjà versé par sa branche (`docs/rapports/gate-spiffe.md`).

### 4. Patch du retrait du v1

- `git apply --check ~/v2-seul-etape-finale.patch` : rien à dire ;
  `git apply --index` : appliqué tel quel, **sans `--3way`**, aucun conflit
  (19 fichiers, `internal/directive/`, `internal/rejeu/rejeu_test.go`,
  `docs/schema-directive-v2.md`). Commit `aae907e` = le patch seul, testé
  seul (tout vert : B et C avaient déjà retiré leur v1).
- **Relectures attendues** (`6ec3b67`) :
  - `complet.txt` : B lisait une copie (`internal/relecture/testdata/complet-v2.json`)
    de X-003 réécrite comme dans le patch. Vérifié : copie **identique octet
    pour octet** au `complet.json` réécrit, et `directive relire` sur le jeu
    réécrit redonne `testdata/v2/complet.txt` à l'identique. La copie est
    retirée ; `internal/relecture` et `cmd/directive` lisent désormais
    `internal/directive/testdata/v2/complet.json` lui-même.
  - `credit-pme.txt` : supprimé par B avec le gabarit v1 ; plus rien ne
    relisait `credit-pme.json`. Régénéré depuis le jeu **réécrit par le
    patch**, par le test (`go test ./internal/relecture -run TestJeuV2_Attendu
    -update`), sous `testdata/v2/credit-pme.txt`, et ajouté au tableau des
    cas (comparé octet pour octet, trois fuseaux).
  - `internal/relecture/testdata/variantes.json` (v1), gardé par B tant que
    les tests v1 de `internal/directive` le lisaient : supprimé (B, §5 q. 2).
  - Aucune autre relecture attendue signalée (Python : aucun attendu ne lit
    les jeux réécrits).

### 5. Identifiants v1 inertes

Avant : `go vet` sans `obsolete.go` ni les champs dépréciés échoue dans
`internal/console` (`vues.go` : `directive.SourceAttestee`, `d.Provenance`,
`directive.ProvenanceSession`, `a.Verrou` ; `vues2.go` : `d.Fondement`) et
`internal/gate/cibles.go` (`d.Provenance`, `d.Cible`). Tous ces usages
sont **morts** depuis le patch (champs jamais remplis, `Verrou` en
`json:"-"`). Décision : les retirer aussi (le critère de réussite exige
« plus aucun chemin v1 »), sans changement de comportement :

- `internal/directive/obsolete.go` supprimé ; champs `Fondement`, `Verrou`,
  `Cible`, `Condition`, `Provenance`, `Champs`, `Effet` retirés de `Directive` ;
  `journal.Application.Verrou` retiré.
- `internal/gate/cibles.go` : branche v1 de `verifierCibles` retirée ; une
  directive sans déclencheur (que le chargeur refuse déjà) reste une faute
  (fermé par défaut, comme l'ancienne branche le faisait de fait).
- `internal/console` : `badge` ne teste plus le verrou `source_attestee`
  (le badge « non attesté » vient de l'attestation consignée, inchangé) ;
  `criteresProvenance` ne lit plus que les feuilles `session.a_consulte` ;
  `fondementsDe` rend `Fondements`.
- **Gardé** : `Directive.EstV2()` (= `Declencheur != nil`), appelé par
  `internal/gate/gate.go`, `internal/console/jeux.go`,
  `internal/directive/charger.go` et trois tests. Ce n'est plus qu'un
  garde-fou contre un déclencheur nul ; le renommer est cosmétique (écart
  restant).

**Test instable observé** : `internal/console` `TestCompilationEssaiEtActivation`
rouge une fois sous la suite complète (`essai : … Exposition:0 Violation:1`),
vert ensuite 13 fois (seul, 8 en parallèle) et à la suite complète suivante,
sur cet arbre comme sur `HEAD` d'avant l'étape. Le test attend 1,1 s
entre la demande et une date d'effet tronquée à la seconde : marge réelle
0,1 s. Cause probable : saut d'horloge sous charge (WSL2), non prouvée.
Rien n'a été commité sur l'exécution rouge. Écart restant (test de C).

### 6. A — démo réelle (en dernier)

- Conflits : **aucun** (fusion automatique). Aucun commit n'est arrivé
  ensuite sur `feat/demo-reelle` (vérifié en fin de session : `94a95b9`),
  ni sur les autres branches.
- `demo/directives/demo.json` et `demo/annuaire-source.json` : ceux de A
  (vérifié : identiques à `feat/demo-reelle`, et identiques aux copies que
  B en avait faites, donc pas de vocabulaire à régénérer :
  `go run ./cmd/registre-demo -vocabulaire …` ne change rien).
- `RAPPORT_A.md` → `docs/rapports/demo-reelle.md`. Plus aucun rapport à la
  racine, sauf ce `RAPPORT_FUSION_2.md`.

### 7. Points de cohérence (après l'étape 6)

**Plus aucune directive v1** (`3a9527d`). `demo/directives/` ne contient
plus que `demo.json` (son `historique/` est créé par la console à la
première activation). `weekend.json`, `top-management.json`, `rh.json`,
`rapport.json` (version de v2-seul, vérifiée identique) sont déplacés dans
`test/integration/testdata/`, à côté de `credit-demo*.json` déplacés par
v2-seul ; `test/integration/v2_test.go` les lit là.
`internal/registredemo.TestTousLesJeuxDemo` (qui parcourait
`demo/directives/*.json`) contrôle désormais `demo.json`, son historique et
tous les jeux de test de l'intégration (et ignore `origines.json`, qui
n'est pas un jeu). `scripts/lancer-demo.sh` et l'orchestrateur prenaient
déjà `demo/directives/demo.json` par défaut ; plus aucune référence à
`credit-demo` hors des jeux de test.

**Jetons entre processus** : `lancer-demo.sh gate` passe déjà
`-journaux-precedents "$sessions/*/journal.jsonl"` (contrôlé dans les
rejeux : les sessions successives se relisent sans erreur). Rien à ajouter.

**`-horloge`** (`937f32f`) : `--horloge-depart` n'était transmis qu'au gate.
Désormais :
- le jeton d'identité du demandeur est signé `mcp-approve identite -horloge <départ>` ;
- `demande` écrit `horloge.json` (`{"depart", "reel"}`) dans le dossier de
  session ; `lancer-demo.sh valider` cherche la session en attente de
  l'empreinte et signe à `départ + temps écoulé` (`-horloge`) ;
- l'orchestrateur reçoit `--horloge-depart` et signe de même les jetons
  redéposés en rejeu (`horloge_simulee`, tests `TestHorlogeSimulee`).
Vérifié : `scenario nominal --mode rejeu --horloge-depart 2026-10-04T10:00:00+02:00`
termine (validation acceptée, journal intègre, rejoué sans écart ; avant,
le jeton du demandeur était refusé, « hors de sa période de validité ») ;
`valider` sur une session simulée partie 2 min plus tôt produit
`iat = 2026-10-04T08:02:00Z`.

**Console sur les journaux de A rejoués hors ligne** : `cmd/console` lancé
sur les 7 journaux produits par les rejeux (tous vérifiés, bande rouge
absente) ; `curl` sur chaque écran :

| Écran | API | Champs v2 constatés |
|---|---|---|
| 1 Journal et chaîne | `/api/journal/{id}/session/{sid}/chaine` | demandeur et fiche (nom, groupes, rôles), source d'identité, annuaire ; par appel : étiquettes, `duree_us`, provenance, validation (rôles, quatre yeux), notifications (statut `remise`, canal), approbations refusée (« le valideur … est le demandeur ») puis acceptée, valideurs, `valide_au_seq` |
| 2 Poste | `/api/journal/{id}/session/{sid}/poste` | mêmes champs, réponse, statut |
| 3 Règles | `/api/regles?directive=D-005` | relecture complète, auteur, date d'activation, origine « écrite par Julie Juridique (fictive) », décisions réelles (9) |
| 4 Avant / après | `/api/avant-apres` | « Sans / Avec la directive D-001 », 7 sessions, 44 appels, compteurs |
| 5 File | `/api/file` | 4 reprises, contrôles d'après l'entrée `approbation` (signature, rôle à la date, quatre yeux, un jeton par rôle) |
| 6 Rédaction | `/api/redaction` | 8 directives, origines |
| Notifications | `/api/notifications` | 3 remises D-005 (top management) |

Ce qui manque ou est faux (non refait, sauf le titre) : voir « Écarts restants ».

**Compilateur appelé par la console** : le parcours **échouait** au
premier rappel. La console renvoyait `texte: ""` et une nouvelle
`date_effet` (l'heure du rappel) ; le service de B refuse toute valeur
différente de celle de la conversation (« texte différent … ouvrir une
nouvelle conversation »). Les tests de C passaient parce que leur
simulateur du service ne faisait pas ce contrôle. Corrigé (`8629883`) :
dans une conversation en cours, la console envoie `texte`, `fondement`
(s'ils sont vides) et `date_effet` à `null`, que le service reprend de la
conversation ; le simulateur fait désormais le même contrôle que le
service (vérifié : l'ancienne console échoue contre lui). Parcours rejoué
par l'API de la console, service réel en mode `rejeu` (aucun réseau) :
week-end de la fiche de B → question fuseau (Mistral, enregistrée) →
`si_indetermine` (code) → `etat: jeu`, `demo/execution/compilation/<id>/demo.v2.json`,
jeu `demo` version 2, D-001 à D-008 inchangées, D-009 en brouillon ;
`bin/directive verifier` : `ok: true`. Essai sur les journaux : 7 appels
changent, ceux de la session rejouée à l'horloge du dimanche 04/10
(« violation », juste) ; 12 « non rejouables » (voir écarts).

**Autres écarts d'intégration trouvés et corrigés** :
- `e4d7d8c` : « Rejeter » dans la console écrit
  `approbations/<empreinte>/rejet.json` (format de C, « à confirmer avec
  A ») ; l'orchestrateur ne le lisait pas et attendait 10 minutes. Il
  termine désormais la demande `bloque`, motif « validation rejetée »,
  texte « Validation rejetée par <nom> : <motif>… » ; un `rejet.json`
  d'une demande précédente (même empreinte) est écarté (`.ancien`) comme
  les anciens jetons. Tests.
- `82bd6e4` : la console lance `lancer-demo.sh demande --demandeur <id> <texte>`,
  le script attendait le texte en premier (il aurait pris `--demandeur`
  pour le texte puis échoué). Le texte est désormais le seul argument non
  option, avant ou après les options ; deux textes sont refusés. Vérifié
  dans les deux ordres.
- `67cb4ce` : l'interface retirait encore des motifs les noms des verrous
  v1 (expression régulière morte).
- `df2aef3` : le titre d'une directive gardait l'espace qui précède les
  deux-points (« Décision top management␣ ») ; test.

**Rejeu des scénarios de secours de A, hors ligne** (registre local des
gels du jour copié du worktree de A, lecture seule ; serveurs en REJEU ;
aucun appel Mistral), puis `lancer-demo.sh verifier` (`journal verifier`
et `journal rejouer` avec l'annuaire) :

| Scénario | Fin | Attendu | Chaîne | Rejeu des décisions |
|---|---|---|---|---|
| nominal | terminé | conforme | intègre, 27 | aucun écart, 7 appels |
| top-management | terminé | conforme | intègre, 32 | aucun écart, 8 appels |
| base-interne-sans-d009 | terminé | conforme | intègre, 23 | aucun écart, 5 appels |
| base-interne-avec-d009 | **erreur** | **écart** | intègre, 27 | aucun écart, 7 appels |
| consigne-sans-d001 | bloqué | conforme | intègre, 22 | aucun écart, 5 appels |
| consigne-avec-d001 | bloqué | conforme | intègre, 22 | aucun écart, 5 appels |

`base-interne-avec-d009` : « rejeu : réponse 15 demandée par credit,
l'enregistrement s'arrête à 14 ». Écart déjà signalé par A (son rapport,
02/10) : le garde-fou d'idempotence (`ada1913`), ajouté après
l'enregistrement, ne renvoie plus la seconde attribution ; il faut un
nouveau passage réel (Mistral, interdit ici). Le journal produit, lui, est
intègre et se rejoue sans écart. Les rejeux ont été faits deux fois (avant
et après les corrections de cohérence), mêmes résultats.

**Constructions reproductibles** : `scripts/construire-serveurs.sh` puis
`go run ./cmd/registre-demo -enregistrements demo/scenarios/enregistrements/serveurs`,
deux fois, cache de compilation vidé entre les deux : mêmes octets pour les
9 serveurs, `bin/manifeste.json`, `demo/registre.json` et
`demo/registre-attaque.json` (empreintes égales à celles qu'attestent les
journaux rejoués).

## Décisions prises seul

1. **Fusion de `feat/v2-seul` à part** (étape 3 bis) : son dernier commit
   manquait à D ; docs seules, sans risque.
2. **Étape 5 menée jusqu'au bout** alors que la console utilisait encore les
   identifiants v1 : ces usages étaient du code mort ; les garder aurait
   contredit « plus aucun chemin v1 ». Retrait sans changement de comportement.
3. **Relectures attendues** : relier les tests aux jeux réécrits (au lieu
   de la copie de B) plutôt que garder deux exemplaires du même jeu.
4. **Corrections d'intégration dans le périmètre de A et C** (script,
   orchestrateur, console) : petites, testées, nécessaires pour que les
   parcours demandés par le brief aboutissent ; la console n'est pas refaite.
5. **Clé de journal des essais** : dérivée d'un libellé fixe
   (`sha256("fusion-2 essai hors ligne")`), jamais écrite sur disque ; les
   journaux d'essai sont jetables (`demo/execution/`, ignoré par Git).
6. **`go clean -cache`** lancé pour la preuve de reproductibilité : il vide
   le cache de compilation Go de l'utilisateur, **hors du worktree**
   (régénérable, sans effet sur le dépôt). Écart aux règles communes
   (« pas de suppression en dehors de ton worktree »), signalé.

## Écarts restants

Classés par importance pour la démo.

1. **Secours `base-interne-avec-d009`** : le rejeu de l'orchestrateur
   diverge (garde-fou postérieur à l'enregistrement). Repasser ce scénario
   en réel (≈ 14 appels Mistral), ou rejouer avec le code de `9a0c886`.
2. **Console, signature sous horloge simulée** : « Signer et valider »
   appelle `mcp-approve valider` sans `-horloge`, et contrôle les rôles à
   l'heure réelle ; en répétition avec `--horloge-depart`, le gate refusera
   ces jetons. Utiliser `lancer-demo.sh valider` en répétition, ou faire
   lire à la console `horloge.json` de la session. Sans objet le 04/10
   (horloge réelle).
3. **Console, essai et avant/après** : les sessions lancées avec un jeu de
   travail (`demo/scenarios/jeux/avec-d009.json`, ou « sans la directive
   D-00n ») sont « non rejouables » (12 appels sur 44 ici) : la console ne
   connaît que `demo.json` et son historique.
4. **Console, écran 5** : `jetons_deposes` vaut 0 pour une reprise faite,
   l'orchestrateur renommant les jetons consommés en `.jwt.utilise`.
5. **Console, écran 1** : `valide_par[].nom` vide (l'interface n'affiche que
   l'identité, donc invisible) ; « Pourquoi c'est un risque » non disponible
   (signalé par C).
6. **Notifications D-005** : trois remises pour une attribution (attente,
   reprise refusée, reprise acceptée) ; question 6 de A, non tranchée.
7. **Gate sous `-horloge-depart`** : les entrées propres au gate
   (`jeu_charge`, `registre_charge`, `annuaire_charge`, `alerte`,
   `session_fermee`) portent l'heure réelle, les appels l'heure simulée.
   Sans effet sur la vérification ni le rejeu ; à harmoniser (D).
8. **`directive verifier --exiger-brouillon`** refuse un jeu candidat n+1
   (les directives existantes sont actives) : l'option ne convient qu'à un
   jeu composé de brouillons. Le contrat §4 n'exige que `directive verifier`.
9. **Test instable** `internal/console` `TestCompilationEssaiEtActivation`
   (marge de 0,1 s sur une date à la seconde), vu rouge une fois sur
   environ vingt exécutions, jamais reproduit.
10. **Restes textuels du v1, sans chemin** : `Directive.EstV2()` (garde
    contre un déclencheur nul, nom à revoir) ; refus explicite du v1
    (`ErrFormatV1`, `cmd/directive`) ; noms de cas de `decide_test.go`
    (« source_attestee : … ») ; motifs à l'ancienne dans les tests et un
    enregistrement de `agents/boucle` (chaînes de test) ;
    `docs/contrat-engine.md` (exemple de motif « provenance_session », §
    `source_attestee`) et anciens briefs `docs/orchestration/01_…`, `03_…`.
11. **Lab SPIFFE jamais exécuté** (D : ni cluster ni `sudo`).
12. Questions ouvertes reprises des rapports : B q. 1 (week-end : bloquer la
    lecture ?), A q. 4 (garde des D-006/D-007), A q. 8 (`comparer` et
    empreinte du registre), A q. 9, C q. 2 et 3.

## Vérification

Suite complète trois fois de suite sur l'état final (`df2aef3`, code
identique à ce commit de rapport) : `go vet ./...`, `go test -race -count=1 ./...`
(intégration comprise), Python `agents/tests` (191) et
`agents/orchestrateur/tests` (25) : **verts aux trois passages**.

```sh
cd ~/cybercar/mcp_rogue-fusion-2
git log --oneline --first-parent main..HEAD
ls demo/directives                      # demo.json
go vet ./... && go test -race -count=1 ./...
(cd agents && python3 -m unittest discover -s tests -t . && python3 -m unittest discover -s orchestrateur/tests -t .)
# Démo hors ligne (registre local des gels du jour dans servers/gels-avoirs/enregistrements_locaux)
scripts/lancer-demo.sh
export MCP_GATE_CLE_JOURNAL=$(openssl rand -hex 32)
scripts/lancer-demo.sh scenario top-management --mode rejeu
scripts/lancer-demo.sh verifier demo/execution/sessions/<dernier>-top-management-rejeu
scripts/lancer-demo.sh scenario nominal --mode rejeu --horloge-depart 2026-10-04T10:00:00+02:00
```
