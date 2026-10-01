# Rapport — session A « démo réelle »

Brief : `nuit/brief_A_demo_reelle.md`. Contrats : `docs/orchestration/contrats-demo.md` (font foi).
Branche : `feat/demo-reelle`. Rien poussé, rien fusionné.

## 1. Fait (commits)

| Commit | Contenu |
|---|---|
| `4d7a9b8` | `demo/directives/demo.json` : jeu `demo` v1, D-001 à D-008 (contrats §1), format v2 ; annuaire complété (identités fictives) |
| `16b067f` | `banque-synthetique` : DOS-FICTIF-006 (TRICOTS SAINT-JAMES, déjà utilisée ; 2 400 000 € inventés ; 18,2 % < 19,08 %, seuil T4 2026) |
| `4f88448` | `agents/boucle` : consigne et outils propres à un sous-agent, arrêt au premier refus, relance (défaut inchangé) |
| `8d243f8` | `agents/orchestrateur` : orchestrateur Mistral, sous-agents déclaratifs, dossier de session, attente des jetons, enregistrement et rejeu |
| (suivant) | `scripts/lancer-demo.sh` (préparer, demande, scenario, valider, verifier, comparer), scénarios déclaratifs `demo/scenarios/*.json` |

## 2. Passages réels

Voir `docs/demo/passages.md` (rempli au fil des passages). Compteur Mistral : `agents/.etat/orchestrateur/budget.json`.

| Passage | Statut | Appels Mistral |
|---|---|---|
| (à venir) | | |

## 3. Pas fait, et pourquoi

(à compléter)

## 4. Décisions prises seul

- **Budget Mistral** : le brief donne 80 appels, les règles communes 40 (« priment sur toute autre considération »). Le brief, propre à cette session et plus récent, l'emporte pour le plafond technique (`ORCHESTRATEUR_MAX_APPELS=80`) ; les scénarios du critère de réussite (1 à 4) passent d'abord, chacun une fois, avant tout passage de fiabilité. À trancher par Manu (question 1).
- **Rapport** : `RAPPORT_A.md` (nom du brief) plutôt que `RAPPORT_NUIT.md` (règles communes).
- **D-002, critère** : le contrat ne précise pas le critère de `session.a_consulte taux-officiel` ; choisi `transmis` (tout appel parti, même sans données), le plus large, donc le plus prudent pour une directive qui retient.
- **D-008, critère** : `transmis` aussi (même raison).
- **D-006 / D-007** : le contrat dit « `existe` sujet groupe … ». Un seul `existe` sur `/salarie_ids` serait indéterminé (donc refus) pour `proposer_sanction`, qui n'a que `/salarie_id`, et inversement. Chaque branche est donc gardée par la présence de son champ : `ou(et(present /salarie_ids, existe …), et(present /salarie_id, sujet …))`. Même intention, aucun refus par indétermination sur l'outil qui n'a pas l'autre champ. **Écart de forme signalé**, à valider.
- **Annuaire** : version laissée à 1 (un test de `internal/annuaire`, hors de mon périmètre, la fige). Ajouts : rôles `analyste-risques` (Claire), `responsable-rh` (Hélène), `direction-juridique` (Julie, rôle et groupe) ; identités `paul.risques` (second directeur des risques), `camille.conseiller` (conseiller), `luc.manager` (demandeur RH : le gate refuse tout valideur égal au demandeur, donc la responsable RH ne peut pas être la demandeuse du scénario 5).
- **Clé du journal** : jamais sur disque. Pour mes passages, elle est dérivée à chaque commande : HMAC-SHA256 du libellé « mcp-rogue demo-reelle cle-journal » sous `MISTRAL_API_KEY` (rien d'écrit, rien d'affiché ; Manu peut la recalculer).
- **Fin sur refus** : au premier refus de l'engine, la demande s'arrête (aucun appel au modèle ensuite) et le texte rendu est la décision de l'engine. Plus sûr et moins coûteux que de rendre la main au modèle.
- **Jetons refusés** : un jeu de jetons refusé à la reprise est renommé `*.jwt.refuse` ; l'orchestrateur attend un nouveau dépôt complet. À la première attente d'un appel, les jetons déjà présents pour la même empreinte (même appel dans une demande précédente) sont renommés `*.jwt.ancien`.
- **Fichiers ajoutés au dossier de session** (au-delà du §2) : `jeu.json` (le jeu passé au gate, pour `journal rejouer`), `orchestrateur.json` (trace : délégations, appels, validations, nombre d'appels Mistral), `mistral/` (requêtes et réponses brutes, hors Git), `mcp-gate.log`.
- **Enregistrements des serveurs de données** : dans `demo/scenarios/enregistrements/serveurs/` (versionnés, prolongés par le MIXTE des passages réels), pour que le secours hors ligne les ait ; eurlex et legifrance sont des liens vers leurs enregistrements (aucun sous-agent n'a leurs outils).
- **Rejeu** : les réponses Mistral sont rejouées **dans l'ordre**, pas par empreinte de requête : le gate scelle chaque résultat sous un identifiant aléatoire, la requête n'est donc jamais deux fois la même.

## 5. Blocages

(aucun pour l'instant)

## 6. Questions pour Manu (par urgence)

1. Budget Mistral : 80 (brief) ou 40 (règles communes) ?
2. D-006/D-007 : la garde par présence du champ convient-elle ?

## 7. Vérifier en 5 minutes

```sh
cd agents && python3 -m unittest discover -s orchestrateur/tests -t . && python3 -m unittest discover -s tests -t . && cd ..
python3 -m orchestrateur sous-agents   # depuis agents/
scripts/lancer-demo.sh                  # prépare (construit, registres, annuaire, gels)
```
