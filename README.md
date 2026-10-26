# pythagoras-sec

A policy engine between AI agents and their MCP tools. Every tool call is decided
by a pure function against versioned directives, written to a MAC-chained journal
and replayable offline. Actions above an authorization threshold wait for human
approval. The identity layer is also published as a standalone public proxy:
**[secu-mcp](https://github.com/AguenonN/secu-mcp)**.

**Problem.** An MCP client trusts a server because a configuration line says so,
written once and never re-checked at runtime. If an agent or a server is
compromised, nothing stands between it and tool execution.

**Architecture.**

- **Identity:** each component presents a SPIFFE SVID at the mTLS handshake; a
  server that cannot is rejected before any of its handlers runs.
- **Decision:** `cmd/mcp-gate` routes every `tools/call` through
  `Decide(rules, session state, call) → decision`, a pure function with no I/O,
  no clock and no language model on the decision path (enforced by an AST test).
  A condition that cannot be evaluated is `Indeterminate`, and the default is to
  refuse.
- **Audit:** every decision is appended to a MAC-chained journal that carries
  everything it read, so "why was this call refused?" is answered by replaying
  the decision, not by explaining it.
- **Containment:** egress default-deny, a capability allowlist before the call
  leaves the process, and tool responses sealed as inert data. These hold even
  when a prompt injection fools the model.

**Validation.** Synthetic banking scenario: a loan approval runs in 12 calls.
With directive D-009 enabled, the action is intercepted, paused for human
validation and completes in 14 calls. Both audit chains pass offline verification.

**Next.** Segmented controls: replacing hand-written rules with chained binary
classifiers whose verdicts combine deterministically, to secure connector code
generated on demand.

> The detailed documentation below is in French.

## La carte des briques

La documentation du dépôt est **[une carte navigable](carte/carte.html)** : un
arbre où chaque brique est un paquet Go, et où les liens sont les imports réels
extraits du code — 48 paquets, 103 arêtes, fondations à gauche, consommateurs à
droite. Cliquer une brique donne ce qu'elle fait, pourquoi elle compte, son
poids, et la liste cliquable de ses dépendances et de ses dépendants.

```bash
make carte        # relit le code, régénère la page
open carte/carte.html
```

Un schéma dessiné à la main se périme au premier refactor ; celui-ci se
régénère. Les annotations éditoriales vivent à part, dans
`carte/annotations.json`, pour que la régénération ne les écrase pas. Voir
[`carte/README.md`](carte/README.md).

Commencer par `internal/directive` : c'est le cœur, et 12 paquets en dépendent.

## Les quatre verrous

| Verrou | Où | Garantie |
|---|---|---|
| **0 — Identité** | `internal/identity` | Le pair doit prouver son SPIFFE ID au handshake mTLS. Sans SVID attesté, pas de session |
| **1 — Egress** | `deploy/gcp/k8s/{40,70}-network-policy*.yaml` | Default-deny réseau, appliqué par le noyau. L'exfiltration meurt avant le premier paquet |
| **2 — Exécution** | `internal/toolpolicy` | Allowlist de capacités **avant** que l'appel quitte le processus |
| **3 — Donnée vs code** | `internal/envelope` | La réponse est scellée dans `<untrusted_data id="nonce">`, et le prompt système la déclare inerte |

Les verrous 1 à 3 tiennent **même si** l'injection réussit à berner le modèle.
L'hypothèse de travail est que le filtre sera contourné.

## Le moteur

`cmd/mcp-gate` expose à l'agent les outils de tous les serveurs du registre sous
le nom `serveur__outil`, et fait passer chaque `tools/call` par le moteur avant
de router. L'agent ne voit jamais ce qu'un serveur annonce : il voit le
catalogue épinglé au registre.

Au centre, `Decide` : `(règles, état de session, appel) → décision`. Pas d'I/O,
pas d'horloge, pas de vérification de jeton — la date vient de l'appel, jamais
de `time.Now()`. Ce n'est pas une intention :
`internal/directive/purete_test.go` parse l'AST du paquet et échoue si un
fichier importe `os`, `net/http`, `context` ou emploie `time.Now`. C'est ce qui
rend le rejeu exact possible, et aucun modèle n'est sur le chemin de la
décision.

La logique est trivalente (Kleene forte, `Faux < Indetermine < Vrai`, `Et` =
min) : une condition non évaluable ne passe pas en silence, elle vaut
`Indetermine` et le défaut est `refuser`. Les effets sont ordonnés
(`Autoriser < Caviarder < AttendreValidation < Refuser`) et le plus contraignant
l'emporte toujours.

Le contrat complet est [`docs/contrat-engine.md`](docs/contrat-engine.md).

## Démarrage rapide

### La preuve, sans Docker ni SPIRE

`internal/testca` mint de vrais SVID en mémoire : les tests traversent de vrais
handshakes TLS et de vrais appels MCP, sans aucune infrastructure.

```bash
go test ./...          # Go >= 1.25 (contrainte du SDK MCP), ou GOTOOLCHAIN=auto
```

Ce que la suite établit, entre autres : le handler du rogue ne s'exécute jamais
en mode zero-trust ; un appel refusé par `toolpolicy` n'atteint pas le
transport ; une réponse contenant un délimiteur forgé est désamorcée et scellée
sous un nonce frais ; `Decide` ne lit ni l'horloge ni le réseau ; un journal
altéré d'un octet est détecté.

### Le moteur

```bash
CGO_ENABLED=0 go build -trimpath -o bin/ ./cmd/mcp-gate ./cmd/directive ./cmd/journal

bin/directive verifier < jeu.json       # {"ok":true,…} ou la liste des erreurs
bin/directive relire   < jeu.json       # la règle en français, telle qu'appliquée

bin/mcp-gate -registre registre.json -directives jeu.json \
             -journal journal.jsonl -coffre coffre.jsonl

bin/journal verifier journal.jsonl
bin/journal rejouer  journal.jsonl -directives jeu.json -coffre coffre.jsonl
```

### Le lab

```bash
make lab-up && make lab-register     # SPIRE, puis entrées — le rogue jamais
make lab-naive-rogue                 # agent naïf → rogue : dévalisé
make lab-zerotrust-rogue             # agent zero-trust → rogue : rejeté au handshake
make lab-zerotrust-legit             # agent zero-trust → légitime : fichier réel
make lab-down
```

Sur cluster k3s (GCP) : `cd deploy/gcp && ./provision.sh`, puis
`setup-vm.sh` et `setup-observability.sh`, et `./provision.sh destroy` pour
arrêter la facturation. Les scénarios se lancent avec
`deploy/gcp/run-scenario.sh` et `deploy/gcp/run-sre.sh`.

Le scénario `compromised` est le plus instructif : l'agent **obéit** à
l'instruction cachée dans le texte d'alerte, et les tentatives échouent sur des
verrous qui n'ont pas eu à reconnaître l'injection.

```
attempt 1  delete_dashboard    → BLOCKED: toolpolicy: "delete_dashboard" not in grant set
attempt 2  annotate exec-board → BLOCKED: toolpolicy: no human approval on record
attempt 3  POST exfil URL      → BLOCKED: lookup exfil.attacker-controlled.test:
                                 no such host
```

## Documentation

| Document | Contenu |
|---|---|
| [carte des briques](carte/carte.html) | Les 48 paquets et leurs liens, navigables |
| [`docs/contrat-engine.md`](docs/contrat-engine.md) | Ce que l'agrégateur doit respecter pour que le journal reste rejouable |
| [`docs/schema-directive-v2.md`](docs/schema-directive-v2.md) | Le schéma d'une règle : déclencheur, condition en arbre, effets, tables de Kleene |
| [`docs/controles-segmentes.md`](docs/controles-segmentes.md) | Refonte en cours : décision déterministe sur observations probabilistes |

`secu-mcp` est l'extraction publique du socle (identité et confinement). Ce
dépôt le contient en entier, plus le moteur, et en est la source de vérité :
les changements vont vers `secu-mcp`, jamais l'inverse.

## Limites d'ingénierie

**Course au démarrage des NetworkPolicies.** Les règles iptables sont
programmées **après** que le pod a reçu son IP. Mesuré sur le cluster : à t+0 s
un `wget` sort vers Internet, à t+20 s il est refusé. Le verrou d'egress tient
contre un workload subverti en cours d'exécution — le cas visé — mais pas contre
un workload qui exfiltre dans sa première seconde.

**Triage déterministe, aucun LLM branché.** `internal/triage` compose la note
d'incident par gabarit. Sous egress default-deny, joindre une API externe
exigerait d'ouvrir une route sortante — exactement la décision de moindre
privilège que le projet sert à rendre visible.

**Vocabulaire applicatif résiduel.** Le moteur a d'abord été écrit pour un
usage juridique. Les types portent encore `texte_fr` et `Fondement{Texte,
Article, URL}`, et la couche de démonstration suppose un catalogue d'outils
fini. Les deux sont du périmètre de nettoyage, pas du cœur ; la carte marque
chaque paquet concerné.

**Autres périmètres assumés.** Le validateur PromQL est syntaxique, pas un
parser complet. Le caviardeur reconnaît des formes, ce n'est pas un
classifieur. `insecure_bootstrap = true` côté agent SPIRE : le lab modélise un
serveur MCP rogue, pas un plan de contrôle SPIRE compromis. Prometheus tourne en
`emptyDir`, rétention 2 h, réplique unique.

## Dépendances

- `github.com/modelcontextprotocol/go-sdk` v1.7.0 — pré-1.0, exige Go ≥ 1.25.
  Point de branchement mTLS : le champ `HTTPClient *http.Client` de
  `mcp.StreamableClientTransport`.
- `github.com/spiffe/go-spiffe/v2` v2.8.1.
- `github.com/go-jose/go-jose/v4` v4.1.4 — jetons d'approbation JWT.

Images pinnées : `spire-server`/`spire-agent` 1.15.3, `prom/prometheus` v3.1.0,
`prom/alertmanager` v0.28.0, `grafana/grafana` 11.4.0.
