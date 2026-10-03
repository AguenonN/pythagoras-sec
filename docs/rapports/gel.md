# Rapport — session « gel » (samedi 03/10, après la fusion n° 4)

Branche `feat/gel`. Engine non modifié (`internal/directive`, `journal`, `rejeu`, `session`, `gate`). Console non modifiée.

## Compteur Mistral

Budget : **400 appels** (autorisé par Manu, prime sur les 40 des règles communes). Compté par les fichiers `agents/.etat/**/appels.jsonl` (une ligne par appel réel).

| Étape | Appels | Cumul |
|---|---|---|
| Phase 1, point 0 : compilations A4, A5-brut, A5a, A5b | 9 | 9 |
| Phase 1, point 9 : 7 règles ATLAS (+ 1 nouvel essai de D-040) | 23 | 32 |

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

### Point 5 — propositions de démo de l'Analyste

`scripts/lancer-demo.sh` (préparation) recopie `demo/scenarios/propositions/*.json` dans `demo/execution/propositions/` quand ce dossier est vide ou absent (jamais par-dessus des propositions présentes). Contenu versionné régénéré en phase 5.

### Point 6 — `ops@cabinet.fr`

Remplacée par `ops@exemple.test` : auteur du registre de démo (`internal/registredemo.Meta`) et du lab (`deploy/lab/registre-lab`), tests du registre et du gate (`internal/registre`, `test/gate`). Restent (non modifiés) : les journaux signés des données de test de l'Analyste (`agents/analyste/tests/donnees/sessions*/`), qu'on ne peut pas réécrire sans casser leur chaîne (voir phase 5) ; et d'autres adresses du même domaine `cabinet.fr` (`juriste@`, `alice@`, `bob@`, `dpo@`, `me.dupont@`), hors de la demande du brief, dont deux fichiers antérieurs au tag `baseline-pre-hackathon` (intouchables) et des parcours du compilateur enregistrés (l'auteur entre dans la requête à Mistral). **Question pour Manu** : faut-il les remplacer aussi (au prix de réenregistrer ces parcours) ?

### Point 7 — consignes des agents crédit

Gardées (décision de Manu). Documenté dans `docs/demo/secteurs.md` (« Une exception, voulue ») ; le test des consignes neutres porte désormais sur **tous** les sous-agents sauf `credit`, `credit-historique`, `conformite`, exception écrite et justifiée dans le test. Pour `assistant-salarie` et `equipe-deal`, « « interne » ou « externe » » (valeurs de l'argument `/destination`) n'est pas une règle et n'est pas compté.

### Point 8 — documentation

- `docs/equipe/modifier-la-console.md` : options `-conseiller`, `-compilateur reel|rejeu`, `-mode-compilateur`, route `GET /api/outils` (libellés), règle « le sondage ne détruit pas les formulaires », tests du navigateur (Playwright) et leur commande.
- `docs/demo/operation.md` : section « Commandes exactes pour dimanche » (clé, préparation, console avec `-conseiller`, les trois scènes sans / avec les règles, en direct et en secours `--mode rejeu`, depuis la console) ; section sur le passage des règles par le compilateur.

### Point 9 — jeu ATLAS passé par le compilateur (23 appels)

Les 7 textes de `operation.json` passés par le service du compilateur en réel (`--jeu-existant demo/directives/demo.json`), réponses de la juriste fidèles au jeu vérifié. Détail : `docs/rapports/gel/compilation-atlas.json`. **Le jeu vérifié est gardé.**

| Règle | Résultat |
|---|---|
| D-041, D-042, D-045 | identiques (D-041/D-042 : termes du « et » dans un autre ordre, équivalent) |
| D-044 | condition de date perdue après la question du fuseau |
| D-040 | inversée deux fois (« refusé si membre des initiés ») — le compilateur ne sait pas écrire « n'appartient à aucun des groupes » ; la relecture le montre |
| D-043 | refusée par les vérifications du code (`session.a_consulte` mal formé) |
| D-046 | impossible : une marque nouvelle n'est pas une étiquette du registre ; le compilateur la refuse |

**Décision (réversible)** : D-040 à D-046 ajoutées à `demo/directives/demo.json` **version 5, en brouillon** (version 4 archivée dans `historique/demo.v4.json`, origines dans `origines.json`, `directive verifier` : ok). Sans cela, la console — qui lance toute demande sous `demo.json` — ne peut pas jouer la scène ATLAS « avec les règles ». Rien n'est activé : Manu décide en phase 2. Les scénarios « sans » (`op-scene-*-sans`) pointent désormais vers `demo/scenarios/jeux/operation-sans-regles.json` (copie figée de la version 4), pour rester « sans règle » quelle que soit l'activation. Tests ajustés : registre de démo avec les serveurs supplémentaires (`directives_test.go`), origines D-040 à D-046, étiquette d'une marque non comptée parmi les étiquettes d'outils (`test_vocabulaire_demo.py`), relectures attendues de la scène (version 6 au lieu de 5).
