# Lab SPIFFE du gate

Le gate (`mcp-gate`) et un serveur de démo (`banque-synthetique`, en HTTP
streamable) tiennent chacun une SVID SPIRE ; l'agent aussi. Ce lab montre les
trois verrous d'identité du gate, de bout en bout, dans un cluster k3s :

```
 agent (SVID agent) ──mTLS──▶ gate (SVID gate) ──mTLS──▶ sidecar spiffe-proxy ──▶ banque-synthetique
   appelant prouvé             épingle spiffe://…/banque-synthetique             (boucle locale du pod)
   consigné au journal         attestation « spiffe » consignée
 rogue (aucune SVID) ──✗ pas d'identité, rien n'est appelé
```

- **Gate → serveur** : transport `http-spiffe` du registre. Le gate exige du
  serveur une SVID X.509 du bundle de confiance dont l'identité est exactement
  `attestation.attendu`. L'attestation consignée porte l'identité observée, le
  trust domain et l'expiration de la SVID.
- **Agent → gate** : `mcp-gate -ecoute-spiffe :9443 -appelants-spiffe <id,…>`.
  Seuls les SPIFFE ID listés passent la poignée de main ; l'appelant consigné au
  journal (`session_ouverte`, `appel`) est celui du certificat client.
- **Le serveur de démo ne parle pas SPIFFE** : il sert du TLS avec un certificat
  jetable sur la boucle locale de son pod, derrière le sidecar
  `deploy/lab/spiffe-proxy`, qui porte sa SVID. En production, le serveur
  porterait lui-même sa SVID.

La démo de dimanche reste en stdio (appelant `local-process`) ; ce lab n'en
change aucun journal.

## Ce que ce dépôt a vérifié, et ce qu'il n'a pas pu

Vérifié sans SPIRE (`go test ./internal/backend ./internal/gate ./deploy/lab/...`),
avec une autorité de test qui émet de vraies SVID X.509 (`internal/identitytest`) :
identité conforme, identité différente, autorité inconnue, SVID expirée, autre
trust domain, appelant prouvé, session volée par un autre SPIFFE ID, sidecar
qui ne laisse passer que le gate, registre et jeu du lab acceptés par les
chargeurs.

**Non exécuté** : les scripts de ce répertoire n'ont jamais tourné. Le poste de
développement n'a ni k3s ni les droits (`sudo`) pour l'installer ; `bash -n` et
une lecture du YAML sont les seuls contrôles. Le premier montage réel peut
révéler des ajustements (noms d'images, délais de readiness).

## Prérequis

- k3s installé et démarré (`curl -sfL https://get.k3s.io | sudo sh -s - --write-kubeconfig-mode 644`),
  `export KUBECONFIG=/etc/rancher/k3s/k3s.yaml` ; ou tout cluster où l'on peut
  charger une image locale (variable `IMPORT_IMAGE`).
- `docker`, `kubectl`, `go` (1.25+), `openssl`, `python3`.
- Réseau : images `golang`, `distroless`, `ghcr.io/spiffe/spire-*` (voir `deploy/gcp/k8s`).

## Procédure

```
deploy/lab/monter.sh      # image, SPIRE, identités, registre, gate, serveur
deploy/lab/verifier.sh    # cinq contrôles, code de sortie 0 si conforme
deploy/lab/demonter.sh    # retire le lab (DEMONTER_SPIRE=1 : et SPIRE)
```

`monter.sh` réutilise le SPIRE de `deploy/gcp/k8s` (trust domain `mcp.gcp.lab`,
attestation de nœud `k8s_psat`, socket `/run/spire/sockets/api.sock`). Il
importe l'image avec `docker save | sudo k3s ctr images import -` : c'est la
seule étape à privilèges ; sur un cluster `kind`,
`IMPORT_IMAGE="kind load docker-image mcp-gate-lab:local"`.

La clé du journal est tirée à la volée (`openssl rand -hex 32`) et ne vit que
dans le Secret `cle-journal` ; aucun fichier ne la porte. Le journal et le
coffre sont dans l'`emptyDir` du pod du gate : ils disparaissent avec lui.

## Identités (enregistrer.sh)

| ServiceAccount | SPIFFE ID | Rôle |
|---|---|---|
| `sa-gate` | `spiffe://mcp.gcp.lab/gate` | gate |
| `sa-banque` | `spiffe://mcp.gcp.lab/banque-synthetique` | serveur de démo (sidecar) |
| `sa-agent` | `spiffe://mcp.gcp.lab/agent` | seul appelant admis par le gate |
| `sa-rogue` | *aucune entrée* | n'obtient jamais de SVID |

## Ce que verifier.sh contrôle

1. le journal du gate contient l'attestation `spiffe` du serveur ;
2. l'agent appelle `banque-synthetique__attribuer_pret` à travers le gate ;
3. la chaîne du journal se vérifie (`journal verifier`) et l'appelant consigné
   est `spiffe://mcp.gcp.lab/agent` ;
4. le rogue n'obtient aucune identité (la sonde s'arrête avant tout appel) ;
5. une charge à SVID valide mais non admise (`sa-banque`) est refusée à la
   poignée de main.

## Pour la diapositive d'architecture

Trois identités, deux sauts mTLS, un journal chaîné qui consigne qui a appelé
(prouvé) et quel serveur a été attesté (identité, trust domain, expiration).
