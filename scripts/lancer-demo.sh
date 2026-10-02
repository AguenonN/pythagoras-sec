#!/usr/bin/env bash
# Lanceur de la démonstration réelle : l'orchestrateur Mistral et ses
# sous-agents devant le gate, lui-même devant les huit serveurs du registre
# de démo. Une demande = un processus mcp-gate = une session MCP partagée
# par tous les sous-agents (docs/orchestration/contrats-demo.md §2, §3).
#
#	export MCP_GATE_CLE_JOURNAL=...      # hexadécimal, 32 octets ; jamais sur disque
#	scripts/lancer-demo.sh                                  # prépare tout
#	scripts/lancer-demo.sh demande "<texte>" --demandeur <id> [options]
#	scripts/lancer-demo.sh scenario <nom> [--mode reel|rejeu] [--enregistrement F] [--horloge-depart T]
#	scripts/lancer-demo.sh valider <empreinte> --valideur <id> --role <role>
#	scripts/lancer-demo.sh verifier <dossier de session>
#	scripts/lancer-demo.sh comparer <dossier de session> <dossier de session>
#	scripts/lancer-demo.sh gate ...     # interne : ce que l'orchestrateur lance
#
# Options de « demande » (et de « scenario », qui les lit dans le fichier) :
#	--horloge-depart T    horloge simulée du gate (RFC 3339), pour les répétitions ;
#	                      la date de la demande transmise aux sous-agents en découle.
#	                      Le 04/10 lui-même : horloge réelle (option absente).
#	--sans-directive D-00n  même jeu, sans cette directive (« sans la directive D-00n »)
#	--jeu F               autre jeu de travail (défaut : demo/directives/demo.json)
#	--registre normal|attaque
#	--mode reel|rejeu     reel (défaut) : Mistral ; rejeu : réponses enregistrées, sans
#	                      réseau (serveurs en DONNEES_MODE=REJEU, validations redéposées)
#	--enregistrement F    reel : où écrire les réponses ; rejeu : lesquelles rejouer
#	--scenario NOM        nom court du dossier de session et de l'enregistrement
#
# Préparation : construit les serveurs (scripts/construire-serveurs.sh),
# mcp-gate, mcp-approve, journal et directive dans bin/ ; génère les
# registres (demo/registre.json, demo/registre-attaque.json) avec les
# enregistrements des serveurs de données dans
# demo/scenarios/enregistrements/serveurs/ (versionnés : ce sont eux que
# rejoue le secours hors ligne) ; l'annuaire signé (mcp-approve
# annuaire-demo) si la source a changé ; le registre local des gels
# (gels-avoirs preparer) s'il est absent ; vérifie que le gate démarre avec
# le registre, l'annuaire et demo/directives/demo.json.
#
# gels-avoirs est servi en REJEU sur le registre local du jour (registre de
# démo, -mode=REJEU) : aucune attente réseau pendant la démo.
# Délai du backend : 120 s (DEMO_DELAI_BACKEND), garde-fou. En MIXTE, gels-avoirs
# télécharge le registre complet (12 Mo, environ 90 s le 01/10/2026) ; son
# client HTTP abandonne à 30 s, trois fois (1 s puis 2 s d'attente entre
# deux), avant de se replier sur le registre local daté : 95 s environ. Avec
# le délai par défaut du gate (30 s), le gate coupait avant ce repli :
# « source indisponible ».
#
# Le gate ne reçoit, par env -i, que la clé du journal, le jeton d'identité
# du demandeur, DONNEES_MODE et les identifiants PISTE : jamais
# MISTRAL_API_KEY (l'orchestrateur la retire aussi de l'environnement qu'il
# lui passe). Aucune clé ni aucun jeton n'est écrit sur disque par ce
# script, sauf les jetons de validation déposés par « valider » (§3).
set -euo pipefail

racine="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
execution="$racine/demo/execution"
sessions="$execution/sessions"
approbations="$execution/approbations"
cles="$execution/cles"
gels_local="$racine/servers/gels-avoirs/enregistrements_locaux"
enr_serveurs="demo/scenarios/enregistrements/serveurs"
jeu_demo="$racine/demo/directives/demo.json"

die() { echo "lancer-demo: $*" >&2; exit 1; }

usage() {
	sed -n '/^#\tscripts/p' "${BASH_SOURCE[0]}" | sed 's/^#\t/  /' >&2
	exit 2
}

exiger_cle() {
	if [[ -z "${MCP_GATE_CLE_JOURNAL:-}" ]]; then
		echo "lancer-demo: MCP_GATE_CLE_JOURNAL absente : pas de journal sans clé." >&2
		echo "  export MCP_GATE_CLE_JOURNAL=\$(openssl rand -hex 32)   # à garder pour relire les journaux" >&2
		exit 1
	fi
}

exiger_prepare() {
	for f in bin/mcp-gate bin/mcp-approve bin/journal demo/registre.json demo/annuaire.json "$cles/autorite.pub"; do
		[[ -e "$racine/$f" || -e "$f" ]] || die "$f absent ; lancer d'abord scripts/lancer-demo.sh"
	done
}

registre_de() {
	case "$1" in
		normal) echo "$racine/demo/registre.json" ;;
		attaque) echo "$racine/demo/registre-attaque.json" ;;
		*) die "--registre normal ou attaque, pas « $1 »" ;;
	esac
}

# ---------------------------------------------------------------------------
# gate : lancé par l'orchestrateur, en stdio. stdout est le canal MCP.
gate() {
	local session="" jeu="" registre="normal" depart=""
	while (( $# > 0 )); do
		case "$1" in
			--session) session="$2"; shift 2 ;;
			--jeu) jeu="$2"; shift 2 ;;
			--registre) registre="$2"; shift 2 ;;
			--horloge-depart) depart="$2"; shift 2 ;;
			*) die "gate : option inconnue $1" ;;
		esac
	done
	exiger_cle
	[[ -n "$session" && -n "$jeu" ]] || die "gate : --session et --jeu obligatoires"
	local env_gate=(MCP_GATE_CLE_JOURNAL="$MCP_GATE_CLE_JOURNAL" DONNEES_MODE="${DEMO_DONNEES_MODE:-MIXTE}")
	for v in MCP_GATE_JETON_DEMANDEUR PISTE_CLIENT_ID PISTE_CLIENT_SECRET; do
		if [[ -n "${!v:-}" ]]; then
			env_gate+=("$v=${!v}")
		fi
	done
	local args=(-registre "$(registre_de "$registre")" -directives "$jeu"
		-journal "$session/journal.jsonl" -coffre "$session/coffre.jsonl"
		-notifications "$session/notifications.jsonl"
		-annuaire "$racine/demo/annuaire.json" -annuaire-autorite "$cles/autorite.pub"
		-journaux-precedents "$sessions/*/journal.jsonl"
		-delai-backend "${DEMO_DELAI_BACKEND:-120s}")
	if [[ -n "$depart" ]]; then
		args+=(-horloge-depart "$depart")
	fi
	exec env -i "${env_gate[@]}" "$racine/bin/mcp-gate" "${args[@]}" 2>>"$session/mcp-gate.log"
}

# ---------------------------------------------------------------------------
preparer() {
	cd "$racine"
	scripts/construire-serveurs.sh
	for c in mcp-gate mcp-approve journal directive; do
		CGO_ENABLED=0 go build -trimpath -buildvcs=false -o "bin/$c" "./cmd/$c"
	done
	# Enregistrements des serveurs de données : versionnés dans
	# demo/scenarios, prolongés par le mode MIXTE des passages réels, rejoués
	# par le secours hors ligne.
	# eurlex et legifrance, dont aucun sous-agent n'a les outils : liens vers
	# les enregistrements des serveurs.
	for s in taux-officiel recherche-entreprises; do
		if [[ ! -d "$enr_serveurs/$s" ]]; then
			mkdir -p "$enr_serveurs"
			cp -R "servers/$s/enregistrements" "$enr_serveurs/$s"
		fi
	done
	for s in eurlex legifrance; do
		[[ -e "$enr_serveurs/$s" ]] || ln -s "../../../../servers/$s/enregistrements" "$enr_serveurs/$s"
	done
	go run ./cmd/registre-demo -enregistrements "$enr_serveurs"

	mkdir -p "$execution"
	if [[ ! -f demo/annuaire.json || demo/annuaire-source.json -nt demo/annuaire.json || ! -f "$cles/autorite.pub" ]]; then
		bin/mcp-approve annuaire-demo -source demo/annuaire-source.json -sortie demo/annuaire.json -cles "$cles"
	fi

	# Registre des gels : servi en REJEU sur ce registre local (registre de
	# démo : gels-avoirs -mode=REJEU), donc téléchargé le jour même. S'il
	# n'est pas du jour, nouveau téléchargement ; en cas d'échec, l'ancien
	# reste servi (sa date de publication figure dans chaque résultat).
	local recent=""
	if [[ -n "$(ls -A "$gels_local" 2>/dev/null)" ]]; then
		recent="$(date -r "$(ls -t "$gels_local"/* | head -1)" +%Y-%m-%d)"
	fi
	if [[ "$recent" != "$(date +%Y-%m-%d)" ]]; then
		echo "lancer-demo: registre local des gels ${recent:+du $recent }absent ou ancien, téléchargement (gels-avoirs preparer, jusqu'à 2 min)" >&2
		if ! bin/gels-avoirs preparer -enregistrements "$gels_local"; then
			[[ -n "$recent" ]] || die "registre des gels indisponible et aucun registre local"
			echo "lancer-demo: téléchargement impossible, registre local du $recent conservé" >&2
		fi
	else
		echo "lancer-demo: registre local des gels du jour présent ($gels_local)" >&2
	fi

	# Le gate démarre-t-il avec ce registre, cet annuaire et le jeu de la
	# démo ? En REJEU, journal jetable, stdin fermé : il s'arrête aussitôt.
	local essai
	essai="$(mktemp -d)"
	trap 'rm -rf "$essai"' RETURN
	for reg in normal attaque; do
		if ! env -i MCP_GATE_CLE_JOURNAL="$(printf '0%.0s' {1..64})" DONNEES_MODE=REJEU bin/mcp-gate \
			-registre "$(registre_de "$reg")" -directives "$jeu_demo" \
			-annuaire demo/annuaire.json -annuaire-autorite "$cles/autorite.pub" \
			-journal "$essai/$reg.jsonl" -coffre "$essai/$reg.coffre" </dev/null 2>"$essai/$reg.log"; then
			cat "$essai/$reg.log" >&2
			die "le gate ne démarre pas (registre $reg)"
		fi
		# Tous les serveurs du registre sont attestés, sauf taux-officiel
		# détourné dans le registre d'attaque.
		local attestes attendus total
		total="$(python3 -c 'import json,sys; print(len(json.load(open(sys.argv[1]))["serveurs"]))' "$(registre_de "$reg")")"
		attendus="$total"
		[[ "$reg" == attaque ]] && attendus=$((total - 1))
		attestes="$(grep -c 'attesté=true' "$essai/$reg.log" || true)"
		echo "lancer-demo: gate prêt (registre $reg, jeu demo), $attestes serveur(s) attesté(s) sur $total" >&2
		[[ "$attestes" == "$attendus" ]] || { grep 'msg=backend' "$essai/$reg.log" >&2 || true; exit 1; }
	done
	(cd agents && python3 -m orchestrateur sous-agents >/dev/null) || die "sous-agents invalides"
	cat <<EOF

Démonstration prête.
  export MCP_GATE_CLE_JOURNAL=...   # la même pour toutes les demandes : les journaux précédents sont relus
  scripts/lancer-demo.sh scenario nominal
  scripts/lancer-demo.sh demande "Peut-on accorder le découvert demandé dans le dossier DOS-FICTIF-002 ?" \\
      --demandeur camille.conseiller@exemple.test
Pendant une attente : scripts/lancer-demo.sh valider <empreinte> --valideur <id> --role <rôle>
EOF
}

# ---------------------------------------------------------------------------
demande() {
	local texte="" demandeur="" depart="" sans="" jeu="$jeu_demo" registre="normal" mode="reel" enr="" scenario=""
	(( $# > 0 )) || usage
	texte="$1"; shift
	while (( $# > 0 )); do
		case "$1" in
			--demandeur) demandeur="$2"; shift 2 ;;
			--horloge-depart) depart="$2"; shift 2 ;;
			--sans-directive) sans="$2"; shift 2 ;;
			--jeu) jeu="$(realpath -m "$2")"; shift 2 ;;
			--registre) registre="$2"; shift 2 ;;
			--mode) mode="$2"; shift 2 ;;
			--enregistrement) enr="$(realpath -m "$2")"; shift 2 ;;
			--scenario) scenario="$2"; shift 2 ;;
			*) die "demande : option inconnue $1" ;;
		esac
	done
	exiger_cle
	exiger_prepare
	[[ "$mode" == reel || "$mode" == rejeu ]] || die "--mode reel ou rejeu"
	[[ "$mode" == rejeu && -z "$enr" ]] && die "--mode rejeu exige --enregistrement"
	registre_de "$registre" >/dev/null
	[[ -f "$jeu" ]] || die "jeu introuvable : $jeu"
	if [[ "$mode" == rejeu ]]; then
		# Le demandeur et la demande sont ceux de l'enregistrement.
		demandeur="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["demandeur"])' "$enr")"
		texte="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["demande"])' "$enr")"
	fi
	[[ -n "$demandeur" ]] || die "--demandeur obligatoire"

	local court horodatage session date
	court="${scenario:-$(python3 -c 'import sys; sys.path.insert(0, sys.argv[2]); from orchestrateur.session import nom_court; print(nom_court(sys.argv[1]))' "$texte" "$racine/agents")}"
	[[ "$mode" == rejeu ]] && court="$court-rejeu"
	horodatage="$(date +%Y%m%d-%H%M%S)"
	session="$sessions/$horodatage-$court"
	mkdir -p "$session" "$approbations"
	while [[ -e "$session/etat.json" ]]; do sleep 1; horodatage="$(date +%Y%m%d-%H%M%S)"; session="$sessions/$horodatage-$court"; mkdir -p "$session"; done

	# Jeu du passage, gardé dans le dossier de session pour le rejeu du
	# journal. Sans directive retirée, le gate lit demo/directives/demo.json
	# lui-même (une directive activée vaut pour la demande suivante).
	local jeu_gate="$jeu"
	if [[ -n "$sans" ]]; then
		python3 - "$jeu" "$sans" "$session/jeu.json" <<'PY'
import json, sys
jeu, sans, sortie = sys.argv[1:]
d = json.load(open(jeu, encoding="utf-8"))
avant = len(d["directives"])
d["directives"] = [x for x in d["directives"] if x.get("id") != sans]
if len(d["directives"]) == avant:
    print("lancer-demo: %s absente du jeu : le jeu « sans » est identique" % sans, file=sys.stderr)
d["jeu"].pop("empreinte", None)
d["jeu"].pop("signature", None)
json.dump(d, open(sortie, "w", encoding="utf-8"), ensure_ascii=False, indent=1)
PY
		jeu_gate="$session/jeu.json"
	else
		cp "$jeu" "$session/jeu.json"
	fi

	if [[ -n "$depart" ]]; then
		date="$(TZ=Europe/Paris date -d "$depart" +%Y-%m-%d)"
	else
		date="$(TZ=Europe/Paris date +%Y-%m-%d)"
	fi
	local jeton
	jeton="$("$racine/bin/mcp-approve" identite -cles "$cles" -signataire "$demandeur")"
	local meta
	meta="$(python3 -c 'import json,sys; print(json.dumps({"sans_directive": sys.argv[1] or None, "jeu": sys.argv[2], "registre": sys.argv[3], "horloge_depart": sys.argv[4] or None}))' \
		"$sans" "${jeu#"$racine"/}" "$registre" "$depart")"
	local gate_cmd=("$racine/scripts/lancer-demo.sh" gate --session "$session" --jeu "$jeu_gate" --registre "$registre")
	[[ -n "$depart" ]] && gate_cmd+=(--horloge-depart "$depart")
	local orch=(python3 -m orchestrateur executer --session "$session" --approbations "$approbations"
		--mode "$mode" --meta "$meta" --demande "$texte" --demandeur "$demandeur" --date "$date")
	[[ -n "$enr" ]] && orch+=(--enregistrement "$enr")
	[[ -n "$scenario" ]] && orch+=(--scenario "$scenario")
	if [[ "$mode" == rejeu ]]; then
		orch+=(--mcp-approve "$racine/bin/mcp-approve" --cles "$cles")
		export DEMO_DONNEES_MODE="${DEMO_DONNEES_MODE:-REJEU}"
	fi
	echo "lancer-demo: session $session" >&2
	local code=0
	(cd "$racine/agents" && MCP_GATE_JETON_DEMANDEUR="$jeton" "${orch[@]}" -- "${gate_cmd[@]}") || code=$?
	echo "lancer-demo: état final : $(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["statut"])' "$session/etat.json")" >&2
	return "$code"
}

# ---------------------------------------------------------------------------
scenario() {
	(( $# > 0 )) || usage
	local nom="$1"; shift
	local fichier="$racine/demo/scenarios/$nom.json"
	[[ -f "$fichier" ]] || die "scénario introuvable : demo/scenarios/$nom.json"
	local mode="reel" enr="" depart=""
	while (( $# > 0 )); do
		case "$1" in
			--mode) mode="$2"; shift 2 ;;
			--enregistrement) enr="$2"; shift 2 ;;
			--horloge-depart) depart="$2"; shift 2 ;;
			*) die "scenario : option inconnue $1" ;;
		esac
	done
	local dossier_enr="$racine/demo/scenarios/enregistrements/$nom"
	if [[ -z "$enr" ]]; then
		if [[ "$mode" == rejeu ]]; then
			enr="$(ls "$dossier_enr"/*.json 2>/dev/null | sort | tail -1)"
			[[ -n "$enr" ]] || die "aucun enregistrement pour $nom dans demo/scenarios/enregistrements/$nom"
		else
			enr="$dossier_enr/$(date +%Y%m%d-%H%M%S).json"
		fi
	fi
	local args=()
	mapfile -t args < <(python3 - "$fichier" <<'PY'
import json, sys
s = json.load(open(sys.argv[1], encoding="utf-8"))
print(s["demande"])
print("--demandeur"); print(s["demandeur"])
for cle, opt in (("sans_directive", "--sans-directive"), ("jeu", "--jeu"), ("registre", "--registre")):
    if s.get(cle):
        print(opt); print(s[cle])
PY
)
	local opts=(--scenario "$nom" --mode "$mode" --enregistrement "$enr")
	[[ -n "$depart" ]] && opts+=(--horloge-depart "$depart")
	local jeu_rel
	for i in "${!args[@]}"; do
		if [[ "${args[$i]}" == --jeu ]]; then
			jeu_rel="${args[$((i + 1))]}"
			args[$((i + 1))]="$racine/$jeu_rel"
		fi
	done
	echo "lancer-demo: scénario $nom ($mode), enregistrement ${enr#"$racine"/}" >&2
	if [[ "$mode" == reel ]]; then
		python3 - "$fichier" <<'PY' >&2
import json, sys
s = json.load(open(sys.argv[1], encoding="utf-8"))
for i, v in enumerate(s.get("validations") or [], 1):
    qui = ", ".join("%s (%s)" % (x["identite"], x["role"]) for x in v["valideurs"])
    print("lancer-demo: validation %d attendue %s : %s" % (i, v.get("attendu", ""), qui))
    print("  scripts/lancer-demo.sh valider <empreinte> --valideur <id> --role <rôle>   (pour chacun)")
PY
	fi
	local code=0
	demande "${args[@]}" "${opts[@]}" || code=$?
	local derniere
	derniere="$(ls -d "$sessions"/*-"$nom"* 2>/dev/null | sort | tail -1)"
	python3 - "$fichier" "$derniere/reponse.json" <<'PY' >&2 || true
import json, sys
s = json.load(open(sys.argv[1], encoding="utf-8"))
a = s.get("attendu") or {}
r = json.load(open(sys.argv[2], encoding="utf-8"))
ecarts = []
if a.get("statut") and a["statut"] != r["statut"]:
    ecarts.append("statut %s, attendu %s" % (r["statut"], a["statut"]))
if a.get("directives") is not None and sorted(a["directives"]) != sorted(r["directives_citees"]):
    ecarts.append("directives %s, attendues %s" % (r["directives_citees"], a["directives"]))
print("lancer-demo: attendu : " + ("conforme" if not ecarts else "ÉCART : " + " ; ".join(ecarts)))
PY
	return "$code"
}

# ---------------------------------------------------------------------------
valider() {
	(( $# > 0 )) || usage
	local empreinte="$1"; shift
	local valideur="" role=""
	while (( $# > 0 )); do
		case "$1" in
			--valideur) valideur="$2"; shift 2 ;;
			--role) role="$2"; shift 2 ;;
			*) die "valider : option inconnue $1" ;;
		esac
	done
	[[ -n "$valideur" && -n "$role" ]] || die "valider : --valideur et --role obligatoires"
	local dossier="$approbations/$empreinte"
	mkdir -p "$dossier"
	local tmp="$dossier/.$valideur.tmp"
	"$racine/bin/mcp-approve" valider -cles "$cles" -valideur "$valideur" -role "$role" -empreinte "$empreinte" >"$tmp"
	mv "$tmp" "$dossier/$valideur.jwt"
	echo "lancer-demo: jeton de $valideur ($role) déposé pour $empreinte" >&2
}

verifier() {
	(( $# == 1 )) || usage
	exiger_cle
	local s="$1"
	"$racine/bin/journal" verifier "$s/journal.jsonl"
	"$racine/bin/journal" rejouer "$s/journal.jsonl" -directives "$s/jeu.json" -coffre "$s/coffre.jsonl" \
		-annuaire "$racine/demo/annuaire.json" -annuaire-autorite "$cles/autorite.pub"
}

comparer() {
	(( $# == 2 )) || usage
	local a b
	a="$(realpath -m "$1")"; b="$(realpath -m "$2")"
	(cd "$racine/agents" && python3 -m orchestrateur comparer "$a/journal.jsonl" "$b/journal.jsonl")
}

commande="${1:-preparer}"
(( $# > 0 )) && shift
case "$commande" in
	preparer) preparer ;;
	gate) gate "$@" ;;
	demande) demande "$@" ;;
	scenario) scenario "$@" ;;
	valider) valider "$@" ;;
	verifier) verifier "$@" ;;
	comparer) comparer "$@" ;;
	-h|--help|aide) usage ;;
	*) usage ;;
esac
