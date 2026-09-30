# Brief de nuit — Session principale : l'agrégateur `cmd/mcp-gate`

Lis d'abord `00_regles_communes.md` (joint) et `docs/contrat-engine.md`.

## Contexte

Produit : le juriste écrit des directives en français ; Mistral les compile en règles structurées ; **l'engine** les applique à chaque appel d'outil MCP d'une chaîne d'agents, et en garde la preuve. Aucun LLM sur le chemin d'exécution.

Déjà fait et commité : `internal/directive` (schéma, chargeur strict, `Decide` pure), `internal/chaine`, `internal/journal` (entrées typées, coffre, purge chaînée, abonnés), `internal/session` (état de session tenu par l'engine, sérialisation par session, délai sur le backend), `internal/rejeu`, `internal/nommage`. Le nommage `serveur__outil` a été **validé en conditions réelles** auprès de l'API Mistral (tirets et `__` acceptés).

Ta mission cette nuit : **la pièce qui relie tout ça à des agents réels** : l'agrégateur.

## Worktree

Branche `feat/gate`, worktree `~/cybercar/mcp_rogue-gate`. Réseau : registres de paquets uniquement. Aucun secret nécessaire.

## Conception : déjà validée, ne la rediscute pas

**Paquet `internal/backend`**. Un backend est une session MCP client vers un serveur, tenue par l'engine, avec une attestation figée à la connexion.

```go
type Backend interface {
    Nom() string
    Attestation() directive.Attestation
    Outils(ctx context.Context) ([]*mcp.Tool, error)
    Appeler(ctx context.Context, outil string, args map[string]any, meta mcp.Meta) (*mcp.CallToolResult, error)
    Fermer() error
}
```

Transports via le go-sdk : **stdio local** (`CommandTransport` après vérification de l'empreinte du binaire, attestation `binaire`) et **HTTP streamable distant** (`StreamableClientTransport`, attestation `tls-pin` : SPKI épinglé et `ServerName` imposé). Le transport **HTTP + SPIFFE** viendra jeudi : prévois seulement l'emplacement. Après connexion : lister les outils, les comparer au schéma épinglé, figer l'attestation. Aucun gestionnaire de sampling ni d'elicitation n'est câblé : ces requêtes initiées par le serveur sont refusées par construction. Une notification `tools/list_changed` déclenche une nouvelle vérification ; en cas de dérive, le backend passe en quarantaine.

**Serveur imposteur ou attestation en échec** : l'engine **n'expose jamais les descriptions d'outils fournies par un serveur non attesté**. Il expose le schéma épinglé du registre, et chaque appel est refusé par la décision `source_attestee` (qui cite la directive et son fondement).

**Registre** : un fichier déclaratif (nom logique conforme à `internal/nommage`, transport, cible, attestation attendue, schémas épinglés, outils en lecture ou en action). Chargé et validé au démarrage, entrée `registre_charge` au journal.

**`cmd/mcp-gate`** : un `mcp.Server` du go-sdk servi en stdio. Chaque outil de chaque backend est exposé sous `serveur__outil`. Une collision de nom est **impossible par construction** (espace de noms par serveur) ; un nom logique en double dans le registre est une erreur de chargement.

**Pipeline d'un `tools/call`**, dans cet ordre, en respectant `docs/contrat-engine.md` :
1. décomposer le nom exposé en (serveur, outil) ;
2. `session.Gestionnaire.Entrer` (appelant : `local-process` en stdio) ;
3. déposer les arguments d'origine au coffre ;
4. `directive.Decide` avec l'horodatage de l'appel, l'état de session et l'attestation du backend ;
5. si `refuser` : entrée `appel` avec `resultat.statut = non_transmis`, réponse d'erreur à l'agent avec le motif en français ;
6. si `attendre_validation` : pour cette nuit, refus avec un motif explicite et un champ structuré `{statut: "en_attente", empreinte_transmise, directive}` (le circuit de reprise avec jeton viendra vendredi) ;
7. sinon : transmettre les arguments **décidés** (éventuellement caviardés) via `Tour.Transmettre`, sceller le résultat comme donnée (paquet `envelope` existant) ;
8. écrire l'entrée `appel` avec l'horodatage passé à `Decide`, puis `Terminer` le tour. Toujours.

**`mcp-proxy` existant ne doit pas être modifié.**

## Ordre de travail

1. `internal/registre` : format, chargement strict, tests. Commit.
2. `internal/backend` : interface, backend stdio, tests avec un petit serveur MCP de test en Go compilé dans le test. Commit.
3. Backend HTTP streamable avec épinglage TLS, testé avec un serveur go-sdk derrière `httptest` en TLS. Commit.
4. `cmd/mcp-gate` : exposition des outils, table des noms, pipeline complet. Commit.
5. **Test de bout en bout** : un gate devant deux backends de transports différents (stdio + HTTP), une directive de provenance chargée. Scénario : l'agent appelle un outil du backend A, puis un outil d'action du backend B ; la seconde décision doit citer la provenance. Vérifier que le journal se rejoue sans écart avec `internal/rejeu`. Commit.
6. Imposteur : un backend dont l'empreinte ne correspond pas au registre ; ses descriptions ne sont pas exposées, ses appels sont refusés par `source_attestee`. Commit.

## Critère de réussite de la nuit

L'étape 5 passe : **deux backends de transports différents routés, provenance tenue par l'engine, journal rejouable sans écart**. C'est le point de contrôle du projet.

## Si tu bloques

- Sur une API du go-sdk : lis son code source dans le cache des modules plutôt que de deviner.
- Si le backend HTTP résiste après 3 tentatives : termine l'étape 4 et 5 avec **deux backends stdio**, note le blocage, et continue sur l'étape 6.
