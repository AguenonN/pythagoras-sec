# Fusion n° 3 : kit-extension, finition, analyste, secteurs

Branche `feat/fusion-3`, base `main` = `5b83cd5` (tag `avant-fusion-3`). Les quatre branches sont fusionnées avec `git merge --no-ff --no-commit`, commitées seulement après `go vet ./...`, `go test -race -count=1 ./...` (intégration comprise) et la suite Python verts. Aucun commit de fusion rouge. `main` n'a pas été touchée, rien n'a été poussé.

## 1. Fusions

| Étape | Branche | Commit | Résultat |
|---|---|---|---|
| 1 | `feat/kit-extension` | `c8af629` | aucun conflit ; vet, race, Python (191 + 25) verts |
| 2 | `feat/finition` | `45c041c` | aucun conflit ; `RAPPORT_NUIT.md` absent de la racine ; verts |
| 3 | `feat/analyste` | `8ac2a63` | `RAPPORT_NUIT.md` à la racine déplacé en `docs/rapports/analyste-resume.md` ; verts (Python : + 68 tests de l'Analyste) |
| 4 | `feat/secteurs` | `eeda8a0` | `RAPPORT_NUIT.md` déplacé en `docs/rapports/secteurs-resume.md` ; deux corrections de tests (§2) ; verts |
| suite | docs, Analyste | `24d3913`, `6d8b7ab` | contrat §6, `secteurs.md`, journaux D-023 |

Aucune des quatre branches n'a reçu de commit pendant la fusion (`git log HEAD..feat/<branche>` vide pour les quatre).

### Conflits, fichier par fichier

Git n'a produit **aucun conflit textuel**. Les points du brief :

- `RAPPORT_NUIT.md` : n'existe à la racine que par `feat/analyste` et `feat/secteurs` (correction du brief). Aucun rapport à la racine ; contenus déplacés en `docs/rapports/analyste-resume.md` et `docs/rapports/secteurs-resume.md` (`git mv`, historique conservé). `docs/rapports/finition-resume.md` gardé tel quel.
- `agents/compilateur/service.py` (kit-extension) et numérotation du compilateur (secteurs) : fusion automatique, les deux conservés. Test D-009 / D-010 : `tests/test_service_scene.py` (9 tests) passe sur le `demo.json` final ; « base interne » donne D-009 (version 5, 29 directives existantes), « week-end » D-010 (version n+2 après activation de D-009). `test/extension` (kit) passe aussi.
- Registre, manifeste, `schema.json`, vocabulaire : rien résolu à la main. `scripts/construire-serveurs.sh` lancé deux fois : même `bin/manifeste.json` (`3fd3a109…bf0d8`), aucun `schema.json` modifié. `go run ./cmd/registre-demo -gels … -catalogue …` et `-vocabulaire … [-etendu]` : catalogue, vocabulaire et registres identiques aux fichiers versionnés.

## 2. Corrections faites pendant la fusion, et pourquoi

Deux familles de tests rouges après l'étape 4, toutes deux causées par l'état enregistré et non par un défaut de code.

1. **`test/extension/kit_test.go`, recette « directive »** : attendait la version 2 du jeu et l'archive `demo.v1.json`. Le `demo.json` fusionné est en version 4 (secteurs). Le test lit maintenant la version du jeu avant la mise en vigueur et attend `version+1` et `demo.v<version>.json`.
2. **Tests de l'Analyste (8 échecs)** : les réponses enregistrées de Mistral sont indexées par la requête. Deux entrées de cette requête ont changé avec secteurs :
   - la liste des rôles de `demo/annuaire-source.json` (rédaction des propositions) ;
   - le jeu, le catalogue et le vocabulaire du compilateur (`critere.derouler`, critère D-009).

   Sans appel Mistral, on ne peut pas réenregistrer. Les tests figent donc les entrées d'origine, copiées de `main` dans `agents/analyste/tests/donnees/` (`annuaire-source.json`, `demo-avant-secteurs.json`, `catalogue-demo-avant-secteurs.json`, `vocabulaire-demo-avant-secteurs.json`), via `annuaire_fige.py`. `critere.derouler` prend trois paramètres facultatifs (`jeu_existant`, `catalogue`, `vocabulaire`) ; comportement par défaut inchangé.

Rien d'autre n'a été modifié dans le code des branches.

## 3. Vérifications après l'étape 4

1. **Engine intact : oui.** `git diff main..feat/fusion-3 -- internal/directive internal/journal internal/rejeu internal/session internal/gate` est vide.
2. **Consignes neutres : oui pour tout ce qui vient des secteurs.** Les consignes de `operations`, `lcb-ft`, `rh-gestion`, `rh-disciplinaire` et la consigne de l'orchestrateur ne contiennent aucune règle de directive. Le seul changement à `rh.json` est la liste d'outils. **À signaler, hérité de `main` et non modifié** (les changer invaliderait les enregistrements de l'orchestrateur) :
   - `credit.json` : « Vérifie toujours le registre des gels avant toute attribution » et la règle du seuil de l'usure (officiel contre base interne) ;
   - `credit-historique.json` : la règle du seuil de l'usure de la base interne ;
   - `conformite.json` : le contrôle du registre des gels (c'est sa fonction).

   Ce sont les agents des scénarios de la scène (D-001, D-009), antérieurs aux secteurs. À trancher par Manu.
3. **Données fictives : conforme, avec une remarque.**
   - E-mails : tout est en `exemple.test` (y compris `contact@<entreprise>.exemple.test`) ou en domaines réservés (`example.com`). Seule adresse hors `exemple.test` ajoutée par la fusion : `ops@cabinet.fr`, **déjà dans `main`** (`internal/registredemo/registredemo.go`, tests du gate), recopiée dans neuf journaux de test de l'Analyste. `cabinet.fr` n'est pas un domaine réservé : à remplacer par `.exemple.test` par la session de gel si on veut la règle stricte (cela change des empreintes de journaux de test).
   - IBAN : 129 chaînes à clé invalide ; une seule à clé valide, l'IBAN d'exemple du test du contrôle de clé (`population_test.go`), qui n'est pas une donnée.
   - Personnes politiquement exposées : six profils, fonctions inventées (« maire de Fictiville-sur-Loire (commune inventée) », etc.), noms générés ; aucune personne réelle. Seule donnée réelle : `JSC ROSOBORONEXPORT`, personne **morale** inscrite au registre public des gels (déjà utilisée par `DOS-FICTIF-005`).
   - Graine fixe : `go test ./servers/banque-synthetique/...` (dont `TestDeterministeEtVersionne`, mêmes octets) vert.
4. **`demo/directives/demo.json` : conforme.** Version 4 ; D-001 à D-008 actives et identiques à `main` (comparaison champ à champ) ; 21 nouvelles directives D-011 à D-031, toutes `brouillon` ; D-009 et D-010 absentes (libres) ; `directive verifier` accepte le jeu ; `origines.json` a exactement les 21 identifiants en brouillon.
5. **Contrat §6** : ajout des champs `constat.occurrences`, `sessions_similaires`, `fondements_suggeres` (facultatifs) et du dossier `demo/execution/analyste/mesures/` (`24d3913`).
6. **Console** (binaire local, `curl`) :
   - `/api/etat` sur un dossier de sessions vide : `journaux: []`, aucun `null` ; `/api/etat` et `/api/journal/<id>/{demande,poste,chaine}` sur une session sans appel d'outil : 200, aucun `null` (tests Go correspondants : `TestListesVidesJamaisNull`, `TestDemandeSansAppelDOutil`, verts).
   - `/api/suggestions` : 46 demandes, toutes avec `titre`.
   - `/api/redaction` : les quatre propositions de l'Analyste apparaissent (copiées en local, hors dépôt, puis retirées).
   - L'onglet « Demandé / décidé » est dans `ecran-journal.js` et servi par le binaire ; **je ne l'ai pas affiché dans un navigateur** : vérifié par le code et les tests de la console, pas visuellement.
7. **Analyste sur les journaux de secteurs** : les journaux d'origine du worktree secteurs n'ont plus la session piège D-023. J'ai rejoué hors ligne `secteur-d023-piege`, `-ferme` et `-propre` (enregistrements de secteurs) et copié ces trois sessions dans `agents/analyste/tests/donnees/sessions-secteurs/`. Résultat : un constat `A-contournement-retrait-rh-synthetique.consulter_fiche`, sur le piège seulement (test `TestJournauxDeSecteurs`). Les quatre propositions de démo portent déjà `occurrences`, `sessions_similaires` et `fondements_suggeres` ; **leur régénération n'est pas faite** (voir écarts).
8. **Rejeu hors ligne de tous les scénarios** (`demo/scenarios/*.json`, 46, horloge `2026-10-04T10:00:00+02:00`) : 41 ont un enregistrement. **Les 41 : `verifier` sans écart** (chaîne intègre, rejeu de l'engine : aucun écart). Rejeu de l'orchestrateur :
   - 24 conformes à l'attendu ;
   - 16 « bloque » (`consigne-*`, pièges D-0xx, A5 avec A et AB) : attendu, le code de sortie 2 du lanceur signifie « bloqué » ;
   - **1 en erreur : `base-interne-avec-d009`**, « rejeu : réponse 15 demandée par `credit`, l'enregistrement s'arrête à 14 ». Écart déjà connu de `fusion-2` et de `finition.md`, non causé par cette fusion ; à repasser en réel.
   - **5 sans enregistrement** : `imposteur`, `rapport-analyste`, `rapport-prestataire`, `rh-prime`, `rh-sanction` (le lanceur sort en code 2 sans rien faire).
9. **Suite complète trois fois de suite : verte.** `go vet`, `go test -race -count=1 ./...` (intégration comprise), Python `agents/tests` (191 tests), `agents/orchestrateur/tests` (36), `agents/analyste/tests` (69) : trois passages, aucun échec, avant les deux derniers commits de documentation (`go vet` et les tests Go n'ont pas été modifiés depuis).
10. **`docs/demo/secteurs.md`** : état réel gardé. `demo.json` version 4 (D-001 à D-008 actives, D-011 à D-031 en brouillon), 80 salariés (30 écrits à la main + 50 générés) et 76 clients. Seule ligne contradictoire restante (« 30 salariés », en tête) corrigée ; la phrase « les directives des secteurs ne sont pas dans `demo.json` » n'existe plus dans le fichier fusionné.
11. **Rejeu du compilateur sur le `demo.json` final** (avec le catalogue et le vocabulaire courants, mode rejeu, rien refait en réel) : voir §4.

## 4. Écarts restants pour la session de gel de samedi

À réenregistrer (appels Mistral, interdits cette nuit) ou à décider :

1. **Parcours du compilateur qui ne se rejouent plus sur le `demo.json`, le catalogue et le vocabulaire finaux** (`EnregistrementAbsent`) :
   - `agents/compilateur/exemples/service/` : `base-interne`, `weekend` ;
   - `agents/compilateur/exemples/scenarios-v2/` : `credit`, `demandeur`, `discrimination`, `rh`, `top-management`, `weekend` ;
   - `demo/scenarios/compilation/` : `A4`, `B1` à `B6`, `R1` à `R6` (13 sur 16).

   Se rejouent : `A5-brut` (questions), `A5a` et `A5b` (jeu). Les tests Python passent parce qu'ils **figent** catalogue et vocabulaire de l'époque de l'enregistrement (`agents/tests/aides.py`, copies dans `agents/tests/donnees/`) ; le secours hors ligne de la scène de rédaction (week-end, base interne) sur le catalogue courant **ne rejoue pas** : à réenregistrer.
2. **Tests de l'Analyste figés sur l'annuaire et le jeu d'avant les secteurs** (`annuaire_fige.py`). Après réenregistrement de la rédaction (liste de rôles) et du compilateur, retirer `figer` / `derouler_fige`, régénérer les quatre propositions de démo (`python -m analyste proposer`) et leurs embeddings.
3. **`base-interne-avec-d009`** : rejeu de l'orchestrateur en erreur à la réponse 15 ; à repasser en réel.
4. **Cinq scénarios sans enregistrement** : `imposteur`, `rapport-analyste`, `rapport-prestataire`, `rh-prime`, `rh-sanction`.
5. **Consignes de `credit`, `credit-historique`, `conformite`** : règles de gel et d'usure dans la consigne (§3.2). Si Manu veut des consignes strictement neutres, il faut les réécrire et réenregistrer les scénarios de la scène.
6. **`ops@cabinet.fr`** (§3.3) : domaine non réservé, hérité de `main`.
7. **Onglet « Demandé / décidé »** non vérifié visuellement.
8. `go test` affiche une ligne `compilateur.demo: error: unrecognized arguments: --v2` : sortie d'un test qui vérifie le refus de l'ancien format, sans effet sur le résultat ; présente dès l'étape 1.

## 5. Incident : accès réseau hors périmètre

En préparant les rejeux, j'ai lancé `scripts/lancer-demo.sh` (préparation). Ce script exécute `bin/gels-avoirs preparer` quand le registre local des gels n'est pas du jour, ce qui **télécharge le registre national des gels des avoirs** : un accès réseau hors « registres de paquets seulement », que je n'avais pas anticipé. Résultat : `servers/gels-avoirs/enregistrements_locaux/` (ignoré par git, 6 609 entrées, donnée publique), aucun secret transmis, rien de commité. Les rejeux suivants n'ont fait aucun accès réseau (mode `rejeu`, registre du jour présent). Aucun appel Mistral pendant toute la session.

## 6. Vérifier en cinq minutes

```
git log --oneline main..feat/fusion-3 | head
git diff main..feat/fusion-3 -- internal/directive internal/journal internal/rejeu internal/session internal/gate   # vide
go vet ./... && go test -race -count=1 ./...
(cd agents && python3 -m unittest discover -s tests -t . && python3 -m unittest discover -s orchestrateur/tests -t . && python3 -m unittest discover -s analyste/tests -t .)
scripts/construire-serveurs.sh && sha256sum bin/manifeste.json     # deux fois : même empreinte
export MCP_GATE_CLE_JOURNAL=$(python3 -c "import secrets;print(secrets.token_hex(32))")
scripts/lancer-demo.sh scenario secteur-d023-piege --mode rejeu --horloge-depart 2026-10-04T10:00:00+02:00
scripts/lancer-demo.sh verifier demo/execution/sessions/<dernier>-secteur-d023-piege-rejeu
```
