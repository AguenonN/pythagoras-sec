# Étendre la démo : quatre recettes

Tout se fait **par des fichiers et des gabarits**. Aucune recette ne demande de lire ni de modifier le code de l'engine. Un ingénieur qui n'a jamais vu le dépôt les enchaîne en moins de 15 minutes. `test/extension/` exécute les blocs de commandes ci-dessous **tels quels** (ceux marqués `sh recette-…`) sur une copie du dépôt, et vérifie que le serveur, le sous-agent, la directive et le scénario passent devant le gate.

Prérequis : Go 1.25, Python 3, une copie du dépôt. Les chemins sont relatifs à la racine du dépôt.

```sh
go test ./test/extension/        # la preuve : environ une minute, sans réseau ni Mistral
```

Ordre conseillé : **1. serveur → 2. sous-agent → 3. directive → 4. scénario** (la directive et le scénario de l'exemple visent le serveur de la recette 1). Le fil rouge : un assureur fictif ; un agent instruit un sinistre et tente d'indemniser ; une règle l'en empêche au-delà de 100 000 €.

---

## Recette 1 — Un serveur de données synthétiques (5 minutes)

### Concevoir des outils gouvernables

Une directive ne juge que ce que l'engine observe : le nom de l'outil, ses arguments, ses étiquettes, son accès. **Un outil mal découpé ne peut pas être gouverné, quelle que soit la règle.** Six règles, avec le mauvais et le bon exemple :

1. **Séparer préparer et agir.** On peut laisser un agent préparer librement ; on gouverne l'acte.
   Mauvais : `gerer_indemnisation(sinistre_id, ...)`, qui calcule et paie. Bon : `simuler_indemnisation` (lecture) puis `valider_indemnisation` (action) ; de même `rediger_conclusions` / `deposer_conclusions`.
2. **Un outil par effet irréversible.** Jamais d'outil « exécuter » générique.
   Mauvais : `executer(operation, parametres)`. Bon : `valider_indemnisation`, `deposer_conclusions`, chacun avec sa directive possible.
3. **Arguments explicites** (qui, combien, vers où), jamais d'instructions en texte libre sur un outil d'action.
   Mauvais : `deposer(instructions: "dépose les conclusions du dossier 4 chez le greffe de Lyon")`. Bon : `deposer_conclusions(dossier_id, titre, pieces_jointes)`. Une règle peut comparer `/montant > 100000` ; elle ne peut pas lire une phrase. `nouveau-serveur.sh` **avertit** d'un argument nommé `instructions`, `texte` ou `commande` sur un outil d'action.
4. **Étiqueter les champs sensibles des réponses.** Ce qui n'est pas étiqueté ne peut être ni caviardé ni tracé (« a consulté des données de santé »).
   Mauvais : `lire_piece` sans étiquette. Bon : `{"outils": {"lire_piece": ["donnees_sante"]}}` dans `etiquettes.json`.
5. **Qualifier la destination : `interne` ou `externe`.** Une directive peut viser « tout ce qui sort ».
   Mauvais : un dépôt auprès d'une juridiction sans étiquette. Bon : `deposer_conclusions` étiqueté `externe`, les lectures de la base du cabinet non.
6. **`readOnlyHint` juste.** `true` seulement si l'outil ne change rien nulle part ; `false` pour toute action. Un outil sans l'annotation est classé « action » (fermé par défaut), et un `true` à tort soustrait l'action aux règles qui visent l'accès `action`. Mauvais : `valider_indemnisation` avec `readOnlyHint: true` « parce qu'elle ne supprime rien ». Bon : `false` ; `true` seulement pour `lire_sinistre`. `nouveau-serveur.sh` **refuse** un outil sans `readOnlyHint` explicite, ou incohérent avec son genre.

Un serveur = deux fichiers JSON (les outils, les données **fictives**). Deux domaines sont prêts dans `servers/modele-synthetique/domaines/` : `assurance` et `contentieux`. Pour le vôtre, copiez-en un et modifiez-le (formats ci-dessous).

```sh recette-serveur
D=servers/modele-synthetique/domaines/assurance
scripts/nouveau-serveur.sh assurance-synthetique $D/donnees.json $D/outils.json $D/etiquettes.json
```

Le script valide les fichiers (rien n'est créé s'ils sont faux), crée `servers/assurance-synthetique/`, ajoute le serveur à `scripts/construire-serveurs.sh` et à `demo/serveurs-supplementaires.json` (avec ses étiquettes), reconstruit, régénère le registre et le catalogue du compilateur, puis lance le test du serveur. Ensuite, sur une machine connectée (le registre des gels est téléchargé) :

```sh
scripts/lancer-demo.sh        # « gate prêt … 9 serveur(s) attesté(s) sur 9 »
```

Les outils sont exposés à l'agent sous `assurance-synthetique__lire_sinistre`, etc. **Pour que le compilateur et la console connaissent le serveur**, exporter, avant de lancer la console, les deux variables que le script affiche (`COMPILATEUR_CATALOGUE`, `COMPILATEUR_VOCABULAIRE` : fichiers de `demo/execution/extension/`).

**`outils.json`** : une liste d'outils.

| Champ | Sens |
|---|---|
| `nom`, `description` | nom (minuscules, chiffres, `_`) et phrase lue par le modèle : soignez-la |
| `genre` | `lire` : un enregistrement dont les champs égalent les arguments ; `lister` : ceux qui satisfont les filtres donnés (tous sans filtre) ; `action` : enregistre une action **en mémoire** et rend une référence `PREFIXE-0001`… |
| `readOnlyHint` | **obligatoire** : `true` pour `lire` et `lister` (accès « lecture »), `false` pour `action` (accès « action », fermé par défaut). Une incohérence est refusée |
| `collection` | (`lire`, `lister`) la collection de `donnees.json` |
| `arguments` | `nom`, `type` (`string`, `integer`, `number`, `boolean`, `liste` de chaînes pour une action), `description`, `obligatoire`, `champ` (champ comparé, par défaut le nom) |
| `reference_prefixe`, `verifie` | (`action`) préfixe de la référence ; `verifie` refuse l'action si l'identifiant n'existe pas dans une collection |

**`donnees.json`** : `{"avertissement": "… FICTIVES …", "collections": {"nom": [ {…}, … ]}}`. L'avertissement doit dire que les données sont fictives (il est répété dans chaque résultat). **Jamais de donnée réelle.**

**`etiquettes.json`** (facultatif) : `{"serveur": ["donnees_client"], "outils": {"lire_piece": ["donnees_sante"]}}` : ce que les directives peuvent viser (`etiquette: …`).

Après avoir modifié `outils.json` ou `donnees.json` d'un serveur créé : `scripts/nouveau-serveur.sh --regenerer <nom>`.

---

## Recette 2 — Un sous-agent (3 minutes)

Un sous-agent = un fichier `agents/orchestrateur/sous_agents/<nom>.json`. L'orchestrateur le découvre à chaque demande.

```sh recette-sous-agent
cat > agents/orchestrateur/sous_agents/instruction-sinistre.json <<'JSON'
{
  "nom": "instruction-sinistre",
  "role": "instruit un sinistre d'assurance et tente d'en valider l'indemnisation.",
  "consigne": "Tu es le sous-agent instruction-sinistre. Lis le sinistre avec assurance-synthetique__lire_sinistre. Si la tâche le demande, valide l'indemnisation avec assurance-synthetique__valider_indemnisation (sinistre_id et montant). N'invente aucun résultat ; si un outil refuse, dis-le.",
  "outils": ["assurance-synthetique__*"],
  "schema_sortie": {
    "type": "object",
    "properties": {
      "sinistre_id": {"type": "string"},
      "decision": {"type": "string", "enum": ["validee", "refusee"]},
      "montant": {"type": ["number", "null"]},
      "reference": {"type": ["string", "null"]},
      "motif": {"type": "string"}
    },
    "required": ["sinistre_id", "decision", "motif"]
  }
}
JSON
(cd agents && python3 -m orchestrateur sous-agents)
```

La dernière commande liste chaque sous-agent avec ses outils ; une erreur nomme le fichier et la cause (outil absent du registre, champ inconnu…). **Anatomie, champ par champ :**

```jsonc
{
  "nom": "instruction-sinistre",        // = nom du fichier ; minuscules, chiffres, tirets
  "role": "…",                          // une ligne : l'orchestrateur la lit pour choisir à qui déléguer
  "consigne": "…",                      // une DEMANDE au modèle, pas une règle : une règle s'écrit en directive
  "outils": ["serveur__outil", "serveur__*"],   // seuls outils que ce sous-agent voit (« * » : tout le serveur)
  "schema_sortie": { … }                // forme de sa réponse finale ; mots-clés admis : type, properties,
}                                       // required, items, enum, additionalProperties (booléen), description
```

Le code pose `_meta.sous_agent` sur chaque appel : le journal sait quel sous-agent a appelé quoi. Pour retirer un sous-agent : supprimer le fichier.

---

## Recette 3 — Une directive : texte → compilateur → console → activation (5 minutes)

1. **Écrire le texte** selon `docs/demo/fiche-juriste.md` (source, seuil chiffré, rôle, fuseau, fondement). Exemple : *« Indemnisation d'un sinistre : une indemnisation supérieure à 100 000 euros n'est jamais validée par un agent. »*, fondement *« Politique d'indemnisation — à valider par le juriste »*.
2. **Console → Rédaction** : coller le texte et le fondement, **Compiler** (le compilateur appelle Mistral ; clé `MISTRAL_API_KEY` dans l'environnement de la console). Répondre aux questions éventuelles ; relire la **relecture en français** (écrite par le code, pas par le modèle).
3. **Mettre en vigueur** (observation ou active) : la console écrit la version n+1 de `demo/directives/demo.json` et archive l'ancienne dans `demo/directives/historique/`. Rien n'est activé sans cette action.
4. **Vérifier** : le jeu est accepté par le chargeur strict, et la prochaine demande applique la règle.

```sh recette-directive
go run ./cmd/directive verifier < demo/directives/demo.json
go run ./cmd/directive relire --directives D-009 < demo/directives/demo.json
```

Si le compilateur répond « non observable » ou pose une question : voir la fiche juriste (reformulation). Le test remplace l'étape 2 par un compilateur-bouchon (jamais d'appel Mistral dans un test) ; la console, le chargeur et l'écriture des règles sont les vrais.

---

## Recette 4 — Un scénario (3 minutes)

Un scénario = un fichier `demo/scenarios/<nom>.json` et un test qui le rejoue hors ligne.

```sh recette-scenario
scripts/nouveau-scenario.sh indemnisation-incendie \
  --demande "Peut-on indemniser le sinistre SIN-FICTIF-004 (incendie) à hauteur de 240 000 euros ?" \
  --demandeur camille.conseiller@exemple.test \
  --sous-agents instruction-sinistre \
  --decision assurance-synthetique/valider_indemnisation=refuser \
  --statut bloque \
  --directives D-009
```

Le fichier fixe la demande, le demandeur (une identité de `demo/annuaire-source.json`), les sous-agents attendus, la décision attendue de l'engine par outil (`serveur/outil=effet`, effet parmi `autoriser`, `refuser`, `caviarder`, `attendre_validation`) et le statut final (`bloque` : un refus de l'engine arrête la demande ; `termine` sinon) et les directives que la réponse doit citer. Compléter `titre` et `description`, puis **enregistrer un passage réel** (Mistral, budget compté) :

```sh
export MCP_GATE_CLE_JOURNAL=$(openssl rand -hex 32)   # à garder pour relire les journaux
scripts/lancer-demo.sh scenario indemnisation-incendie
```

À une attente de validation : `scripts/lancer-demo.sh valider <empreinte> --valideur <id> --role <rôle>`. Les réponses du modèle sont enregistrées dans `demo/scenarios/enregistrements/<nom>/`. **Secours hors ligne**, sans réseau ni Mistral :

```sh recette-scenario-rejeu
scripts/lancer-demo.sh scenario indemnisation-incendie --mode rejeu
```

La sortie doit finir par « attendu : conforme » (code de sortie 0 si la demande se termine, 2 si l'engine la bloque, comme ici : c'est le résultat attendu). Le test généré `test/extension/scenario_indemnisation_incendie_test.go` fait la même chose et vérifie en plus les sous-agents, les décisions par outil et la chaîne du journal : `go test ./test/extension -run Scenario_indemnisation_incendie` (ignoré tant que rien n'est enregistré, que `scripts/lancer-demo.sh` n'a pas préparé la démo, ou que `MCP_GATE_CLE_JOURNAL` n'est pas exportée : la même clé que pour les passages, car le gate relit les journaux précédents).

---

## Ce qu'on ne touche pas, et pourquoi

| Ne pas modifier | Pourquoi |
|---|---|
| `internal/directive`, `internal/journal`, `internal/rejeu`, `internal/session`, `internal/gate` | L'engine : gelé le samedi. Il décide, journalise et rejoue ; ce qu'il a décidé doit pouvoir être rejoué à l'identique. Tout ce que vous ajoutez (directive, serveur, sous-agent, scénario) passe par lui sans le changer. |
| Une version déjà écrite de `demo/directives/demo.json` | Le gate refuse une version réécrite et un autre identifiant de jeu : chaque changement est la version n+1, archivée. Passez par la console. |
| `agents/orchestrateur/*.py`, `servers/internal/lanceur`, `servers/modele-synthetique/modele.go` | Le moteur commun des sous-agents et des serveurs : un défaut y touche tous les serveurs. Ajoutez un fichier, pas du code. Besoin d'un genre d'outil absent : le signaler. |
| `demo/etiquettes.json`, `agents/compilateur/exemples/*-demo.json` | La démo figée : les enregistrements du compilateur en dépendent. Les étiquettes de vos serveurs vont dans `demo/serveurs-supplementaires.json` (le script s'en charge). |
| Les journaux (`demo/execution/sessions/*/journal.jsonl`) | Chaînés par HMAC : une ligne changée casse la chaîne, c'est voulu. |

Besoin de modifier la console : `docs/equipe/modifier-la-console.md`.
