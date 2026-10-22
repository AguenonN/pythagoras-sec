#!/usr/bin/env bash
# Console live : vrais agents Mistral, gate et serveurs MCP dans le conteneur.
# Secrets : MCP_GATE_CLE_JOURNAL, MISTRAL_API_KEY, DEMO_MOT_DE_PASSE.
set -euo pipefail
cd /app
for v in MCP_GATE_CLE_JOURNAL MISTRAL_API_KEY DEMO_MOT_DE_PASSE; do
	[[ -n "${!v:-}" ]] || echo "demarrer: $v absente" >&2
done
export ORCHESTRATEUR_MAX_APPELS="${ORCHESTRATEUR_MAX_APPELS:-400}"
bin/console -racine /app -addr 127.0.0.1:8088 -conseiller camille.conseiller@exemple.test \
	-lanceur scripts/lanceur-demo-d009.sh &
python3 deploy/console-live/proxy.py &
wait -n
exit 1
