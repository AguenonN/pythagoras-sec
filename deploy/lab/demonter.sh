#!/usr/bin/env bash
# Retire le lab (gate, serveur de démo, comptes, configuration, clé du
# journal). Garde SPIRE et k3s : DEMONTER_SPIRE=1 retire aussi SPIRE
# (deploy/gcp/k8s). k3s lui-même se désinstalle avec k3s-uninstall.sh (sudo).
set -uo pipefail
racine="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
lab="$racine/deploy/lab"

kubectl delete -f "$lab/lab.yaml" --ignore-not-found
kubectl -n mcp-lab delete configmap registre-lab directives-lab --ignore-not-found
kubectl -n mcp-lab delete secret cle-journal --ignore-not-found
if [[ "${DEMONTER_SPIRE:-}" == "1" ]]; then
	kubectl delete -f "$racine/deploy/gcp/k8s/20-spire-agent.yaml" --ignore-not-found
	kubectl delete -f "$racine/deploy/gcp/k8s/10-spire-server.yaml" --ignore-not-found
	kubectl delete -f "$racine/deploy/gcp/k8s/00-namespaces.yaml" --ignore-not-found
fi
echo ">> lab démonté"
