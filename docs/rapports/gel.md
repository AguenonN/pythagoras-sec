# Rapport — session « gel » (samedi 03/10, après la fusion n° 4)

Branche `feat/gel`. Engine non modifié (`internal/directive`, `journal`, `rejeu`, `session`, `gate`). Console non modifiée.

## Compteur Mistral

Budget : **400 appels** (autorisé par Manu, prime sur les 40 des règles communes). Compté par les fichiers `agents/.etat/**/appels.jsonl` (une ligne par appel réel).

| Étape | Appels | Cumul |
|---|---|---|
| Phase 1, point 0 : compilations A4, A5-brut, A5a, A5b | 9 | 9 |

## Phase 1 — derniers changements de code

### Point 0 — tests rouges

- `test_conforme_au_registre` : le registre de démo compte 12 serveurs depuis l'opération (8 + `data-room`, `messagerie`, `bourse`, `communication` de `demo/serveurs-supplementaires.json`), le catalogue versionné n'en avait que 8. Catalogue régénéré par la commande existante (`go run ./cmd/registre-demo -enregistrements demo/scenarios/enregistrements/serveurs -catalogue agents/compilateur/exemples/catalogue-demo.json`), deux fois : mêmes octets (catalogue `7a3b0809…`, registre `25581e19…`). Vocabulaire régénéré : inchangé.
  - **Décision** : le catalogue versionné inclut désormais les serveurs ATLAS (le compilateur en direct doit les connaître pour la scène principale). `internal/registredemo/catalogue_test.go` (test seulement) et les listes de serveurs des tests Python (`test_catalogue_demo.py`, `test_noms_serveurs.py`) comptent les serveurs supplémentaires. Le commentaire de `scripts/nouveau-serveur.sh` (« ceux de agents/compilateur/exemples restent ceux de la démo figée ») ne vaut plus pour les serveurs versionnés dans `demo/serveurs-supplementaires.json`.
- `test_secteurs` (A4, A5) : compilations **réenregistrées en réel** sur le catalogue et le vocabulaire actuels (`mistral-medium-latest`, modèle du compilateur). Mistral pose d'autres questions qu'à l'enregistrement d'origine : les réponses des parcours (`demo/scenarios/compilation/A4.json`, `A5a.json`, `A5b.json`) ont été réécrites avec le **même contenu**, sur le texte exact des nouvelles questions. Résultats identiques aux directives installées (D-024 à D-029, D-030, D-031), à l'ordre près des `champs_autorises` du caviardage de A5b : le test compare désormais cette liste sans tenir compte de l'ordre (sans portée pour l'engine).
  - **Constat pour la scène** : avec les serveurs ATLAS au catalogue, sur le texte brut A5 (« mails … rémunération »), Mistral demande si « mails » vise `messagerie/envoyer_message` (ATLAS) ou un autre outil. Question observable, utile à montrer.
