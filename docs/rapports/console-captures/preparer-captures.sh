#!/usr/bin/env bash
# Prépare une démonstration de mise au point (règles, annuaire signé, clés
# fictives, quelques sessions produites par le simulateur) dans un dossier
# jetable, et lance la console dessus, pour les captures.
#
#   docs/rapports/console-captures/preparer-captures.sh /tmp/essai 8088
#
# Rien n'est écrit dans le dépôt : le dossier donné reçoit demo/ et bin/.
# La clé du journal est celle des tests, sans valeur. Arrêt : kill $(cat DOSSIER/console.pid).
set -euo pipefail
dossier="$(mkdir -p "${1:?dossier}" && cd "$1" && pwd)"
port="${2:-8088}"
racine="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
export MCP_GATE_CLE_JOURNAL="$(printf 'captures-captures-captures-cap' | xxd -p | tr -d '\n')"
export SIMULATEUR_PAUSE=50ms
mkdir -p "$dossier/bin"
(cd "$racine" && go build -o "$dossier/bin/" ./cmd/console ./cmd/console/simulateur ./cmd/mcp-approve ./cmd/directive)
sim="$dossier/bin/simulateur"
"$sim" preparer -racine "$dossier"
demande() { "$sim" demande -racine "$dossier" --demandeur "$1" "$2" 2>/dev/null || true; }

# Console d'abord : les validations passent par elle.
cd "$dossier"
mkdir -p "$dossier/agents"
cp -R "$racine/internal/console/testdata/compilateur_simule.py" "$dossier/compilateur_simule.py"
nohup "$dossier/bin/console" -racine "$dossier" -addr "127.0.0.1:$port" -dev "$racine/internal/console/web" \
	-approuver "$dossier/bin/mcp-approve" -lanceur "$dossier/bin/simulateur" \
	-compilateur "python3 $dossier/compilateur_simule.py" >"$dossier/console.log" 2>&1 &
echo $! >"$dossier/console.pid"
for _ in $(seq 1 50); do curl -fs "http://127.0.0.1:$port/api/etat" >/dev/null 2>&1 && break; sleep 0.2; done
post() { curl -fs -X POST -H 'Content-Type: application/json' "http://127.0.0.1:$port$1" -d "$2" >/dev/null; }

anna=anna.risques@exemple.test; diane=diane.risques@exemple.test
claire=claire.analyste@exemple.test; bruno=bruno.prestataire@exemple.test
sessions() { ls "$dossier/demo/execution/sessions"; }

demande $claire "Peut-on accorder un prêt sans gel vérifié ?"
demande $bruno "Produis le rapport de synthèse du dossier DOS-FICTIF-002"
demande $claire "Peut-on accorder un prêt de 100 000 euros avec la base interne des taux ?"

# Un prêt validé par deux rôles (quatre yeux), notification à la direction juridique.
avant="$(sessions)"
"$sim" demande -racine "$dossier" --demandeur $claire "Peut-on accorder un prêt de 3 000 000 euros ?" >/dev/null 2>&1 &
pid_gros=$!
for _ in $(seq 1 100); do
	nouvelle="$(comm -13 <(echo "$avant") <(sessions) | head -1)"
	[ -n "$nouvelle" ] && grep -q en_attente "$dossier/demo/execution/sessions/$nouvelle/etat.json" >/dev/null 2>&1 && break
	sleep 0.1
done
post /api/validation/valider "{\"journal\":\"$nouvelle\",\"valideur\":\"$anna\",\"role\":\"analyste-risques\"}"
post /api/validation/valider "{\"journal\":\"$nouvelle\",\"valideur\":\"$diane\",\"role\":\"directeur-risques\"}"
wait "$pid_gros"

# Une session falsifiée, pour voir la chaîne rompue en rouge.
falsifie="20261002-000000-falsif"
cp -R "$dossier/demo/execution/sessions/$(sessions | head -1)" "$dossier/demo/execution/sessions/$falsifie"
sed -i '0,/refusé par D-001/s//autorisé par D-001/' "$dossier/demo/execution/sessions/$falsifie/journal.jsonl"

# Un prêt qui attend encore sa validation (écran « File du superviseur »).
"$sim" demande -racine "$dossier" --demandeur $claire "Peut-on accorder un prêt de 120 000 euros pour le dossier DOS-FICTIF-002 ?" >/dev/null 2>&1 &
sleep 1
echo "console : http://127.0.0.1:$port/  (dossier $dossier)"
