# Brief de nuit — Session « Agents hors ligne » : compilateur de directives et relecture

Lis d'abord `00_regles_communes.md` (joint).

## Contexte

Produit : le juriste écrit une directive **en français**, avec son fondement juridique. Un agent Mistral la compile en règle structurée. Une **relecture en français, produite sans LLM**, montre au juriste exactement ce qui sera appliqué. Le juriste valide. L'engine applique la règle, sans LLM sur le chemin d'exécution.

Le contrat existe déjà et fait foi : `internal/directive/schema.go` et le chargeur strict `internal/directive/charger.go`. Le jeu d'exemple `internal/directive/testdata/credit-pme.json` montre le format attendu. **Tu ne modifies pas `internal/directive`.**

## Worktree

Branche `feat/agents`, worktree `~/cybercar/mcp_rogue-agents`. Python dans `agents/`, avec un environnement virtuel dans le worktree ; Go uniquement pour les deux outils en ligne de commande ci-dessous.

**Réseau autorisé** : registres de paquets, `api.mistral.ai`. **Secret** : `MISTRAL_API_KEY` dans l'environnement.

## Principe non négociable

**Mistral propose, le code vérifie, le juriste décide.** Aucune sortie de Mistral n'est acceptée sans passer par le chargeur strict en Go. Mistral ne choisit jamais une valeur que le juriste n'a pas écrite : face à une ambiguïté, il **pose une question**.

## Ordre de travail

### 1. `cmd/directive` (Go) : deux sous-commandes
- `directive verifier` : lit un jeu JSON sur l'entrée standard, appelle le chargeur strict, rend `ok` ou **la liste complète des erreurs** en JSON, code de sortie non nul en cas d'échec.
- `directive relire` : lit un jeu JSON valide et produit, **par gabarit déterministe**, sa relecture en français. Pour chaque directive : identifiant, version, statut, fondement, période de validité, date de révision, cible, conditions, provenance, champs, effet. Exemple de phrase : « Directive D-001, provisoire jusqu'au 31/12/2026, fondée sur l'art. 22 RGPD. Pour tous les appelants : si la session a reçu des données du serveur taux-officiel, tout appel à attribuer_pret sur banque-synthetique exige une validation humaine. »

Le gabarit vit dans un nouveau paquet `internal/relecture`. Tests par table sur `credit-pme.json` : même entrée, même sortie, octet pour octet. Commit.

### 2. Le compilateur (Python, `agents/compilateur/`)
Entrée : le texte de la directive du juriste, l'identifiant du jeu, et la liste des serveurs et outils disponibles (fichier fourni en argument).

Fonctionnement :
1. Appel Mistral en **function calling**, avec deux outils possibles :
   - `proposer_directive(...)` : paramètres calqués sur le schéma, **verrou limité par une énumération** aux six verrous du schéma, serveurs et outils limités par énumération à ceux de la liste fournie ;
   - `poser_question(question, champ, options)` : quand une information manque ou est ambiguë (un seuil non chiffré, un serveur non nommé, une durée « en attendant » sans date, « de manière autonome » qui pourrait signifier validation ou interdiction, un périmètre flou).
2. Si Mistral propose : le JSON est passé à `directive verifier`. En cas d'erreur, les erreurs sont renvoyées à Mistral **une seule fois** pour correction ; au second échec, on rend les erreurs au juriste.
3. Si c'est valide : `directive relire` produit la relecture, affichée avec le texte d'origine du juriste, côte à côte.
4. **Le statut de toute directive produite est `brouillon`.** Seul le juriste la passe en `observation` ou `active`, hors de ce programme.
5. `si_indetermine: ignorer` n'est **jamais** proposé par Mistral : il n'est accepté que si le texte du juriste le demande explicitement.
6. `texte_fr` contient le texte du juriste **tel quel**.

### 3. Jeu de tests du compilateur
Enregistre les réponses de Mistral (au plus 40 appels réels sur la nuit), puis teste en rejeu. Cas minimaux :
- la directive d'exemple : « En application de l'art. 22 RGPD, en attendant des solutions concrètes, plus aucun workload ne doit utiliser de manière autonome les informations du serveur de taux officiel pour attribuer un prêt. » → **doit produire des questions** (date de révision, sens de « de manière autonome ») ;
- la même avec les réponses aux questions → directive valide, relecture conforme ;
- un seuil non chiffré (« au-delà d'un montant significatif ») → question, jamais un montant inventé ;
- un serveur inexistant → question ou erreur, jamais un serveur inventé ;
- une directive de champs autorisés vers un partenaire (« le serveur partenaire ne reçoit que le SIREN ») → verrou `champs_autorises_par_destination`, champ `/siren`.

Commit.

### 4. Si tout est fait
Un petit script `agents/compilateur/demo.py` qui enchaîne, en mode texte, texte du juriste → questions → réponses saisies → relecture → fichier de jeu en `brouillon`. C'est ce que l'interface juriste appellera plus tard.

## Critère de réussite de la nuit

La directive d'exemple produit les bonnes questions, puis, une fois complétée, une directive **acceptée par le chargeur strict** et une relecture en français **identique à chaque exécution**.
