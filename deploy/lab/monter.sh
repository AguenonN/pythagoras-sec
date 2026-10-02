#!/usr/bin/env bash
# Monte le lab SPIFFE du gate sur un cluster k3s (ou tout cluster accessible
# par kubectl) : SPIRE, identités, serveur de démo, gate. Voir README.md.
#
# Prérequis (non faits ici : ils demandent sudo) : k3s installé et démarré,
# docker, kubectl avec KUBECONFIG sur le cluster, et le droit d'importer une
# image dans containerd (IMPORT_IMAGE, ci-dessous).
set -euo pipefail

racine="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
lab="$racine/deploy/lab"
gcp="$racine/deploy/gcp"
image="mcp-gate-lab:local"
# Comment l'image entre dans le cluster. k3s : containerd, donc ctr (sudo).
# kind : « kind load docker-image $image ». Autre : l'adapter.
IMPORT_IMAGE="${IMPORT_IMAGE:-docker save $image | sudo k3s ctr images import -}"

for c in docker kubectl go openssl; do
	command -v "$c" >/dev/null || { echo "monter: $c absent" >&2; exit 1; }
done
kubectl get nodes >/dev/null || { echo "monter: kubectl ne joint aucun cluster (KUBECONFIG ?)" >&2; exit 1; }

echo ">> [1/6] image $image"
docker build -t "$image" -f "$lab/Dockerfile" "$racine"
echo ">> [2/6] import de l'image dans le cluster : $IMPORT_IMAGE"
bash -c "$IMPORT_IMAGE"

echo ">> [3/6] SPIRE (serveur puis agent), réutilisés de deploy/gcp/k8s"
kubectl apply -f "$gcp/k8s/00-namespaces.yaml"
kubectl apply -f "$gcp/k8s/10-spire-server.yaml"
kubectl -n spire rollout status statefulset/spire-server --timeout=300s
kubectl apply -f "$gcp/k8s/20-spire-agent.yaml"
kubectl -n spire rollout status daemonset/spire-agent --timeout=300s

echo ">> [4/6] identités SPIRE (gate, serveur, agent ; jamais le rogue)"
"$lab/enregistrer.sh"

echo ">> [5/6] registre, directives et clé du journal"
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
(cd "$racine" && go run ./deploy/lab/registre-lab \
	-url "https://banque-synthetique.mcp-lab.svc.cluster.local:8443/mcp" \
	-spiffe-id "spiffe://mcp.gcp.lab/banque-synthetique" -sortie "$tmp/registre.json")
kubectl -n mcp-lab create configmap registre-lab --from-file=registre.json="$tmp/registre.json" \
	--dry-run=client -o yaml | kubectl apply -f -
kubectl -n mcp-lab create configmap directives-lab --from-file=directives.json="$lab/directives-lab.json" \
	--dry-run=client -o yaml | kubectl apply -f -
# La clé n'existe que dans le Secret du cluster ; une clé déjà posée est gardée
# (changer de clé romprait la chaîne d'un journal existant).
if ! kubectl -n mcp-lab get secret cle-journal >/dev/null 2>&1; then
	kubectl -n mcp-lab create secret generic cle-journal --from-literal=cle="$(openssl rand -hex 32)"
fi

echo ">> [6/6] serveur de démo et gate"
kubectl apply -f "$lab/lab.yaml"
kubectl -n mcp-lab rollout status deployment/banque-synthetique --timeout=300s
kubectl -n mcp-lab rollout status deployment/gate --timeout=300s
echo ">> Prêt. Vérifier : deploy/lab/verifier.sh"
