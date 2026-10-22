#!/usr/bin/env bash
# Console sur la boucle locale, proxy en lecture seule sur le port public.
set -euo pipefail
[[ -n "${MCP_GATE_CLE_JOURNAL:-}" ]] || echo "demarrer: MCP_GATE_CLE_JOURNAL absente : les journaux ne seront pas vérifiés" >&2
/app/console -racine /app -addr 127.0.0.1:8088 -conseiller camille.conseiller@exemple.test -compilateur rejeu &
python3 /app/proxy.py &
wait -n
exit 1
