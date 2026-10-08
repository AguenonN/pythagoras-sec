# mcp_rogue

Deux couches sur une même question : **que vaut la confiance qu'un agent
accorde à ses outils ?**

- **Le socle** répond à « à qui je parle ». En MCP, un agent fait confiance à
  un serveur sur la foi d'une entrée de configuration écrite une fois et jamais
  revérifiée à l'exécution. Le socle remplace cette confiance déclarative par
  une vérification cryptographique continue (SPIFFE/SPIRE, mTLS) et par trois
  verrous de réduction du rayon d'action qui tiennent même quand un filtre
  cède.
- **Le moteur** répond à « cet appel a-t-il le droit d'avoir lieu, et peut-on
  le prouver dans six mois ». Chaque `tools/call` passe par une décision pure,
  journalisée dans une chaîne infalsifiable, et rejouable à l'identique.

Le socle est publié séparément sous [`secu-mcp`](https://github.com/AguenonN/secu-mcp).
Ce dépôt le contient en entier, plus le moteur. Il est la source de vérité :
`secu-mcp` en est une extraction, à sens unique.

---

## Couche 1 — le socle

### Threat model : deux moitiés à ne pas confondre

|  | Question | Nature | Traitement |
|---|---|---|---|
| **Identité** | « Qui es-tu ? » | Binaire, prouvable | **Résolue** par SPIFFE/mTLS |
| **Comportement** | « Que fais-tu ? » | Continue, non prédictible | **Contenue**, pas résolue |

La moitié 2 est incontournable : un serveur dûment authentifié peut dériver, et
du contenu empoisonné transite sans obstacle par un service légitime. Elle est
filtrée au niveau applicatif (`internal/guardrail`) puis — un filtre textuel
finissant toujours par céder face à l'obfuscation, aux langues rares ou au
Base64 — confinée par des verrous qui ne dépendent d'aucun filtre.

Il est impossible d'injecter un prompt dans une route réseau absente, un
privilège non accordé ou un type de donnée inerte.

### Les quatre verrous

| Verrou | Où | Garantie |
|---|---|---|
| **0 — Identité** | `internal/identity` | Le pair doit prouver son SPIFFE ID au handshake mTLS. Sans SVID attesté, pas de session |
| **1 — Egress** | `deploy/gcp/k8s/{40,70}-network-policy*.yaml` | Default-deny réseau, appliqué par le noyau. L'exfiltration meurt avant le premier paquet |
| **2 — Exécution** | `internal/toolpolicy` | Allowlist de capacités **avant** que l'appel quitte le processus. Séparation lecture/action, approbation humaine nommant sa cible |
| **3 — Donnée vs code** | `internal/envelope` | La réponse est scellée dans `<untrusted_data id="nonce">`, nonce de 16 octets tiré par scellement, et le prompt système la déclare inerte |

Les verrous 1 à 3 tiennent **même si** l'injection réussit à berner le modèle.
L'hypothèse de travail est que le filtre sera contourné.

### Le lab

```
┌─ VM GCP e2-standard-2 · k3s single-node · CNI kube-router ────────────────────┐
│                                                                              │
│  ns spire                                                                    │
│    spire-server   CA racine, trust domain mcp.gcp.lab, SVID 1 h              │
│    spire-agent    DaemonSet, attestation de nœud k8s_psat                    │
│                                                                              │
│  ns mcp-lab                        SPIFFE ID              NetworkPolicy      │
│    agent-sre  ─────────────┐       …/agent-sre            egress: bridge+DNS │
│                            │ mTLS                                            │
│    mcp-obs-bridge  ◀───────┘       …/obs-bridge           détient le token   │
│         │                                                  Grafana           │
│    network-config                  …/network-config       lab synthétique    │
│    rogue                           AUCUN — jamais enregistré dans SPIRE      │
│         │                                                                    │
│  ns monitoring                                                               │
│    prometheus :9090   alertmanager :9093   grafana :3000   checkout-api :8080│
└──────────────────────────────────────────────────────────────────────────────┘
```

- **Lab synthétique** — `network-config` (légitime) et `rogue` (malveillant)
  exposent le **même nom d'outil** `get_file` : c'est la collision qui rend
  l'usurpation possible. Le rogue possède un ServiceAccount mais aucune entrée
  SPIRE, donc aucun SVID. `TRUST_MODE=naive` reproduit MCP tel quel et l'agent
  se fait dévaliser ; `TRUST_MODE=zerotrust` le rejette avant tout appel.
- **Service SRE** — `agent-sre` lit la télémétrie et annote les dashboards à
  travers `mcp-obs-bridge`. L'asymétrie est le design : **l'agent reçoit un
  outil, jamais un token**. Un agent convaincu d'exfiltrer la clé API Grafana
  n'a rien à exfiltrer, et aucune route réseau pour le faire.

`cmd/mcp-proxy` généralise le dispositif : un sidecar qui interpose les verrous
0, 2 et 3 devant n'importe quel serveur MCP tiers, sans modifier son code. Le
verrou 1 reste appliqué au niveau pod/réseau — un processus ne se l'applique
pas à lui-même : l'upstream écoute sur loopback et le pod porte une politique
d'egress default-deny.

---

## Couche 2 — le moteur

Le socle sécurise le canal. Le moteur décide de ce qui passe dedans, et le
prouve.

`cmd/mcp-gate` est un **agrégateur** : un serveur MCP qui expose à l'agent les
outils de tous les serveurs du registre sous le nom `serveur__outil`, et fait
passer chaque `tools/call` par le moteur avant de le router. L'agent ne voit
jamais ce qu'un serveur annonce : il voit le catalogue épinglé au registre.

### Pipeline d'un appel

Le contrat complet est [`docs/contrat-engine.md`](docs/contrat-engine.md).

1. décomposer le nom exposé en (serveur, outil) ;
2. entrer dans la session — verrou tenu jusqu'à la fin ;
3. lire l'horloge **une seule fois**, déposer les arguments d'origine au coffre ;
4. `Decide`, avec cette date, l'état de session et l'attestation ;
5. refus → entrée `non_transmis`, erreur motivée à l'agent ;
6. attente de validation → refus explicite et champ structuré `en_attente` ;
7. sinon, écrire l'entrée `transmission`, transmettre les arguments décidés,
   sceller le résultat ;
8. écrire l'entrée `appel` à la date passée à `Decide`.

### La propriété centrale : `Decide` est pure

```
Decide(jeu de règles, état de session, appel) → décision
```

Pas d'I/O, pas d'horloge, pas de vérification de jeton. La date vient de
l'appel, jamais de `time.Now()`. **Aucun modèle n'est sur le chemin de la
décision.**

Ce n'est pas une intention : `internal/directive/purete_test.go` parse l'AST de
tous les fichiers non-test du paquet et échoue si l'un importe `os`, `net`,
`net/http`, `context`, `regexp`, `reflect`, `math/rand`, ou emploie `time.Now`,
`LoadLocation`, `Sleep`. C'est ce qui rend le rejeu exact possible.

Deux conséquences de conception :

- **Logique trivalente** (Kleene forte, `Faux < Indetermine < Vrai`, `Et` =
  min). Une condition non évaluable ne passe pas en silence : elle vaut
  `Indetermine`, et le défaut est `refuser`. L'option `ignorer` n'est acceptée
  que validée nommément, et le nom est consigné.
- **Effets ordonnés** : `Autoriser < Caviarder < AttendreValidation <
  Refuser`. `Decide` retient toujours le plus contraignant.

### La boucle complète

| Étape | Composant | Rôle |
|---|---|---|
| Écriture | `agents/compilateur` | Texte → règle structurée. **Seul usage de modèle du système**, et uniquement à l'écriture |
| Vérification | `cmd/directive verifier` | Chargeur strict, refuse tout champ hors schéma. Seul juge de ce que le compilateur propose |
| Relecture | `internal/relecture` | Rend la règle en français **par gabarit déterministe, sans modèle** : la règle telle que le moteur l'appliquera, pas le texte écrit. Même jeu, même texte, octet pour octet |
| Application | `cmd/mcp-gate` | Le pipeline ci-dessus |
| Preuve | `internal/chaine`, `internal/journal` | JSONL chaîné par MAC ; l'entrée `appel` contient tout ce que `Decide` a lu |
| Audit | `cmd/journal verifier\|rejouer` | Vérifie la chaîne, puis rejoue chaque appel sous le jeu d'origine, à sa date |

### Le rejeu qualifie les écarts

`internal/rejeu` ne dit pas seulement « ça diverge », il nomme la cause :

- **purge** — le coffre porte une pierre tombale et le journal une entrée
  `purge` chaînée qui la couvre : l'appel n'est plus rejouable, son entrée
  reste vérifiable ;
- **altération** — tout le reste : arguments modifiés ou disparus sans trace,
  jeu différent, décision qui ne se retrouve pas ;
- **effet de règle** — le rejeu est forcé sous un autre jeu ou une autre date,
  et la décision diffère. C'est attendu : c'est ce qu'on voulait mesurer.

### Contrôles hors règles

`internal/controle` tient ce que le moteur impose sans qu'aucune règle ne le
demande : un serveur dont l'attestation n'est pas valide est refusé, même si
rien ne le cible. L'engine et le rejeu appellent la **même fonction**, pour que
ce qu'affirme le journal se recalcule à l'identique.

### Habilitations

`internal/annuaire` est la source des identités, groupes, rôles et dates
d'effet. Le fichier est signé Ed25519 par une autorité dont **la clé publique
est fournie à part, jamais lue dans le fichier lui-même** : une signature
vérifiée avec la clé qu'elle apporte ne prouve rien. La référence de l'annuaire
(identifiant, version, empreinte) est écrite au journal, pour que le rejeu sache
contre quoi tout a été vérifié.

---

## Carte du dépôt

### Socle

| Chemin | Rôle |
|---|---|
| `internal/identity` | `tls.Config`/`http.Client` des deux modes, interface `Verifier` découplant le verrou 0 de SPIRE (`spiffe`, `mesh` XFCC, `local`) |
| `internal/toolpolicy` | Middleware `CallTool`. Refus par défaut ; un appel refusé n'atteint jamais la session inner |
| `internal/approval` | HITL : interface `Approver`, webhook, jetons JWT `_meta.approvalToken`, trail d'audit chaîné HMAC |
| `internal/toolpin` | Anti rug-pull : pinning SHA-256 des schémas dans `tools.lock.json`, quarantaine `ERR_TOOL_SCHEMA_MUTATED` |
| `internal/envelope` | Scellement `<untrusted_data>`, défangage (`Defang`), clause de prompt système |
| `internal/scrub` | Caviardage : jetons, secrets par nom de clé, userinfo d'URL, emails, IP routables uniquement |
| `internal/promql` | Politique de coût PromQL : 1 Kio, fenêtre 6 h, 8 sélecteurs, 6 matchers regex |
| `internal/proxy` | Reverse-proxy MCP générique portant les verrous 0, 2 et 3 |
| `internal/guardrail` | Filtre de contenu : schéma strict → marqueurs → caviardage |
| `internal/labconfig` | L'entrée de configuration de confiance — l'artefact que la thèse attaque |
| `internal/obsserver`, `obsclient`, `obstool`, `triage` | Bridge d'observabilité et ses contrats |
| `internal/server`, `mcptool` | Les deux serveurs du lab synthétique |
| `internal/testca`, `identitytest` | CA jetable émettant de vrais SVID pour les tests |

### Moteur

| Chemin | Rôle |
|---|---|
| `internal/directive` | Schéma d'un jeu de règles versionné, chargeur strict, et `Decide` — pur, trivalent, vérifié par test |
| `internal/gate` | L'agrégateur : exposition `serveur__outil`, pipeline de décision, rechargement à chaud |
| `internal/session` | État de session tenu par le moteur, jamais déclaré par l'agent |
| `internal/journal` | Entrées typées ; l'entrée `appel` porte tout ce que `Decide` a lu |
| `internal/chaine` | JSONL chaîné par MAC : retirer, réordonner ou modifier une ligne se voit |
| `internal/rejeu` | Rejoue `Decide` à l'identique et qualifie chaque écart |
| `internal/registre` | Serveurs acceptés, attestation attendue, catalogue d'outils épinglé |
| `internal/annuaire` | Habilitations signées, résolution du demandeur et des valideurs |
| `internal/relecture` | Rendu d'un jeu de règles en français, par gabarit, sans modèle |
| `internal/controle` | Contrôles toujours actifs, hors règles, appliqués après `Decide` |
| `internal/nommage` | Règles de nommage partagées par le registre, les règles et l'agrégateur |
| `agents/compilateur` | Texte → règle structurée (Mistral). Seul usage de modèle du système |

### Commandes

| Binaire | Rôle |
|---|---|
| `cmd/mcp-gate` | L'agrégateur et le moteur |
| `cmd/directive` | `verifier`, `relire`, `fuseaux` — le juge de ce que le compilateur propose |
| `cmd/journal` | `verifier`, `rejouer` — l'audit |
| `cmd/mcp-proxy` | Sidecar portant les verrous 0, 2 et 3 devant un serveur tiers |
| `cmd/mcp-approve` | Émission d'un jeton d'approbation (forme lab / bris de glace) |
| `cmd/mcp-audit` | Vérification d'un trail d'approbation |
| `cmd/agent`, `network-config`, `rogue`, `mcp-observability`, `agent-sre`, `checkout-api` | Le lab |

---

## Démarrage rapide

### Preuve exécutable, sans Docker ni SPIRE

La logique de sécurité est prouvée par la suite de tests, indépendamment de
toute infrastructure : `internal/testca` mint de vrais SVID en mémoire et les
tests traversent de vrais handshakes TLS et de vrais appels MCP.

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
CGO_ENABLED=0 go build -trimpath -o bin/ ./cmd/mcp-gate ./cmd/directive ./cmd/journal ./cmd/mcp-approve

bin/directive verifier < jeu.json       # {"ok":true,...} ou la liste des erreurs
bin/directive relire   < jeu.json       # la règle en français, telle qu'appliquée

bin/mcp-gate -registre registre.json -directives jeu.json \
             -journal journal.jsonl -coffre coffre.jsonl

bin/journal verifier journal.jsonl
bin/journal rejouer  journal.jsonl -directives jeu.json -coffre coffre.jsonl
```

### Lab conteneurisé (SPIRE réel)

```bash
make lab-up && make lab-register     # SPIRE, puis entrées — le rogue jamais
make lab-naive-rogue                 # agent naïf → rogue : dévalisé
make lab-zerotrust-rogue             # agent zero-trust → rogue : rejeté au handshake
make lab-zerotrust-legit             # agent zero-trust → légitime : fichier réel
make lab-down
```

### Cluster k3s sur GCP

```bash
cd deploy/gcp && ./provision.sh                          # VM, k3s, SPIRE, lab
gcloud compute ssh mcp-rogue-lab --zone europe-west1-b   # GCP_ZONE par défaut
bash ~/mcp_rogue/deploy/gcp/setup-vm.sh                  # lab synthétique
bash ~/mcp_rogue/deploy/gcp/setup-observability.sh       # service SRE
./provision.sh destroy                                   # arrêter la facturation
```

```bash
bash deploy/gcp/run-scenario.sh naive-rogue|zerotrust-rogue|zerotrust-legit
bash deploy/gcp/run-sre.sh break        # « déploiement v2.1.0 » : checkout-api tombe
bash deploy/gcp/run-sre.sh incident     # passe d'astreinte, approbation en place
bash deploy/gcp/run-sre.sh unapproved   # lectures OK, écriture refusée
bash deploy/gcp/run-sre.sh compromised  # l'agent obéit à l'injection ; tout tombe
```

Le job `compromised` est le plus instructif : l'agent **obéit** à l'instruction
cachée dans le texte d'alerte, et les tentatives échouent sur des verrous qui
n'ont pas eu à reconnaître l'injection.

```
attempt 1  delete_dashboard    → BLOCKED: toolpolicy: "delete_dashboard" not in grant set
attempt 2  annotate exec-board → BLOCKED: toolpolicy: "annotate_dashboard" refused by
                                 approver: no human approval on record
attempt 3  POST exfil URL      → BLOCKED: lookup exfil.attacker-controlled.test:
                                 no such host
```

---

## Documentation

| Document | Contenu |
|---|---|
| [`docs/contrat-engine.md`](docs/contrat-engine.md) | Ce que l'agrégateur doit respecter pour que le journal reste rejouable |
| [`docs/schema-directive-v2.md`](docs/schema-directive-v2.md) | Le schéma d'une règle : déclencheur, condition en arbre, effets, tables de Kleene |
| [`docs/controles-segmentes.md`](docs/controles-segmentes.md) | Refonte en cours : décision déterministe sur observations probabilistes |

---

## Limites d'ingénierie

**Course au démarrage des NetworkPolicies.** Les règles iptables sont
programmées **après** que le pod a reçu son IP. Mesuré sur le cluster : à t+0 s
un `wget` sort vers Internet, à t+20 s il est refusé. Le verrou d'egress tient
contre un workload subverti en cours d'exécution — le cas visé — mais pas contre
un workload qui exfiltre dans sa première seconde. Fermer la fenêtre demande une
application antérieure au workload : un CNI programmant la policy avant de
rendre le réseau, ou une passerelle d'egress obligatoire.

**Triage déterministe, aucun LLM branché.** `internal/triage` compose la note
d'incident par template. Sous egress default-deny, joindre une API externe
exigerait d'ouvrir une route sortante — exactement la décision de moindre
privilège que le projet sert à rendre visible. Brancher un modèle ne change rien
aux verrous : il réécrit la phrase, il ne choisit pas quels outils sont appelés.

**Vocabulaire applicatif résiduel.** Le moteur a d'abord été écrit pour un
usage juridique. Les types portent encore `texte_fr` et `Fondement{Texte,
Article, URL}`, et la couche de démonstration (`internal/console`,
`internal/registredemo`, `servers/*`, `agents/analyste`) suppose un catalogue
d'outils fini. Les deux sont du périmètre de nettoyage, pas du cœur.

**Autres périmètres assumés.** Le validateur PromQL est syntaxique, pas un
parser complet. Le caviardeur reconnaît des formes, ce n'est pas un
classifieur. `insecure_bootstrap = true` côté agent SPIRE : le lab modélise un
serveur MCP rogue, pas un plan de contrôle SPIRE compromis. Prometheus tourne
en `emptyDir`, rétention 2 h, réplique unique. Le mot de passe admin Grafana est
généré au premier déploiement et ne quitte jamais le cluster.

---

## Dépendances

- `github.com/modelcontextprotocol/go-sdk` v1.7.0 — pré-1.0, exige Go ≥ 1.25.
  Point de branchement mTLS : le champ `HTTPClient *http.Client` de
  `mcp.StreamableClientTransport`.
- `github.com/spiffe/go-spiffe/v2` v2.8.1.
- `github.com/go-jose/go-jose/v4` v4.1.4 — signature/vérification des jetons
  d'approbation JWT.

Images pinnées : `spire-server`/`spire-agent` 1.15.3, `prom/prometheus` v3.1.0,
`prom/alertmanager` v0.28.0, `grafana/grafana` 11.4.0.
