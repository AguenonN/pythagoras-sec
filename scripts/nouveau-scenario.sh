#!/usr/bin/env bash
# Crée un scénario de démonstration en une commande.
#
#	scripts/nouveau-scenario.sh <nom> [options]
#
#	<nom>                        minuscules, chiffres, tirets
#	--demande "texte"            la demande faite à l'orchestrateur
#	--demandeur <identité>       une identité de demo/annuaire-source.json
#	                             (défaut : camille.conseiller@exemple.test)
#	--sous-agents a,b            sous-agents que le scénario doit faire intervenir
#	--decision serveur/outil=effet   décision attendue de l'engine pour cet outil :
#	                             autoriser, refuser, caviarder ou attendre_validation
#	                             (option répétable)
#	--statut S                   termine (défaut), bloque, en_attente ou erreur
#	--directives D-001,D-002     directives que la réponse doit citer
#	                             (absent : non vérifié)
#
# Crée :
#	demo/scenarios/<nom>.json              joué par scripts/lancer-demo.sh scenario <nom>
#	test/extension/scenario_<nom>_test.go  le rejoue hors ligne une fois enregistré
#	                                       (ignoré tant qu'aucun enregistrement n'existe)
#
# Aucun appel réseau, aucun appel Mistral : l'enregistrement se fait plus tard,
# par scripts/lancer-demo.sh scenario <nom> (docs/demo/etendre.md, recette 4).
set -euo pipefail

racine="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$racine"
nom_valide='^[a-z0-9]([a-z0-9-]*[a-z0-9])?$'

die() { echo "nouveau-scenario: $*" >&2; exit 1; }
usage() {
	sed -n '/^#\tscripts/,/^# Crée/p' "${BASH_SOURCE[0]}" | sed '$d; s/^#\t\?/  /' >&2
	exit 2
}

(( $# > 0 )) || usage
[[ "$1" != -* ]] || usage
nom="$1"; shift
[[ "$nom" =~ $nom_valide ]] && (( ${#nom} <= 64 )) || die "nom « $nom » non conforme : minuscules, chiffres, tirets"
demande="" demandeur="camille.conseiller@exemple.test" sous_agents="" statut="termine" directives=""
decisions=()
while (( $# > 0 )); do
	(( $# >= 2 )) || die "option $1 sans valeur"
	case "$1" in
		--demande) demande="$2" ;;
		--demandeur) demandeur="$2" ;;
		--sous-agents) sous_agents="$2" ;;
		--decision) decisions+=("$2") ;;
		--statut) statut="$2" ;;
		--directives) directives="$2" ;;
		*) die "option inconnue $1" ;;
	esac
	shift 2
done

fichier="demo/scenarios/$nom.json"
test_go="test/extension/scenario_${nom//-/_}_test.go"
[[ ! -e "$fichier" ]] || die "$fichier existe déjà"
[[ ! -e "$test_go" ]] || die "$test_go existe déjà"
[[ -z "$demande" ]] && demande="À REMPLACER : la demande que le conseiller fait à l'agent."

python3 - "$fichier" "$nom" "$demande" "$demandeur" "$sous_agents" "$statut" "$directives" "${decisions[@]}" <<'PY'
import json, os, re, sys

fichier, nom, demande, demandeur, sous_agents, statut, directives, *decisions = sys.argv[1:]

def echec(msg):
    print("nouveau-scenario: " + msg, file=sys.stderr)
    sys.exit(1)

annuaire = json.load(open("demo/annuaire-source.json", encoding="utf-8"))
identites = [i["identite"] for i in annuaire["identites"]]
if demandeur not in identites:
    echec("demandeur %r inconnu de demo/annuaire-source.json (identités : %s)" % (demandeur, ", ".join(identites)))
if statut not in ("termine", "bloque", "en_attente", "erreur"):
    echec("statut %r : termine, bloque, en_attente ou erreur" % statut)

agents = [a for a in sous_agents.split(",") if a]
existants = {f[:-5] for f in os.listdir("agents/orchestrateur/sous_agents") if f.endswith(".json")}
for a in agents:
    if a not in existants:
        echec("sous-agent %r absent de agents/orchestrateur/sous_agents/ (existants : %s)" % (a, ", ".join(sorted(existants))))

effets = ("autoriser", "refuser", "caviarder", "attendre_validation")
attendues = {}
for d in decisions:
    outil, sep, effet = d.partition("=")
    if not sep or not re.fullmatch(r"[a-z0-9-]+/[a-z0-9_]+", outil) or effet not in effets:
        echec("--decision %r : forme serveur/outil=effet, effet parmi %s" % (d, ", ".join(effets)))
    attendues[outil] = effet

liste = [d for d in directives.split(",") if d]
for d in liste:
    if not re.fullmatch(r"D-\d{3}", d):
        echec("directive %r : forme D-009" % d)

scenario = {
    "titre": nom,
    "demande": demande,
    "demandeur": demandeur,
    "validations": [],
    "attendu": {
        "statut": statut,
        "directives": liste if directives else None,
        "sous_agents": agents,
        "decisions": attendues,
        "description": "À REMPLACER : ce que le scénario montre, en une phrase.",
    },
}
os.makedirs(os.path.dirname(fichier), exist_ok=True)
with open(fichier, "w", encoding="utf-8") as f:
    json.dump(scenario, f, ensure_ascii=False, indent=1)
    f.write("\n")
PY

cat >"$test_go" <<EOF
package extension

import "testing"

// Rejoue hors ligne le scénario $nom une fois enregistré
// (scripts/lancer-demo.sh scenario $nom) : sans réseau ni Mistral, devant le
// gate ; vérifie la réponse, les directives citées, les sous-agents et les
// décisions attendus par demo/scenarios/$nom.json, puis la chaîne du journal.
// Ignoré tant qu'aucun enregistrement n'existe.
func TestScenario_${nom//-/_}(t *testing.T) {
	RejouerScenario(t, "$nom")
}
EOF
gofmt -l test/extension >/dev/null || true
echo "nouveau-scenario: $fichier et $test_go créés." >&2
echo "  Compléter « titre », « demande » et « description » dans $fichier, puis l'enregistrer :" >&2
echo "    scripts/lancer-demo.sh scenario $nom" >&2
echo "  Rejeu hors ligne : scripts/lancer-demo.sh scenario $nom --mode rejeu ; go test ./test/extension -run Scenario_${nom//-/_}" >&2
