#!/usr/bin/env bash
# Démo en direct, étape par étape (hackathon Accel AI Innovate, 22/10/2026).
#
#	scripts/demo-hackathon.sh verifier     # outils et variables présents (rien n'est affiché en clair)
#	scripts/demo-hackathon.sh preparer     # construit, prépare la démo, crée le schéma dans Aiven
#	scripts/demo-hackathon.sh passage      # scénario 4 avec D-009, hors ligne, puis versé dans Aiven
#	scripts/demo-hackathon.sh gels [SIREN] # « supposons que ce SIREN soit ajouté au registre des gels »
#	scripts/demo-hackathon.sh regle        # « et si cette règle avait existé ? » (règle prête)
#	scripts/demo-hackathon.sh regle-live "<texte de la règle>"   # compilée par Mistral, puis rejouée
#	scripts/demo-hackathon.sh detective [PORT]   # page Detective sur la base Aiven (défaut 8090)
#	scripts/demo-hackathon.sh sync         # verse en continu chaque session nouvelle ou modifiée dans Aiven
#	scripts/demo-hackathon.sh tout         # preparer, passage, gels, regle
#
# Variables : AIVEN_PG_URI, MCP_GATE_CLE_JOURNAL (la même toute la journée),
# MISTRAL_API_KEY pour regle-live. Aucune n'est affichée.
set -euo pipefail

racine="$(cd "$(dirname "$0")/.." && pwd)"
cd "$racine"

JEU=demo/scenarios/jeux/avec-d009.json
REGLE=demo/scenarios/jeux/hypothese-seuil-officiel.json
SIREN_DEMO=508858230   # DOS-FICTIF-003 : entreprise réelle de l'annuaire public, gel HYPOTHÉTIQUE

titre() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
dire() { printf '   %s\n' "$*"; }
echec() { printf '\033[31mdemo: %s\033[0m\n' "$*" >&2; exit 1; }

exiger() {
	for v in "$@"; do
		[[ -n "${!v:-}" ]] || echec "$v absente : export $v=…"
	done
}

derniere_session() {
	local s
	s=$(ls -td demo/execution/sessions/*-base-interne-avec-d009-* 2>/dev/null | head -1)
	[[ -n "$s" && -f "$s/journal.jsonl" ]] || echec "aucune session « avec D-009 » : lancer d'abord « $0 passage »"
	printf '%s' "$s"
}

psql_aiven() { psql -q -v ON_ERROR_STOP=1 "$AIVEN_PG_URI"; }

verifier() {
	titre "Vérification"
	local ok=1
	for outil in go psql python3 openssl; do
		if command -v "$outil" >/dev/null; then dire "outil $outil : présent"; else dire "outil $outil : ABSENT"; ok=0; fi
	done
	for v in AIVEN_PG_URI MCP_GATE_CLE_JOURNAL MISTRAL_API_KEY; do
		if [[ -n "${!v:-}" ]]; then dire "$v : définie"; else dire "$v : non définie"; fi
	done
	if [[ -n "${AIVEN_PG_URI:-}" ]]; then
		if psql "$AIVEN_PG_URI" -Atc "select 1" >/dev/null 2>&1; then dire "Aiven PostgreSQL : joignable"; else dire "Aiven PostgreSQL : INJOIGNABLE (allowlist d'IP ?)"; ok=0; fi
	fi
	[[ $ok == 1 ]] || echec "à corriger avant la démo"
}

construire() {
	go build -o bin/journal-sql ./cmd/journal-sql
}

preparer() {
	exiger AIVEN_PG_URI MCP_GATE_CLE_JOURNAL
	titre "Construction et préparation"
	construire
	scripts/lancer-demo.sh
	titre "Schéma dans Aiven PostgreSQL"
	bin/journal-sql schema | psql_aiven
	dire "tables entrees, consultations, applications, impacts ; vue detective"
}

passage() {
	exiger AIVEN_PG_URI MCP_GATE_CLE_JOURNAL
	construire
	titre "L'agent crédit décide (scénario 4, rejeu hors ligne, D-009 active)"
	scripts/lancer-demo.sh scenario base-interne-avec-d009 --mode rejeu
	local s; s=$(derniere_session)
	titre "Vérification de la chaîne HMAC, puis versement dans Aiven"
	bin/journal-sql exporter "$s/journal.jsonl" | psql_aiven
	dire "session : $s"
}

gels() {
	exiger AIVEN_PG_URI MCP_GATE_CLE_JOURNAL
	construire
	local siren="${1:-$SIREN_DEMO}" s; s=$(derniere_session)
	titre "Hypothèse : le registre des gels ajoute le SIREN $siren"
	dire "(exercice : aucune entreprise réelle n'est présentée comme gelée)"
	bin/journal-sql impacts "$s/journal.jsonl" -directives "$JEU" -gels "$siren" \
		-evenement "gels (hypothèse) $siren" | psql_aiven
}

regle() {
	exiger AIVEN_PG_URI MCP_GATE_CLE_JOURNAL
	construire
	local s; s=$(derniere_session)
	titre "Et si cette règle avait existé ?"
	python3 -c 'import json,sys; print("   « " + json.load(open(sys.argv[1]))["texte_fr"] + " »")' "$REGLE"
	bin/journal-sql impacts "$s/journal.jsonl" -directives "$JEU" -apres "$REGLE" | psql_aiven
}

regle_live() {
	exiger AIVEN_PG_URI MCP_GATE_CLE_JOURNAL MISTRAL_API_KEY
	local texte="${1:-}"
	[[ -n "$texte" ]] || echec "usage : $0 regle-live \"<texte de la règle>\""
	construire
	local s; s=$(derniere_session)
	local sortie="$racine/demo/execution/regle-live.json" reponses="$racine/demo/execution/reponses-live.json"
	[[ -f "$reponses" ]] || printf '%s\n' '[{"question": "si_indetermine", "champ": "si_indetermine", "options": ["refuser", "ignorer"], "reponse": "refuser", "origine": "compilateur"}]' > "$reponses"
	rm -f "$sortie"
	titre "Compilation de la règle (Mistral propose, le vérificateur contrôle)"
	( cd agents
	  [[ -d .venv ]] || python3 -m venv .venv
	  .venv/bin/python -m compilateur --mode reel --texte "$texte" \
		--fondement "${FONDEMENT:-Code monétaire et financier, article L313-5-1 — à valider par le juriste}" \
		--jeu demo --jeu-existant "../$JEU" --catalogue compilateur/exemples/catalogue-demo.json \
		--auteur juriste@exemple.test --date-effet "$(date +%F)" \
		--reponses "$reponses" --sortie "$sortie" )
	[[ -f "$sortie" ]] || echec "pas de règle écrite : le compilateur a posé une question ou refusé (voir ci-dessus)"
	titre "Rejeu de tout l'historique avec cette règle"
	bin/journal-sql impacts "$s/journal.jsonl" -directives "$JEU" -apres "$sortie" -evenement "règle : $texte" | psql_aiven
}

detective() {
	exiger AIVEN_PG_URI
	local port="${1:-8090}" venv=apps/detective/.venv
	[[ -x $venv/bin/python ]] || { python3 -m venv "$venv" && "$venv/bin/pip" install -q -r apps/detective/requirements.txt; }
	titre "Page Detective : http://127.0.0.1:$port"
	DATABASE_URL="$AIVEN_PG_URI" PORT="$port" exec "$venv/bin/python" apps/detective/app.py
}

sync() {
	exiger AIVEN_PG_URI MCP_GATE_CLE_JOURNAL
	construire
	declare -A vu=()
	titre "Synchronisation continue des journaux vers Aiven PostgreSQL (Ctrl+C pour arrêter)"
	while :; do
		for j in demo/execution/sessions/*/journal.jsonl; do
			[[ -f "$j" ]] || continue
			local marque; marque=$(stat -c '%s:%Y' "$j")
			[[ "${vu[$j]:-}" == "$marque" ]] && continue
			if bin/journal-sql exporter "$j" 2>/dev/null | psql_aiven 2>/dev/null; then
				vu[$j]=$marque
				dire "$(date +%T)  versé : $(basename "$(dirname "$j")")  ($(wc -l < "$j") entrées, chaîne vérifiée)"
			else
				dire "$(date +%T)  non versé (chaîne en cours d'écriture ou rompue) : $(basename "$(dirname "$j")")"
			fi
		done
		sleep 5
	done
}

case "${1:-}" in
	verifier) verifier ;;
	preparer) preparer ;;
	passage) passage ;;
	gels) shift; gels "${1:-}" ;;
	regle) regle ;;
	regle-live) shift; regle_live "${1:-}" ;;
	detective) shift; detective "${1:-}" ;;
	sync) sync ;;
	tout) preparer; passage; gels; regle ;;
	*) sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
