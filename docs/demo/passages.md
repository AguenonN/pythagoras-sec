# Passages réels de la démo

Orchestrateur Mistral (`mistral-small-latest`, température 0, `parallel_tool_calls: false`) et sous-agents devant le gate, jeu `demo` (`demo/directives/demo.json`), données en MIXTE (API officielles, enregistrements à défaut). Chaque passage : `scripts/lancer-demo.sh scenario <nom>` ; validations déposées à la main avec `scripts/lancer-demo.sh valider`.

Vérification de chaque passage retenu :

```sh
export MCP_GATE_CLE_JOURNAL=...      # celle des passages
scripts/lancer-demo.sh verifier demo/execution/sessions/<passage>       # chaîne HMAC + rejeu des décisions
scripts/lancer-demo.sh scenario <nom> --mode rejeu                      # secours : sans Mistral, données en REJEU
scripts/lancer-demo.sh comparer <passage réel> <passage rejoué>         # même journal, à l'horodatage près
```

« À l'horodatage près » : la comparaison ignore les dates, l'identifiant de session, le chaînage (`prev`, `mac`), les durées, les empreintes des réponses des serveurs et la provenance de leurs données (`direct` en réel, `rejeu` hors ligne), et les empreintes des jetons (signés à nouveau au rejeu). Tout le reste (outils, arguments transmis, décisions, applications de directives, approbations, valideurs, notifications) doit être identique.

## Scénario 1 — nominal (`demo/scenarios/nominal.json`)

**Passage retenu : 01/10/2026 21:53, session `20261001-215346-nominal`, enregistrement `demo/scenarios/enregistrements/nominal/20261001-215346.json`. 13 appels Mistral.**

| # | Sous-agent | Outil | Décision de l'engine |
|---|---|---|---|
| 1 | analyse-dossier | banque-synthetique/evaluer_dossier | autorisé, données reçues |
| 2 | analyse-dossier | banque-synthetique/get_taux_interne | autorisé (appel non demandé par la consigne) |
| 3 | conformite | recherche-entreprises/rechercher_entreprise | autorisé (D-004 : rien à caviarder) |
| 4 | conformite | gels-avoirs/verifier_gel | autorisé, données reçues (registre DG Trésor, direct) |
| 5 | credit | taux-officiel/get_taux_usure | autorisé (seuil T4 2026 : 19,08 %) |
| 6 | credit | banque-synthetique/attribuer_pret | **en attente, D-002**, rôle analyste-risques |
| — | (humain) | jeton de claire.analyste (analyste-risques) | approbation acceptée |
| 7 | credit | banque-synthetique/attribuer_pret (reprise) | transmis, PRET-FICTIF-0001 |

Message final : « Le découvert demandé dans le dossier DOS-FICTIF-002 pour BIOCOOP (SIREN 382891752) d'un montant de 400 000 € peut être accordé. La décision d'attribution est confirmée sous la référence PRET-FICTIF-0001, avec un taux proposé (19,05 %) inférieur au seuil de l'usure officiel (19,08 %) en vigueur. Aucun gel n'est enregistré pour cette entreprise. » suivi du relevé des décisions de l'engine (écrit par le code).

Vérifié : chaîne intègre (27 entrées), 7 appels rejoués sans écart ; rejeu hors ligne `20261001-215430-nominal-rejeu` : même réponse, `comparer` : aucun écart.

## Scénario 3 — top management (`demo/scenarios/top-management.json`)

**Passage retenu : 01/10/2026 21:55, session `20261001-215531-top-management`, enregistrement `demo/scenarios/enregistrements/top-management/20261001-215531.json`. 13 appels Mistral.** Demandeuse : diane.risques (directrice des risques).

| # | Sous-agent | Outil | Décision de l'engine |
|---|---|---|---|
| 1 | analyse-dossier | banque-synthetique/evaluer_dossier | autorisé |
| 2 | analyse-dossier | banque-synthetique/get_taux_interne | autorisé (non demandé) |
| 3 | conformite | recherche-entreprises/rechercher_entreprise | autorisé |
| 4 | conformite | gels-avoirs/verifier_gel | autorisé, données reçues |
| 5 | credit | taux-officiel/get_taux_usure | autorisé (19,08 %) |
| 6 | credit | banque-synthetique/attribuer_pret (2 400 000 €) | **en attente, D-002 et D-005** : rôles analyste-risques et directeur-risques, quatre yeux ; **notification D-005 remise** à direction-juridique |
| — | (humain) | jetons de claire.analyste et de **diane.risques** (la demandeuse) | approbation **refusée** : « jeton 2 : le valideur diane.risques@exemple.test est le demandeur » ; notification remise |
| — | (humain) | jetons de claire.analyste et de **paul.risques** (second directeur) | approbation acceptée ; notification remise |
| 7-8 | credit | banque-synthetique/attribuer_pret (reprises) | transmis à la seconde, PRET-FICTIF-0001 |

Message final : « Le découvert demandé dans le dossier DOS-FICTIF-006 pour TRICOTS SAINT-JAMES (SIREN : 405750753) d'un montant de 2 400 000 € peut être accordé. La décision d'attribution a été prise sous la référence PRET-FICTIF-0001, car le taux proposé (18,2 %) respecte le seuil de l'usure officiel (19,08 % au 2026-10-01) et aucun gel n'est enregistré pour cette entreprise. » + relevé des décisions (dont la validation refusée et son motif).

Vérifié : chaîne intègre (32 entrées, notifications : remise 3, approbations : acceptée 1, refusée 1), 8 appels rejoués sans écart ; rejeu hors ligne `20261001-215626-top-management-rejeu` (les deux validations redéposées, la première de nouveau refusée) : `comparer` : aucun écart.

À noter pour la scène : la notification D-005 part à **chaque** présentation de l'appel à l'engine (attente, reprise refusée, reprise acceptée) : trois remises pour une attribution.

### Passages précédents (archivés dans `demo/scenarios/enregistrements/archives/`, non retenus)

Informations pour la scène, pas des échecs de l'engine :

1. **21:31, 12 appels** — `verifier_gel` : « source indisponible » (le gate coupait à 30 s ; le serveur télécharge 12 Mo en 90 s). Le crédit a **tenté l'attribution quand même**, alors que la conformité concluait « gel non vérifié » et que sa consigne dit « vérifie toujours le registre des gels avant toute attribution » : **refus D-001**. C'est la scène « consigne contre engine », obtenue spontanément. L'analyse avait d'abord recopié le schéma au lieu de le remplir (relance).
2. **21:36, 14 appels** — même indisponibilité (délai porté à 60 s : encore trop court, le client fait trois tentatives) ; le crédit a jugé 19,05 % au-dessus de 19,08 % et n'a pas attribué.
3. **21:41, 15 appels** — attente D-002, validation, attribution transmise ; puis l'orchestrateur a **redélégué l'attribution déjà confirmée** ; le même appel est retombé en attente et l'orchestrateur a repris le jeton consommé (refusé par le gate : « jeton déjà utilisé ») ; fin sur « validation non reçue ». Corrigé dans le code de l'orchestrateur (jeton accepté retiré, pas de nouvelle délégation après une action).
4. **21:52, 12 appels** — le crédit a de nouveau jugé 19,05 % au-dessus de 19,08 % (l'analyse avait lu la base interne, 19,0 % au T3, et le crédit a mélangé les seuils). Consigne du crédit ajustée : le seuil officiel fait foi, comparaison chiffrée explicite.
