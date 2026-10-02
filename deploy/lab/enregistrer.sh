#!/usr/bin/env bash
# Enregistre dans SPIRE les identités du lab : le gate, le serveur de démo et
# l'agent. Le rogue (sa-rogue) n'y est jamais : il n'obtiendra aucune SVID.
# Idempotent. Suppose spire-server-0 prêt (monter.sh).
set -euo pipefail

TD="mcp.gcp.lab"
CLUSTER="mcp-gcp-lab"
NODE_ID="spiffe://${TD}/k3s-node"
EXEC=(kubectl -n spire exec spire-server-0 -c spire-server -- /opt/spire/bin/spire-server)

creer() {
	local sortie
	if sortie="$("${EXEC[@]}" entry create "$@" 2>&1)"; then
		echo "${sortie}"
	elif grep -qi "already exists" <<<"${sortie}"; then
		echo "   (déjà enregistré)"
	else
		echo "${sortie}" >&2
		return 1
	fi
}

echo ">> alias du nœud k3s (attesté par k8s_psat)"
creer -node -spiffeID "${NODE_ID}" -selector "k8s_psat:cluster:${CLUSTER}"

for paire in "gate:sa-gate" "banque-synthetique:sa-banque" "agent:sa-agent"; do
	nom="${paire%%:*}"; sa="${paire##*:}"
	echo ">> spiffe://${TD}/${nom}  (ns mcp-lab, sa ${sa})"
	creer -parentID "${NODE_ID}" -spiffeID "spiffe://${TD}/${nom}" \
		-selector "k8s:ns:mcp-lab" -selector "k8s:sa:${sa}"
done
echo ">> sa-rogue n'a volontairement aucune entrée."
