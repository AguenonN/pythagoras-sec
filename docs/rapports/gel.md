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

### Points 1 et 2 — modèle des agents ; demandes en direct hors des fichiers versionnés

- Orchestrateur et sous-agents : `mistral-medium-latest` par défaut (`orchestrateur.modele.MODELE_AGENTS`) ; `--modele` dans `python -m orchestrateur executer` (ou `ORCHESTRATEUR_MODELE`), dans `scripts/lancer-demo.sh demande|scenario|enregistrer`. L'enregistrement note le modèle réellement utilisé. Le compilateur garde le sien (`MODELE_COMPILATEUR`, `mistral-medium-latest`). Tests : `TestCLI.test_option_modele` et défaut vérifié dans `test_reel_puis_rejeu`.
- Les serveurs de données du registre enregistrent leurs réponses dans une copie de travail, `demo/execution/serveurs/<serveur>/` (ignorée par Git), remplie à la préparation et avant chaque demande depuis `demo/scenarios/enregistrements/serveurs/` (sans rien écraser). `scenario <nom>` en réel écrit l'enregistrement de Mistral sous `demo/execution/enregistrements/<nom>/`. **Nouvelle sous-commande `scripts/lancer-demo.sh enregistrer <nom>`** : seule à écrire les secours versionnés (`demo/scenarios/enregistrements/<nom>/<date>.json`, et les réponses de serveurs obtenues pendant le passage, recopiées vers les enregistrements versionnés). Le rejeu (`--mode rejeu`) lit toujours le dernier enregistrement versionné.
- Test : `test/extension/demande_directe_test.go` — chaque chemin `-enregistrements` du registre est ignoré par Git, et un scénario lancé en réel (sans clé Mistral : arrêt au premier appel au modèle, après le lancement du gate et des serveurs en MIXTE) laisse `git status` inchangé.

### Point 3 — `base-interne-avec-d009` en erreur

Cause : l'enregistrement du 02/10 09:19 a été fait **avant** le garde-fou d'idempotence de l'orchestrateur. Dans ce passage, après l'attribution transmise par `credit-historique` (réponse 10), l'orchestrateur a délégué au `credit` une vérification du seuil officiel (réponse 12), et le `credit` a renvoyé la même attribution (réponse 14). Avec le code actuel, cet appel n'est plus transmis au gate (« Non envoyé par l'orchestrateur : cette action … a déjà été effectuée ») : le sous-agent reçoit ce résultat et demande une 15e réponse au modèle, que l'enregistrement n'a pas → `ErreurRejeu`. Ni l'orchestrateur ni l'engine ne sont en défaut (le garde-fou fait ce qu'il doit) : l'enregistrement est périmé. Correction : scénario réenregistré sous le code actuel (phase 4).

### Point 4 — clé du journal changée

- `scripts/lancer-demo.sh demande|scenario` vérifie, avant de lancer le gate, chaque `demo/execution/sessions/*/journal.jsonl` non vide avec `bin/journal verifier` et la clé courante ; au premier échec : « journaux signés avec une autre clé : scripts/lancer-demo.sh archiver-sessions » (avec le nombre et le premier journal en cause), code 1, gate non lancé.
- `scripts/lancer-demo.sh archiver-sessions` : déplace les **seules** sessions dont le journal ne se vérifie pas dans `demo/execution/sessions-archivees/<AAAAMMJJ-HHMMSS>/` ; rien n'est supprimé ; « rien à archiver » sinon.
- Test : `test/extension/cle_journal_test.go` (journal signé avec une autre clé → message clair, pas de gate ; archivage ; second archivage vide).
- **Console (non modifiée, point 5)** : elle affiche « chaîne rompue à l'entrée n° 1 » pour un journal signé avec une autre clé (`internal/console/web/app.js`, `alerteChaines`). Modification à faire par la session console : quand la première entrée ne se vérifie pas (`premier_ecart == 0`), afficher « journal signé avec une autre clé (ou clé différente de celle de la console) : `scripts/lancer-demo.sh archiver-sessions` ».
