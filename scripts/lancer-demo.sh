#!/usr/bin/env bash
# Lanceur de la démonstration : l'agent Mistral devant le gate, lui-même
# devant les huit serveurs du registre de démo.
#
#	export MCP_GATE_CLE_JOURNAL=$(openssl rand -hex 32)   # à garder pour relire le journal
#	scripts/lancer-demo.sh            # prépare, puis affiche comment lancer la boucle
#	scripts/lancer-demo.sh gate       # démarre mcp-gate sur stdio (c'est la boucle qui le lance)
#
# Préparation : construit les serveurs (scripts/construire-serveurs.sh) et
# mcp-gate dans bin/, génère demo/registre.json, vérifie que le registre
# local des gels est présent (sinon lance « gels-avoirs preparer », qui
# télécharge le registre national des gels auprès de la DG Trésor), et
# vérifie que le gate démarre avec le registre et le jeu de démo.
#
# « gate » exécute bin/mcp-gate avec les serveurs en DONNEES_MODE=MIXTE
# (API officielle, enregistrements à défaut), dans un environnement réduit
# à ce que le gate et les serveurs doivent recevoir : la clé du journal,
# DONNEES_MODE et les identifiants PISTE s'ils sont définis. MISTRAL_API_KEY
# et le reste de l'environnement de l'agent ne passent pas. Journal et
# coffre : demo/execution/ (ignoré par Git), prolongés d'une exécution à
# l'autre : garder la même clé, ou les retirer avant de changer de clé, sans
# quoi la chaîne ne se vérifie plus ; traces du gate et des serveurs :
# demo/execution/mcp-gate.log.
#
# En MIXTE, un serveur de données écrit chaque réponse obtenue dans son
# répertoire d'enregistrements. Pour que la démo ne réécrive jamais les
# enregistrements versionnés (ceux que rejouent les tests), le registre de
# démo pointe vers une copie de travail, demo/execution/enregistrements/,
# faite une fois à la préparation. De même, la boucle affichée garde ses
# réponses Mistral dans demo/execution/mistral/, pas dans les
# enregistrements des tests.
#
# La clé du journal n'est lue que dans l'environnement ; ce script n'en
# crée ni n'en écrit jamais.
set -euo pipefail

racine="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
registre="$racine/demo/registre.json"
directives="$racine/demo/directives/credit-demo.json"
execution="$racine/demo/execution"
gels_local="$racine/servers/gels-avoirs/enregistrements_locaux"
demande="Peut-on accorder le découvert demandé dans le dossier DOS-FICTIF-002 ?"

exiger_cle() {
	if [[ -z "${MCP_GATE_CLE_JOURNAL:-}" ]]; then
		echo "lancer-demo: MCP_GATE_CLE_JOURNAL absente : pas de journal sans clé." >&2
		echo "  export MCP_GATE_CLE_JOURNAL=\$(openssl rand -hex 32)   # à garder pour relire le journal" >&2
		exit 1
	fi
}

# gate : ce que la boucle lance comme serveur MCP. stdout est le canal MCP.
gate() {
	exiger_cle
	for f in "$racine/bin/mcp-gate" "$registre"; do
		if [[ ! -e "$f" ]]; then
			echo "lancer-demo: $f absent ; lancer d'abord scripts/lancer-demo.sh" >&2
			exit 1
		fi
	done
	mkdir -p "$execution"
	env_gate=(MCP_GATE_CLE_JOURNAL="$MCP_GATE_CLE_JOURNAL" DONNEES_MODE=MIXTE)
	for v in PISTE_CLIENT_ID PISTE_CLIENT_SECRET; do
		if [[ -n "${!v:-}" ]]; then
			env_gate+=("$v=${!v}")
		fi
	done
	exec env -i "${env_gate[@]}" "$racine/bin/mcp-gate" \
		-registre "$registre" -directives "$directives" \
		-journal "$execution/journal.jsonl" -coffre "$execution/coffre.jsonl" \
		2>>"$execution/mcp-gate.log"
}

preparer() {
	cd "$racine"
	scripts/construire-serveurs.sh
	CGO_ENABLED=0 go build -trimpath -buildvcs=false -o bin/mcp-gate ./cmd/mcp-gate
	# Copie de travail des enregistrements des serveurs de données, gardée
	# d'une préparation à l'autre (elle accumule les réponses du MIXTE).
	for s in taux-officiel recherche-entreprises eurlex legifrance; do
		if [[ ! -d "$execution/enregistrements/$s" ]]; then
			mkdir -p "$execution/enregistrements"
			cp -R "servers/$s/enregistrements" "$execution/enregistrements/$s"
		fi
	done
	go run ./cmd/registre-demo -enregistrements demo/execution/enregistrements

	if [[ -z "$(ls -A "$gels_local" 2>/dev/null)" ]]; then
		echo "lancer-demo: registre local des gels absent, téléchargement (gels-avoirs preparer)" >&2
		bin/gels-avoirs preparer -enregistrements "$gels_local"
	else
		echo "lancer-demo: registre local des gels présent ($gels_local)" >&2
	fi

	# Le gate démarre-t-il avec ce registre et ce jeu ? En REJEU, sans
	# réseau, journal jetable : stdin fermé, il s'arrête aussitôt.
	essai="$(mktemp -d)"
	trap 'rm -rf "$essai"' EXIT
	if ! env -i MCP_GATE_CLE_JOURNAL="$(printf '0%.0s' {1..64})" DONNEES_MODE=REJEU bin/mcp-gate \
		-registre "$registre" -directives "$directives" \
		-journal "$essai/journal.jsonl" -coffre "$essai/coffre.jsonl" </dev/null 2>"$essai/log"; then
		cat "$essai/log" >&2
		echo "lancer-demo: le gate ne démarre pas" >&2
		exit 1
	fi
	attestes="$(grep -c 'attesté=true' "$essai/log" || true)"
	echo "lancer-demo: gate prêt, $attestes serveur(s) attesté(s) sur 8" >&2
	if [[ "$attestes" != 8 ]]; then
		grep 'msg=backend' "$essai/log" >&2 || true
		exit 1
	fi

	cat <<EOF

Démonstration prête. Lancer la boucle (Mistral, appels réels comptés dans agents/.etat/boucle) :

  export MCP_GATE_CLE_JOURNAL=\$(openssl rand -hex 32)   # si ce n'est déjà fait
  cd agents
  python3 -m boucle --sous-agent credit --mode reel --enregistrements ../demo/execution/mistral \\
    --demande "$demande" \\
    -- ../scripts/lancer-demo.sh gate

Journal : demo/execution/journal.jsonl ; traces : demo/execution/mcp-gate.log
EOF
}

case "${1:-}" in
	"") preparer ;;
	gate) gate ;;
	*)
		echo "usage : scripts/lancer-demo.sh [gate]" >&2
		exit 2
		;;
esac
