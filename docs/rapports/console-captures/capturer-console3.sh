#!/usr/bin/env bash
# Captures de la session console-3 : la console (binaire réel, interface
# embarquée) dans Chromium (Playwright), sur une démonstration produite par le
# simulateur d'orchestrateur et de gate (cmd/console/simulateur), jetons signés
# par mcp-approve, compilateur simulé en mode « réel ». Aucun appel Mistral.
#
#   docs/rapports/console-captures/capturer-console3.sh <dossier jetable> <python avec playwright>
#
# Les images vont dans docs/rapports/console-captures/reel/.
set -euo pipefail
dossier="$(mkdir -p "${1:?dossier}" && cd "$1" && pwd)"
py="${2:?python avec playwright}"
port=8093
ici="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
racine="$(cd "$ici/../../.." && pwd)"
export MCP_GATE_CLE_JOURNAL="$(printf 'captures-console3-captures-cap' | xxd -p | tr -d '\n')"
export SIMULATEUR_PAUSE=50ms
mkdir -p "$dossier/bin"
(cd "$racine" && go build -o "$dossier/bin/" ./cmd/console ./cmd/console/simulateur ./cmd/mcp-approve ./cmd/directive)
sim="$dossier/bin/simulateur"
"$sim" preparer -racine "$dossier"
cp "$racine/internal/console/testdata/compilateur_simule.py" "$dossier/compilateur_simule.py"
mkdir -p "$dossier/agents/compilateur/exemples"
cp "$racine/agents/compilateur/exemples/catalogue-demo.json" "$dossier/agents/compilateur/exemples/"
demande() { "$sim" demande -racine "$dossier" --demandeur "$1" "$2" >/dev/null 2>&1 || true; }
claire=claire.analyste@exemple.test; bruno=bruno.prestataire@exemple.test

demande $claire "Peut-on accorder un prêt sans gel vérifié ?"
demande $bruno "Produis le rapport de synthèse du dossier DOS-FICTIF-002"
# Une réponse en Markdown avec le relevé de l'engine, comme l'orchestrateur
# les écrit (fichiers écrits ici pour la capture : le simulateur rend du
# texte simple).
d="$dossier/demo/execution/sessions/20261003-090000-mdwn"
mkdir -p "$d"
cat >"$d/etat.json" <<JSON
{"statut":"termine","demande":"Le dossier DOS-FICTIF-003 est-il recevable ?","demandeur":"$claire","debut":"2026-10-03T07:00:00Z","fin":"2026-10-03T07:00:09Z","attente":null}
JSON
cat >"$d/reponse.json" <<'JSON'
{"texte":"Le dossier **DOS-FICTIF-003** est recevable sur le fond.\n\nPoints vérifiés :\n- gels des avoirs : aucune correspondance pour le dirigeant ;\n- seuil de l'usure : consulté via banque-synthetique__get_taux_interne ;\n- montant demandé : **150 000 €**, dans la délégation du conseiller.\n\nL'attribution reste soumise à la validation d'un analyste risques.\n\nDécisions de l'engine :\n- analyse-dossier · banque-synthetique__evaluer_dossier : transmis, données reçues\n- verification · gels-avoirs__verifier_gel : transmis, données reçues\n- decision · banque-synthetique__attribuer_pret : en attente de validation (D-002)","statut":"termine","directives_citees":["D-002"]}
JSON

cd "$dossier"
"$dossier/bin/console" -racine "$dossier" -addr "127.0.0.1:$port" -approuver "$dossier/bin/mcp-approve" \
	-lanceur "$sim" -compilateur "python3 $dossier/compilateur_simule.py" -mode-compilateur reel \
	-conseiller "$claire" >"$dossier/console.log" 2>&1 &
pid=$!
trap 'kill $pid 2>/dev/null || true' EXIT
for _ in $(seq 1 50); do curl -fs "http://127.0.0.1:$port/api/etat" >/dev/null 2>&1 && break; sleep 0.2; done
SIMULATEUR_PAUSE=900ms "$py" "$ici/capturer-console3.py" "http://127.0.0.1:$port" "$dossier" "$ici/reel"
