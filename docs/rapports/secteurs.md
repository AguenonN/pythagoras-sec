# Rapport — session G « secteurs » (banque et RH complètes)

Brief : `nuit/brief_G_secteurs.md`. Branche `feat/secteurs`. Règles : `docs/orchestration/00_regles_communes.md`.

**Compteur d'appels Mistral réels : 0 / 120** (compilateur : 0 ; orchestrateur : 0).

## 1. Fait

| Commit | Contenu |
|---|---|
| (étape 1) | Catalogue d'outils banque (9 outils) et RH (9 outils), données fictives (6 clients, 30 salariés), référentiel RH complété, étiquettes, identités fictives des postes, catalogue et vocabulaire du compilateur régénérés |

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
7. Réseau : aucun appel aux API officielles ; les passages réels se font avec `DEMO_DONNEES_MODE=REJEU` (Mistral réel, données rejouées) et le registre local des gels copié du dépôt principal.

## 4. Blocages

(aucun pour l'instant)

## 5. Questions pour Manu (par urgence)

1. **Scène du compilateur, samedi** : le catalogue que voit Mistral a changé (18 outils de plus). Les enregistrements de la scène (week-end, base interne) ne valent plus pour le service avec le catalogue courant : en mode rejeu, la console ne retrouvera pas ces réponses ; en réel, Mistral voit un catalogue plus grand et peut répondre autrement. Voir §2 pour ce qui a été fait à ce sujet.

## 6. Vérifier en 5 minutes

```sh
go vet ./... && go test -race -count=1 ./...
(cd agents && .venv/bin/python -m unittest discover -s tests -t . && cd orchestrateur && ../.venv/bin/python -m unittest discover -s tests -t ..)
```
