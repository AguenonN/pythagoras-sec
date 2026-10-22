#!/usr/bin/env bash
# Lanceur de la console pour la démo du hackathon : comme scripts/lancer-demo.sh,
# mais une demande tapée dans « Advisor desk » tourne sous le jeu de travail
# avec D-009 (demo/scenarios/jeux/avec-d009.json), en réel (Mistral).
#	bin/console -racine "$PWD" -lanceur scripts/lanceur-demo-d009.sh …
set -euo pipefail
racine="$(cd "$(dirname "$0")/.." && pwd)"
if [[ "${1:-}" == demande ]]; then
	shift
	exec "$racine/scripts/lancer-demo.sh" demande --jeu "$racine/demo/scenarios/jeux/avec-d009.json" "$@"
fi
exec "$racine/scripts/lancer-demo.sh" "$@"
