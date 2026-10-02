# Rapport — session G « secteurs » (banque et RH complètes)

Brief : `nuit/brief_G_secteurs.md`. Branche `feat/secteurs`. Règles : `docs/orchestration/00_regles_communes.md`.

**Compteur d'appels Mistral réels : 29 / 120** (compilateur, `mistral-medium-latest` : 29 ; orchestrateur : 0).

## 1. Fait

| Commit | Contenu |
|---|---|
| `167ca1a` | Catalogue d'outils banque (9 outils) et RH (9 outils), données fictives (6 clients, 30 salariés), référentiel RH complété, étiquettes, identités fictives des postes, catalogue et vocabulaire du compilateur régénérés |

| `7794fb3` | Sous-agents `operations`, `lcb-ft`, `rh-gestion`, `rh-disciplinaire` (consignes neutres, testées sans vocabulaire juridique) ; consigne de l'orchestrateur : deux secteurs, routage, réponse polie hors périmètre |

| (étape 3) | 12 textes compilés (13 directives D-011 à D-023), jeu de travail actif et jeu en brouillon, `origines.json`, parcours de compilation rejouables, test |

### Étape 3 — directives

Textes écrits en français, fondement « à valider par le juriste », passés par `python -m compilateur.service --mode reel` (jeu existant : `demo/directives/demo.json`), questions répondues, `si_indetermine` : refuser (défaut, fermé). Parcours rejouables hors ligne : `demo/scenarios/compilation/<clé>.json` (test `agents/orchestrateur/tests/test_secteurs.py`).

| Id | Texte (clé) | Déclencheur | Condition | Effet |
|---|---|---|---|---|
| D-011 | B1 virement sans gel vérifié | banque-synthetique/effectuer_virement | non(session a consulté gels-avoirs, données reçues) | refuser |
| D-012 | B2 courrier après soupçon | banque-synthetique/envoyer_courrier_client | session a consulté banque-synthetique/declarer_soupcon (transmis) **rétablie** | refuser |
| D-013 | B3 plafond > 10 000 € | banque-synthetique/modifier_plafond_decouvert | /nouveau_plafond > 10000 | attendre_validation analyste-risques |
| D-014 | B4 export externe | banque-synthetique/exporter_donnees_client | /destination == externe **rétablie** | refuser |
| D-015 | B4 export interne | banque-synthetique/exporter_donnees_client | /destination == interne **rétablie** | caviarder (garde /client_id, /destinataire, /destination, /inclure_operations) |
| D-016 | B5 clôture | banque-synthetique/cloturer_compte | — | attendre_validation, quatre yeux |
| D-017 | B6 refus de crédit | banque-synthetique/refuser_credit | — | attendre_validation |
| D-018 | R1 critère protégé | rh-synthetique/rechercher_salaries | ou(présent /criteres/age_min, age_max, en_arret_maladie, enceinte, nationalite, syndique) | refuser |
| D-019 | R2 fiche : santé, syndical | rh-synthetique/consulter_fiche | — | caviarder (garde /salarie_id, /inclure_remuneration) |
| D-020 | R3 sanction sans convocation | rh-synthetique/sanctionner | non(session a consulté rh-synthetique/convoquer_entretien_prealable, données reçues) | refuser |
| D-021 | R4 licenciement | rh-synthetique/licencier | — | attendre_validation drh |
| D-022 | R5 paie hors UE | rh-synthetique/exporter_paie | non(/pays dans les 27 codes UE) | refuser |
| D-023 | R6 salaires d'une liste | rh-synthetique/lister_salaries | — | caviarder (garde /service) |

- **Numérotation (arrêt demandé par le brief)** : sur le jeu produit, le compilateur numérote « base interne » et « week-end » **D-024** (parcours du service rejoués, catalogue de la scène) : `numeros_suivants` prend le plus grand identifiant + 1, sans combler D-009/D-010. Je me suis donc arrêté sur ce point : **`demo/directives/demo.json` n'est pas modifié**, le compilateur non plus. Le jeu prêt à installer est `demo/scenarios/jeux/secteurs-brouillon.json` (D-001 à D-008 inchangées + D-011 à D-023 en brouillon, version 2) ; les scénarios utilisent `demo/scenarios/jeux/secteurs.json` (mêmes directives, actives).
- **Défaut du compilateur (session B), constaté, non corrigé** : pour B2 et B4, Mistral a proposé la bonne condition mais l'a imbriquée sous une clé `condition` (`{"condition": {"racine", "noeuds"}}`) au lieu de `racine`/`noeuds` au niveau de la directive ; `construire_directive` lit `p.get("noeuds")`, ne trouve rien, et **produit la directive sans condition, sans erreur ni question** : « aucun courrier au client » au lieu de « aucun courrier après une déclaration de soupçon », et « aucun export » au lieu de « aucun export externe ». La relecture le montre, mais un juriste pressé peut l'activer. Dans le jeu de travail, la condition proposée par Mistral est rétablie à la main (D-012, D-014, D-015), signalée dans `origines.json` (« note ») et dans le test.
- R3 : premier texte refusé (« la source rh-synthetique n'est pas désignée par le juriste ») ; reformulé une fois en nommant les outils, compilé.
- Fondements : le compilateur sépare « texte » et « article » et perd parfois la mention « à valider par le juriste » ; elle est rajoutée au premier fondement à l'assemblage.
- `demo/directives/origines.json` : une entrée par directive (`origine: preparee_pour_la_demo`, texte « à valider par le juriste », conversation du compilateur). La console lit ce fichier dans un type à deux champs (`proposition`, `decision`) : mes champs y sont ignorés, et la console les effacerait en réécrivant le fichier (sans gêne : aucune de ces directives n'est une proposition de l'Analyste).
- Aucun texte non exprimable au final.

### Étape 1 — catalogue

- `banque-synthetique` : `lister_comptes_client`, `historique_operations` (lecture) ; `effectuer_virement`, `modifier_plafond_decouvert`, `cloturer_compte`, `exporter_donnees_client`, `envoyer_courrier_client`, `declarer_soupcon`, `refuser_credit` (action). Données : `clients_fictifs.json` (6 clients, IBAN en `FR76 9999 9…`, adresses `exemple.test`). Outils existants inchangés (noms et schémas).
- `rh-synthetique` : `lister_salaries`, `rechercher_salaries`, `consulter_fiche` (lecture) ; `convoquer_entretien_prealable`, `sanctionner`, `licencier`, `attribuer_prime`, `exporter_paie`, `envoyer_message_salarie` (action). 30 salariés fictifs (libellés « Salarié fictif A… AD »), avec les rubriques santé, syndical, salaire, âge, nationalité, grossesse, étiquetées dans `etiquettes_champs`. `consulter_salarie` garde son schéma (il ne lit que ses champs).
- Conception pour que l'engine puisse agir : les rubriques sensibles ne sortent que **si l'appel les demande** (`inclure_iban`, `inclure_revenus`, `inclure_sante`, `inclure_syndical`, `inclure_salaires`…). Un caviardage (qui porte sur les arguments, schéma v2 §6) retire la demande, et la rubrique ne sort pas. La destination d'un export est l'argument `destination` (`interne` / `externe`), le pays un code ISO à deux lettres.
- `referentiel-rh` : les 25 nouveaux salariés y figurent (SAL-FICTIF-010 protégé, SAL-FICTIF-017 en contentieux) ; un test vérifie que tout salarié est au référentiel (sinon D-006/D-007 seraient indéterminées, donc refus).
- `demo/etiquettes.json` : `donnees_client`, `transfert_donnees`, `mouvement_fonds`, `donnees_sante`, `appartenance_syndicale`, `salaire` ; personnes concernées (`/salarie_id`) des nouveaux outils RH.
- `demo/annuaire-source.json` : sophie.gestionrh (gestionnaire RH), nadia.disciplinaire (responsable disciplinaire), ines.drh (seconde DRH, rôle `drh`), olivier.lcbft (chargé LCB-FT).
- Vérifié : les cinq scénarios enregistrés qui rejouaient avant rejouent encore (attendu conforme, `verifier` : aucun écart) ; `comparer` entre le rejeu d'avant et celui d'après : écarts **uniquement** sur les empreintes du registre, de l'annuaire et des binaires (attestation). `base-interne-avec-d009` était déjà en erreur au rejeu avant cette session (connu, `docs/demo/passages.md`).

## 2. Pas fait (et pourquoi)

(en cours)

## 3. Décisions prises seul

1. **Budget Mistral : 120** (brief) plutôt que 40 (règles communes) : le brief de cette session fixe explicitement ce budget ; les deux compteurs (`agents/.etat/budget.json` pour le compilateur, `agents/.etat/orchestrateur` pour l'orchestrateur) sont plafonnés pour que leur somme ne dépasse pas 120.
2. **Tests du compilateur (session B) touchés** : régénérer le catalogue (obligatoire : `TestCatalogueVersionne`) change chaque requête du compilateur, donc invalide les enregistrements de la scène (week-end, base interne) et des scénarios « medium ». J'ai figé l'ancien catalogue et l'ancien vocabulaire (`agents/tests/donnees/*-scene-enregistrements.json`, copies de `5b83cd5`) pour ces tests, sur le modèle des copies gelées existantes ; deux tests de vocabulaire (`test_outils_vises`, `test_argument_absent_d_un_outil_vise`) attendent maintenant les outils RH ajoutés. **Conséquence pour samedi : voir question 1.**
3. `agents/orchestrateur/tests/faux_gate.py` (fixture) expose les nouveaux outils, sans quoi les sous-agents de la démo ne se chargent pas devant le faux gate.
4. Sous-agent existant `rh` : `rh-synthetique__*` remplacé par la liste de ses trois outils d'origine, pour qu'il garde exactement son périmètre (il aurait sinon reçu les neuf nouveaux).
5. `test/integration/integration_test.go` : nombre d'outils exposés 14 → 32.
6. Clé du journal de mes passages : générée pour la session, gardée hors du dépôt (bloc-notes de la session), jamais affichée.
7. `refuser_credit` est confié au sous-agent `operations` (le brief ne le place dans aucun sous-agent ; le sous-agent `credit` existant n'est pas modifié). `operations` reçoit aussi `lister_comptes_client` (lecture) pour retrouver un compte ; `rh-gestion` et `rh-disciplinaire` ont exactement les outils du brief.
8. Réseau : aucun appel aux API officielles ; les passages réels se font avec `DEMO_DONNEES_MODE=REJEU` (Mistral réel, données rejouées) et le registre local des gels copié du dépôt principal.

## 4. Blocages

(aucun pour l'instant)

## 5. Questions pour Manu (par urgence)

0. **Numérotation D-009/D-010** : le compilateur numérote après le plus grand identifiant. Choisir : (a) laisser `demo.json` tel quel pour samedi et n'installer les secteurs qu'après la scène ; (b) installer `secteurs-brouillon.json` et accepter D-024/D-025 à la scène (les tests `test_service_scene` et `test_jeux` attendent D-009 et `demo.json` à huit directives) ; (c) faire combler les trous réservés par le compilateur (session B). Installer = copier `demo/scenarios/jeux/secteurs-brouillon.json` sur `demo/directives/demo.json`.
1. **Défaut du compilateur** (condition imbriquée écartée sans erreur, §1 étape 3) : à corriger par la session B avant que le juriste ne compile en direct ; en attendant, relire chaque relecture.
2. **Scène du compilateur, samedi** : le catalogue que voit Mistral a changé (18 outils de plus). Les enregistrements de la scène (week-end, base interne) ne valent plus pour le service avec le catalogue courant : en mode rejeu, la console ne retrouvera pas ces réponses ; en réel, Mistral voit un catalogue plus grand et peut répondre autrement. Voir §2 pour ce qui a été fait à ce sujet.

## 6. Vérifier en 5 minutes

```sh
go vet ./... && go test -race -count=1 ./...
(cd agents && .venv/bin/python -m unittest discover -s tests -t . && cd orchestrateur && ../.venv/bin/python -m unittest discover -s tests -t ..)
```
