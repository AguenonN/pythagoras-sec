# Rapport de nuit — session « Agents hors ligne »

Branche `feat/agents`, worktree `~/cybercar/mcp_rogue-agents`. Rien n'a été poussé ni fusionné. Je n'ai touché ni à `internal/directive` ni aux autres paquets protégés, ni à aucun fichier antérieur à `baseline-pre-hackathon`.

**Critère de réussite atteint.** La directive d'exemple produit les deux questions attendues : l'effet (validation humaine ou refus) et la date de relecture. Une fois complétée, elle donne une directive `provenance_session` **acceptée par le chargeur strict**, avec une relecture **identique octet pour octet** à chaque exécution (fichier attendu : `agents/tests/attendus/d001.txt`).

## 1. Fait

| Commit | Contenu |
|---|---|
| `0a6c69e` | `cmd/directive` (`verifier`, `relire`) et `internal/relecture` (gabarit français déterministe) ; tests par table et fichiers attendus, rejoués sous trois fuseaux horaires |
| `ca5fc8c` | `agents/compilateur` : function calling Mistral (`proposer_directive` / `poser_question`), vérifications anti-invention, chargeur strict, une seule correction, relecture côte à côte ; 30 tests unitaires hors ligne |
| `ef77605` | Scénarios du brief rejoués sur des réponses réelles enregistrées : exemple D-001, seuil non chiffré, serveur inexistant, champs vers le partenaire |
| `07ca8d1` | `agents/compilateur/demo.py` : texte → questions → réponses saisies → relecture → fichier de jeu en `brouillon` |
| `30bb5bf` | Tour de correction validé contre l'API réelle, puis rejoué |
| `b304af7` | Noms de serveurs limités aux six du compilateur (test dédié) ; noms de remplacement des tests de relecture alignés sur cette liste |

Au total : 45 tests Python, tous hors ligne, plus les tests Go de `internal/relecture` et `cmd/directive`. `go vet ./...` et `go test -race ./...` passent sur tout le dépôt.

**Appels Mistral réels : 10 sur les 40 autorisés** (compteur dans `agents/.etat/budget.json`). Les 8 réponses utiles sont enregistrées dans `agents/compilateur/enregistrements/`. Deux autres étaient devenues inutiles après la correction de la consigne ; je les ai supprimées.

Résultats réels obtenus :
- **D-001** : deux questions (effet, `revision_avant`), puis `provenance_session`, cible `banque-synthetique/attribuer_pret`, provenance `taux-officiel`, effet `attendre_validation`, à relire avant le 31/12/2026.
- **« au-delà d'un montant significatif »** : une question sur le montant, jamais de montant inventé. Avec la réponse « 500 000 euros », on obtient `seuil_argument`, `/montant > 500000`.
- **Serveur « Notanova »**, absent du catalogue : une question qui cite Notanova et les serveurs du catalogue, sans en choisir un.
- **« le serveur partenaire ne reçoit que le SIREN »** : Mistral demande d'abord de confirmer qu'il s'agit de `recherche-entreprises`. Après « oui », on obtient `champs_autorises_par_destination`, `/siren`, `caviarder`.

## 2. Pas fait, et pourquoi

- Tout le brief est fait, étape 4 comprise.
- Pas de relance réelle du cas Notanova après une réponse du juriste. La question suffit au critère (« question ou erreur »), et cela économise le budget.
- Le compilateur produit **une directive par texte**. Un texte qui en contient plusieurs devra être découpé en amont. Je n'ai pas testé ce que fait Mistral dans ce cas.

## 3. Décisions prises seul

1. **« provisoire jusqu'au » n'est pas repris.** Dans le schéma, `revision_avant` est une échéance de relecture : une fois passée, la directive **reste appliquée** et une alerte est émise (`schema.go`, `Decide`). « Provisoire jusqu'au » laisserait croire qu'elle s'arrête. La relecture écrit donc : « à relire avant le 31/12/2026 à 00:00 UTC (passée cette date, la directive reste appliquée et une alerte est émise) ». Le reste de la phrase d'exemple est repris mot pour mot : « Pour tous les appelants : si la session a reçu des données du serveur taux-officiel, tout appel à attribuer_pret sur banque-synthetique exige une validation humaine. »
2. **Dates toujours en UTC, avec l'heure**, jamais dans le fuseau de la machine, pour que la sortie soit identique partout.
3. **La relecture ne recopie pas `texte_fr`.** Elle montre la règle compilée ; le compilateur affiche le texte du juriste à côté.
4. **La relecture explicite ce que `Decide` fait implicitement** :
   - une condition indéterminée refuse l'appel, même si l'effet demandé est une validation ;
   - un appelant non prouvé tombe sous toute directive à périmètre ;
   - un champ autorisé couvre tout son contenu ;
   - un statut autre qu'`active` est suivi de « non appliquée ».
5. **La date d'effet n'est jamais fixée par Mistral.** C'est un paramètre obligatoire (`--date-effet`, demandé par la démo). Mistral propose la date de relecture, mais le code la refuse si elle n'est pas écrite dans le texte ou les réponses du juriste.
6. **`si_indetermine: ignorer` est hors de portée de Mistral.** Il n'apparaît pas dans son outil, et le code refuse toute proposition qui le contient. On ne l'obtient que par `--ignorer-indetermine-valide-par NOM` : une demande explicite et nommée, comme l'exige le chargeur. Sinon, `si_indetermine: refuser` est posé explicitement sur toute directive à condition.
7. **Vérifications anti-invention en Python, avant le chargeur Go.** Chaque montant, chaîne, date et fondement doit figurer dans le texte ou les réponses du juriste. Chaque serveur, outil, champ et appelant doit figurer au catalogue, et l'outil doit appartenir au serveur ciblé. Les énumérations du function calling ne suffisent pas à elles seules.
8. **Consigne corrigée après le premier essai réel.** Mistral avait compilé D-001 en `approbation_humaine` : le chargeur l'acceptait, mais la condition « informations du serveur de taux officiel » avait disparu. J'ai ajouté deux choses à la consigne : l'interdiction de retirer une condition, et la règle « utiliser les informations de A pour faire B ⇒ `provenance_session` ». Le chargeur n'aurait rien vu : c'est la relecture qui sert de garde-fou.
9. **Fondement normalisé par le code** : « Art. 22 RGPD » devient `texte: RGPD`, `article: 22`. Même contenu, découpé autrement, rien d'ajouté.
10. **« ne reçoit que X » sans précision donne `caviarder`**, comme `D-004` dans le jeu d'exemple. `refuser` seulement si le juriste demande de bloquer l'appel.
11. **Si Mistral pose une question en même temps qu'il propose, la question l'emporte.**
12. **Bibliothèque standard uniquement** (urllib, unittest), comme `pyagent/`. Le venv `agents/.venv` existe mais n'a aucune dépendance.
13. **`.gitignore` à la racine non modifié**, car il date d'avant la baseline. Un `agents/.gitignore` couvre `.venv`, `.bin`, `.etat` et `__pycache__`.
14. `--mode rejeu` est le mode par défaut du CLI et de la démo. Un appel réel exige `--mode reel`.
15. `RAPPORT_NUIT.md` et `nuit/` ne sont pas commités : ce sont des documents de travail, pas du code.
16. **unittest plutôt que pytest.** Toute la suite est écrite en `unittest` : pytest n'est pas installé, le venv reste sans dépendance. Commande exacte, depuis `agents/` : `.venv/bin/python -m unittest discover -s tests -t .` (45 tests, OK).
17. **Noms de serveurs.** Les seuls noms du compilateur sont `taux-officiel`, `banque-synthetique`, `recherche-entreprises`, `gels-avoirs`, `eurlex` et `legifrance`. Le test `agents/tests/test_noms_serveurs.py` le vérifie sur le catalogue, sur les énumérations offertes à Mistral, sur ses propositions enregistrées et sur les exemples. Le catalogue d'exemple ne contient que les trois premiers. Je n'ai **pas** ajouté `gels-avoirs`, `eurlex` et `legifrance` : il aurait fallu inventer leurs outils et leurs arguments, et réenregistrer tout le rejeu, puisque le catalogue entre dans l'empreinte des requêtes. Seules exceptions voulues, dans les tests négatifs : `notation-externe` (serveur inventé que le code doit refuser) et « Notanova » (serveur inexistant cité par le juriste). Dans `cmd/directive/main_test.go`, `Taux-Officiel` est une graphie volontairement invalide (majuscules), qui teste le refus par le chargeur.

## 4. Blocages et incidents

- Aucun blocage.
- **Incident de procédure sur `30bb5bf`.** Mon script de contrôle de secret a échoué sur un chemin relatif erroné, et le commit est parti quand même, parce que j'avais enchaîné avec `;` au lieu de `&&`. J'ai refait les contrôles juste après sur ce commit : aucune clé, `go vet` et `-race` verts, tests Python verts. Rien à corriger.
- **Fragilité connue du rejeu.** Les enregistrements sont indexés par l'empreinte de la requête complète : consigne, catalogue, texte et réponses. Toute modification de la consigne ou du catalogue d'exemple casse le rejeu : les tests lèvent alors `EnregistrementAbsent`. Il faut alors réenregistrer avec `--mode reel` (environ 8 appels).

## 5. Questions pour Manu

Par urgence décroissante :

1. **Échéance de relecture** (décision 1) : garder « à relire avant », ou le produit veut-il une directive qui expire ? Dans le second cas, il faut changer le schéma `internal/directive`, ce qui sort de mon périmètre.
2. **« avant le 31/12/2026 »** : je stocke la date telle qu'écrite (`2026-12-31T00:00:00Z`) et la relecture l'affiche ainsi. Faut-il plutôt lire « le 31/12 inclus » et stocker `2027-01-01T00:00Z` ? C'est une règle d'interprétation à fixer côté produit.
3. **Le « serveur partenaire »** : Mistral demande confirmation au lieu de le déduire de la description du catalogue. C'est prudent, mais cela ajoute un tour. Est-ce voulu ?
4. **Catalogue** : faut-il ajouter `gels-avoirs`, `eurlex` et `legifrance` au catalogue d'exemple ? Si oui, donne-moi leurs outils et arguments réels (décision 17).
5. **`si_indetermine: ignorer`** doit-il rester réservé au CLI (`--ignorer-indetermine-valide-par`), ou l'interface juriste doit-elle proposer une case dédiée ?

## 6. Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-agents
git log --oneline 69e32ee..HEAD
go vet ./... && go test -race -count=1 ./internal/relecture ./cmd/directive
go run ./cmd/directive verifier < internal/directive/testdata/credit-pme.json
go run ./cmd/directive relire   < internal/directive/testdata/credit-pme.json

cd agents
python3 -m venv .venv            # si absent
.venv/bin/python -m unittest discover -s tests -t .     # 45 tests, aucun appel réseau

T="En application de l'art. 22 RGPD, en attendant des solutions concrètes, plus aucun workload ne doit utiliser de manière autonome les informations du serveur de taux officiel pour attribuer un prêt."
C="--jeu credit-pme --catalogue compilateur/exemples/catalogue-credit-pme.json --auteur juriste@cabinet.fr --date-effet 2026-10-04"
.venv/bin/python -m compilateur --texte "$T" $C                                                    # questions (code 3)
.venv/bin/python -m compilateur --texte "$T" $C --reponses compilateur/exemples/reponses-d001.json  # relecture côte à côte (code 0)

# Démo interactive (rejeu) : coller le texte, 2026-10-04, puis les deux réponses de reponses-d001.json
.venv/bin/python -m compilateur.demo --jeu credit-pme --catalogue compilateur/exemples/catalogue-credit-pme.json \
  --auteur juriste@cabinet.fr --sortie /tmp/brouillon.json
```
