# RAPPORT_F — session F « kit d'extension »

Branche `feat/kit-extension`. Aucun appel Mistral, aucun réseau hors modules Go. Aucun fichier de l'engine modifié (`internal/directive`, `journal`, `rejeu`, `session`, `gate`).

## Fait

| Étape | Livrable | Commit |
|---|---|---|
| 1 | `scripts/nouveau-serveur.sh <nom> <donnees.json> <outils.json> [etiquettes.json]` et `--regenerer <nom>` ; moteur `servers/modele-synthetique/` (genres d'outil `lire`, `lister`, `action`, validation stricte), gabarits, `valider`, `outillage.py` | 5b588bc, cf63c03 |
| 2 | `scripts/nouveau-scenario.sh <nom> [options]` : `demo/scenarios/<nom>.json` + `test/extension/scenario_<nom>_test.go` (rejeu hors ligne, ignoré tant que rien n'est enregistré) | d191381 |
| 3 | Sous-agent documenté, exemple commenté champ par champ, dans `docs/demo/etendre.md` | a8422bf |
| 4 | `docs/demo/etendre.md` : quatre recettes + « ce qu'on ne touche pas, et pourquoi » ; les blocs ` ```sh recette-… ` sont exécutés tels quels par `test/extension/` | a8422bf |
| 5 | `docs/demo/fiche-juriste.md` (une page) | a8422bf |
| 6 | Domaines `assurance` et `contentieux`, données seulement, dans `servers/modele-synthetique/domaines/` (assurance : pièces de santé étiquetées `donnees_sante`, art. 9 ; contentieux : dépôt de conclusions, échéances) | 5b588bc |

Critère de réussite : `go test ./test/extension/` (environ 1 min, sans réseau) crée sur une copie du dépôt, **en exécutant etendre.md à la lettre**, un serveur synthétique, un sous-agent, une directive (console réelle, compilateur bouchonné) et un scénario ; prouve que le gate atteste le serveur, que ses outils sont appelables (lecture autorisée, action classée `action`), que la directive D-009 refuse l'indemnisation de 240 000 € et laisse passer celle de 3 200 €, que le scénario se rejoue hors ligne (« attendu : conforme »), que le test généré passe, que la chaîne du journal est intègre. Les refus (nom non conforme à `internal/nommage`, données non fictives, outil sans `readOnlyHint`, étiquette sur outil inconnu, etc.) laissent le dépôt inchangé. Le temps réel des commandes du kit est d'environ une minute ; les 15 minutes restantes sont de la rédaction.

Vérifié : `go vet ./...`, `go test -race ./...`, tests Python des agents (191, OK), tous verts.

## Pas fait / limites

- **Compilateur réel non testé** (Mistral interdit en test) : l'étape « Compiler » de la recette 3 est remplacée par un bouchon (`test/extension/testdata/compilateur_bouchon.py`) ; la console (`Depot.Compiler`, `MettreEnVigueur`), le chargeur et l'écriture des règles sont les vrais. À essayer à la main avec Mistral : texte de l'exemple, sur un serveur créé.
- **L'enregistrement du scénario est écrit à la main** dans le test (format 1 du lanceur, champ `modele` explicite « écrit à la main… aucun appel Mistral »). Il prouve la chaîne orchestrateur → sous-agent → gate → serveur créé → directive, pas le comportement de Mistral devant le nouveau sous-agent.
- `scripts/lancer-demo.sh` (préparation, avec téléchargement du registre des gels) n'est pas exécuté par les tests ; ils en reproduisent les étapes sans réseau (`preparerSansReseau`).
- Un scénario dont l'attente est une validation humaine n'est pas couvert par un test (les scénarios existants le sont par la démo réelle).
- La directive de l'exemple est un refus ; une attente de validation par rôle suit le même chemin que D-002 / D-005.

## Décisions prises seul

1. **Étiquettes et serveurs ajoutés : dans `demo/serveurs-supplementaires.json`, pas dans `demo/etiquettes.json`** (le brief disait `etiquettes.json`). Raison : des tests versionnés (registre de démo, vocabulaire Go et Python, intégration) appliquent `etiquettes.json` aux huit serveurs de la démo et rejettent un serveur inconnu ; et le vocabulaire du compilateur en dérive, or les enregistrements de rejeu du compilateur dépendent du texte exact de ce vocabulaire (essayé : 16 tests Python rouges). Ainsi la démo figée et ses secours hors ligne restent intacts.
2. **Catalogue et vocabulaire étendus écrits dans `demo/execution/extension/`** (hors Git) et lus par le compilateur via `COMPILATEUR_CATALOGUE` / `COMPILATEUR_VOCABULAIRE` (le script affiche les `export`). Raison idem. Sans ces variables, le compilateur ne connaît pas le nouveau serveur.
3. **Édition hors périmètre, minimale et rétrocompatible** (nécessaire au critère « passent devant le gate ») : `internal/registredemo/registredemo.go` (lecture de `demo/serveurs-supplementaires.json`, option `Supplementaires`, appliquée seulement si le fichier existe), `cmd/registre-demo/{main,vocabulaire}.go` (lecture du fichier ; option `-etendu` ; création du dossier de sortie), `agents/compilateur/service.py` (deux variables d'environnement pour les chemins par défaut), `internal/registredemo/catalogue_test.go` (le test compare aux seuls serveurs de la démo). Sans le fichier, comportement strictement inchangé. Si Manu préfère, ces quatre éditions se rejouent à part ; elles ne touchent ni l'engine ni le gate.
4. **Moteur commun plutôt que génération de code** : le serveur créé n'a aucun Go propre (il embarque `outils.json` et `donnees.json`), pour qu'un défaut se corrige une fois. Conséquence : seuls trois genres d'outils (`lire`, `lister`, `action`). Un besoin hors de ces genres demande du code (à signaler).
5. **Fermé par défaut** : `readOnlyHint` obligatoire et cohérent avec le genre ; l'avertissement « fictif » obligatoire dans les données ; arguments inconnus refusés ; une action est en mémoire (rien ne survit au processus).
6. `nouveau-scenario.sh` ajoute à `attendu` les clés `sous_agents` et `decisions` (par `serveur/outil`) ; `lancer-demo.sh` les ignore, seul le test généré les lit. Un refus de l'engine **bloque** la demande (statut `bloque`, code de sortie 2) : le gabarit et la recette le prévoient.
7. Le test généré lit `MCP_GATE_CLE_JOURNAL` dans l'environnement (jamais de clé inventée) : le gate relit les journaux des passages précédents et refuse de démarrer si l'un a été signé par une autre clé. Sans clé, le test est ignoré avec un message.
8. `docs/equipe/ajouter-un-serveur.md` (session A, hors périmètre) décrit l'ancien `nouveau-serveur.sh <nom>` (squelette Go avec `lire_fiche`) et l'ajout manuel à `registredemo.go` : **devenu faux**. Non modifié (hors périmètre) ; `etendre.md` fait foi.
9. Le fichier `nuit/brief_F_kit_extension.md` n'est pas commité (consigne de la session).

## Blocages

Aucun bloquant. Le test du kit prend environ 35 s (copie du dépôt, 10 binaires, go build) : acceptable en CI, mais à exclure d'un `-short` si l'équipe en ajoute un.

## Questions pour Manu

1. (Avant samedi) Acceptez-vous les quatre éditions hors périmètre de la décision 3 ? Alternative : les retirer, et le kit n'ajoute plus de serveur au registre.
2. (Avant samedi) Mettre à jour ou retirer `docs/equipe/ajouter-un-serveur.md` (décision 8).
3. Genre d'outil manquant pour dimanche (par exemple une action qui échoue sur condition de données, ou une lecture avec jointure) ?
4. Essayer le compilateur réel (Mistral) sur le texte de la recette 3 avec le serveur créé : 1 à 3 appels, à faire par vous.

## Vérifier en 5 minutes

```sh
go vet ./... && go test -race ./...                    # tout vert
go test ./test/extension/ -v -count=1                  # la preuve du kit (~1 min)
git diff 5b83cd5 --stat -- internal/directive internal/journal internal/rejeu internal/session internal/gate   # vide : l'engine n'a pas bougé
# À la main, sur une copie :
D=servers/modele-synthetique/domaines/contentieux
scripts/nouveau-serveur.sh cabinet-contentieux $D/donnees.json $D/outils.json $D/etiquettes.json
scripts/nouveau-scenario.sh mon-scenario --sous-agents credit --statut termine
```
