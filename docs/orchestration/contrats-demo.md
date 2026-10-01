# Contrats de la démo — figés avant les sessions A, B, C, D

À commiter sur `main` (dans `docs/orchestration/contrats-demo.md`) **après la fusion** et **avant** de créer les worktrees. Toute session qui a besoin de s'écarter d'un contrat le **signale** dans son rapport, ne le change pas seule.

## 0. Vocabulaire à l'écran

On ne montre jamais « v1 / v2 » (schéma) ni « jeu credit-demo v3 ». À l'écran :
- une directive : `D-00n · <titre>`, son auteur, sa date d'activation, son statut (brouillon, observation, active, retirée) ;
- l'ensemble des règles : « règles en vigueur depuis le <date heure> » ;
- la comparaison : **« sans la directive D-00n » / « avec la directive D-00n »**.
La version du schéma et le numéro de version du jeu restent dans les fichiers et le journal.

## 1. Jeu unique de la démo

- Fichier : `demo/directives/demo.json`, identifiant de jeu **`demo`**, format v2 (schéma `docs/schema-directive-v2.md`).
- Le juriste n'ajoute jamais un autre jeu : chaque ajout, activation ou retrait produit **la version n+1 du même jeu** (le gate refuse un autre identifiant et une version réécrite).
- Version 1, préchargée, statut `active`, fondements marqués « à valider par le juriste » quand ils le sont :

| Id | Titre | Déclencheur | Condition | Effets |
|---|---|---|---|---|
| D-001 | Gels vérifiés avant tout prêt | `banque-synthetique/attribuer_pret` | `non(session.a_consulte gels-avoirs/verifier_gel, donnees_recues)` | refuser |
| D-002 | Taux officiel consulté | `banque-synthetique/attribuer_pret` | `session.a_consulte taux-officiel` | attendre_validation par_role `analyste-risques` |
| D-003 | Source officielle attestée | `taux-officiel/*` | `attestation.verifiee == false` | refuser |
| D-004 | SIREN seul vers l'extérieur | `recherche-entreprises/*` | — | caviarder `/siren`, mode caviarder |
| D-005 | Décision top management | `banque-synthetique/attribuer_pret` | `/montant > 2000000` | attendre_validation par_role `directeur-risques`, quatre_yeux ; notifier `direction-juridique` temps_reel, si_echec refuser |
| D-006 | RH : contentieux hors scope | étiquette `donnees_personnelles`, accès `action` | `existe` sujet groupe `contentieux-en-cours` | refuser |
| D-007 | RH : salariés protégés | étiquette `donnees_personnelles`, accès `action` | `existe` sujet groupe `salaries-proteges` | attendre_validation par_role `responsable-rh` |
| D-008 | Rapport sur données client | étiquette `production_rapport` | `et(demandeur groupe prestataires-externes, session.a_consulte étiquette donnees_client)` | refuser ; notifier `direction-juridique` temps_reel, si_echec refuser |

- Ajoutées **en direct** le jour J (jamais préchargées) :
  - **D-009 « Base interne des taux »** : `session.a_consulte banque-synthetique/get_taux_interne` → attendre_validation par_role `analyste-risques` sur `attribuer_pret`. Origine Analyste (§6) : proposée par l'Analyste à partir des exécutions passées ; le juriste la rejette ou la reprend (modifiée ou telle quelle) et décide seul de l'activer.
  - **D-010 « Pas d'action d'agent le week-end »** : origine juriste (§6) : dictée par le jury, écrite par le juriste, compilée en direct, activée par lui, puis **retirée** (le jour J est un dimanche).
- Rôles, groupes et identités : ceux de `demo/annuaire-source.json` ; s'il en manque (`analyste-risques`, `responsable-rh`, `direction-juridique`, `prestataires-externes`), **la session A les ajoute** (identités fictives).

## 2. Exécution d'une demande

- **Un processus `mcp-gate` par demande** (lancé par l'orchestrateur en stdio), qui relit `demo/directives/demo.json` au démarrage : une directive activée vaut pour la demande suivante. `SIGHUP` reste disponible mais la démo n'en dépend pas.
- Dossier d'une demande : `demo/execution/sessions/<AAAAMMJJ-HHMMSS>-<court>/` contenant :
  - `journal.jsonl` (journal chaîné du gate), `coffre.jsonl` ;
  - `etat.json` : `{"statut": "en_cours"|"en_attente"|"termine"|"bloque"|"erreur", "demande", "demandeur", "debut", "fin", "attente": {"empreinte_transmise", "roles", "quatre_yeux", "directives"} | null}`, réécrit atomiquement (fichier temporaire + renommage) à chaque changement ;
  - `reponse.json` : `{"texte", "statut", "directives_citees": [...]}` à la fin ;
  - `notifications.jsonl` (abonné `-notifications` du gate).
- Clé du journal : `MCP_GATE_CLE_JOURNAL`, fournie par l'environnement, jamais écrite sur disque. La console la reçoit de la même façon.

## 3. Validation humaine

- Quand `etat.json` passe `en_attente`, l'orchestrateur **reste vivant** (même session MCP) et attend des jetons dans `demo/execution/approbations/<empreinte_transmise>/*.jwt` (un fichier par valideur), en relisant le dossier toutes les secondes, **10 minutes** au plus (puis `statut: bloque`, motif « validation non reçue »).
- Dès que les jetons requis sont là (un par rôle), il reprend l'appel avec `_meta.approvalToken` = liste des jetons. Une reprise refusée par le gate (`motif_jeton`) remet `etat.json` en `en_attente` avec ce motif.
- Le jeton est produit par `mcp-approve valider -valideur <id> -role <role> -empreinte <empreinte>` (clé du valideur dans `demo/execution/cles/`, hors Git). La console appelle cette commande ; elle ne signe rien elle-même.

## 4. Compilateur appelé par la console

`python -m compilateur.service` lit **une** requête JSON sur l'entrée standard, écrit **une** réponse JSON sur la sortie standard, code 0 si la réponse est exploitable :

Requête :
```json
{"conversation": "<id ou null>", "texte": "...", "fondement": "...", "auteur": "juriste@exemple.test",
 "reponses": [{"question": "<id>", "reponse": "..."}],
 "jeu_existant": "demo/directives/demo.json", "date_effet": "2026-10-04T11:05:00+02:00"}
```
Réponse :
```json
{"conversation": "<id>", "etat": "questions"|"non_observable"|"jeu"|"refuse"|"erreurs",
 "questions": [{"id", "texte", "options": [...], "origine": "mistral"|"code"}],
 "non_observable": {"extrait", "raison"} | null,
 "relecture": "<texte français déterministe des seules directives ajoutées>",
 "jeu_candidat": "demo/execution/compilation/<id>/demo.vN.json" | null,
 "erreurs": [...]}
```
- L'état de la conversation vit dans `demo/execution/compilation/<id>/` ; rappeler le service avec les réponses **ne refait pas** les appels Mistral déjà faits.
- `jeu_candidat` = toutes les directives existantes inchangées + les nouvelles en `brouillon`, version n+1, accepté par `directive verifier`.
- **Activation** (console) : copie du jeu candidat, statut de la nouvelle directive passé à `observation` ou `active`, version n+1 (ou n+2 si un candidat a déjà pris n+1), `directive verifier` sans `--exiger-brouillon`, écriture atomique de `demo/directives/demo.json`, ancienne version archivée dans `demo/directives/historique/demo.v<n>.json`. Retrait = même chemin, statut `retiree`.

## 5. Possession des fichiers (pour éviter les conflits)

| Session | Possède |
|---|---|
| A — démo réelle | `agents/orchestrateur/` (nouveau), `agents/boucle/`, `servers/banque-synthetique/`, `demo/directives/demo.json`, `demo/annuaire-source.json`, `demo/scenarios/`, `scripts/lancer-demo.sh`, `scripts/nouveau-serveur.sh`, `docs/demo/`, `docs/equipe/` (sauf `modifier-la-console.md`) |
| B — compilateur | `agents/compilateur/`, `internal/relecture/`, `cmd/directive/`, `cmd/registre-demo/` (export du vocabulaire seulement) |
| C — console | `cmd/console/`, `internal/console/`, `docs/equipe/modifier-la-console.md` |
| D — gate | `internal/gate/`, `internal/backend/`, `internal/registre/`, `internal/identity/`, `cmd/mcp-gate/`, `deploy/lab/` |
| E — analyste (vendredi) | `agents/analyste/` (nouveau), `demo/execution/propositions/<id>.json` (jamais les `.decision.json`) |

| v2-seul (avant A–D sur ces fichiers) | `internal/directive/`, `internal/rejeu/`, tests et données de `internal/journal/`, `internal/gate/` (tests seulement), `test/gate/`, `test/integration/`, jeux de test de `demo/directives/` hors `demo.json` |

**Une seule forme de directive** : le format v1 est retiré du dépôt (décision de Manu). Aucune session n'écrit, ne charge ni n'affiche de directive v1.

`internal/directive`, `internal/journal`, `internal/rejeu`, `internal/session` : en dehors de la session v2-seul, **personne** sans le signaler comme correctif de bogue dans son rapport.

## 6. Deux origines de directives, un seul décideur

- **Origine juriste** : texte écrit par le juriste dans l'écran de rédaction.
- **Origine Analyste** : `demo/execution/propositions/<id>.json` = `{"id", "date", "constat": {"type", "detail"}, "sessions": [...], "texte_propose", "fondement_propose", "explication"}`, écrit par l'Analyste seul.
- **Décision du juriste sur une proposition** : `demo/execution/propositions/<id>.decision.json` = `{"decision": "rejetee"|"reprise_modifiee"|"reprise_telle_quelle", "motif", "texte_retenu", "auteur", "date", "conversation_compilateur"}`, écrit par la console seule, à l'action du juriste. Un rejet exige un motif.
- Dans les deux cas, le texte retenu passe par le compilateur (§4), puis le juriste choisit : laisser en brouillon, mettre en observation, activer, ou abandonner. Rien n'est jamais activé sans son action.
- Chaque directive garde son origine dans le jeu : la relecture et la vitrine affichent « écrite par J. Juriste » ou « proposée par l'Analyste le …, reprise [telle quelle | modifiée] par J. Juriste ». Le schéma n'ayant pas de champ dédié, l'origine est tenue dans `demo/directives/origines.json` (`{"D-009": {"proposition": "<id>", "decision": "reprise_modifiee"}}`), écrit par la console à l'activation.
