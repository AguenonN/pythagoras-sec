#!/usr/bin/env bash
# Crée un serveur MCP synthétique en une commande, sans écrire de Go.
#
#	scripts/nouveau-serveur.sh <nom> <donnees.json> <outils.json> [<etiquettes.json>]
#	scripts/nouveau-serveur.sh --regenerer <nom>     # après avoir modifié outils.json
#
#	<nom>              nom logique : minuscules, chiffres, tirets (internal/nommage)
#	<donnees.json>     données FICTIVES (collections d'enregistrements)
#	<outils.json>      outils : nom, description, genre, arguments, readOnlyHint
#	<etiquettes.json>  facultatif : étiquettes du serveur et de ses outils
#
# Formats et exemples : servers/modele-synthetique/domaines/ et
# docs/demo/etendre.md. Le serveur n'a aucun code propre : il embarque les
# deux JSON et s'appuie sur servers/modele-synthetique (moteur commun) et
# servers/internal/lanceur (stdio et HTTP TLS).
#
# Le script : valide les fichiers (rien n'est créé s'ils sont invalides),
# crée servers/<nom>/ (main.go, donnees.json, outils.json, serveur_test.go,
# schema.json), ajoute <nom> à scripts/construire-serveurs.sh et à
# demo/serveurs-supplementaires.json (avec ses étiquettes : demo/etiquettes.json
# reste celui de la démo figée), reconstruit les serveurs (bin/manifeste.json),
# régénère le registre de démo, puis le catalogue et le vocabulaire étendus du
# compilateur dans demo/execution/extension/, et lance le test du serveur.
# Aucun réseau.
set -euo pipefail

racine="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$racine"
mod="servers/modele-synthetique"
nom_valide='^[a-z0-9]([a-z0-9-]*[a-z0-9])?$' # internal/nommage.ValiderNomLogique

die() { echo "nouveau-serveur: $*" >&2; exit 1; }
usage() {
	echo "usage : scripts/nouveau-serveur.sh <nom> <donnees.json> <outils.json> [<etiquettes.json>]" >&2
	echo "        scripts/nouveau-serveur.sh --regenerer <nom>" >&2
	exit 2
}
nom_ok() { [[ "$1" =~ $nom_valide ]] && [[ "$1" != *__* ]] && (( ${#1} <= 64 )); }

# Reconstruit tout ce qui dérive des outils : schéma, manifeste, registre,
# catalogue et vocabulaire du compilateur.
regenerer() {
	local nom="$1" travail
	travail="$(mktemp -d)"
	trap 'rm -rf "$travail"' RETURN
	CGO_ENABLED=0 go build -trimpath -buildvcs=false -o "$travail/$nom" "./servers/$nom"
	go build -o "$travail/exporter-schema" ./servers/exporter-schema
	DONNEES_MODE=REJEU "$travail/exporter-schema" -o "servers/$nom/schema.json" "$travail/$nom"
	scripts/construire-serveurs.sh
	# Catalogue et vocabulaire étendus du compilateur : fichiers locaux, hors
	# Git. Ceux de agents/compilateur/exemples/ restent ceux de la démo figée.
	go run ./cmd/registre-demo -catalogue demo/execution/extension/catalogue.json
	go run ./cmd/registre-demo -vocabulaire demo/execution/extension/vocabulaire.json -etendu
	go test "./servers/$nom/"
}

if [[ "${1:-}" == "--regenerer" ]]; then
	[[ $# -eq 2 ]] || usage
	{ nom_ok "$2" && [[ -d "servers/$2" ]]; } || die "servers/$2 n'existe pas"
	go run "./$mod/valider" "servers/$2/outils.json" "servers/$2/donnees.json" || die "fichiers invalides"
	regenerer "$2"
	echo "nouveau-serveur: servers/$2 régénéré." >&2
	exit 0
fi

[[ $# -eq 3 || $# -eq 4 ]] || usage
nom="$1"; donnees="$2"; outils="$3"; etiquettes="${4:-}"
nom_ok "$nom" || die "nom « $nom » non conforme : minuscules, chiffres, tirets ; ni tiret au début ou à la fin, ni « __ »"
[[ "$nom" != "modele-synthetique" ]] || die "nom réservé"
[[ ! -e "servers/$nom" ]] || die "servers/$nom existe déjà"
[[ -f "$donnees" ]] || die "fichier de données introuvable : $donnees"
[[ -f "$outils" ]] || die "fichier d'outils introuvable : $outils"
[[ -z "$etiquettes" || -f "$etiquettes" ]] || die "fichier d'étiquettes introuvable : $etiquettes"

# Tout est validé avant d'écrire quoi que ce soit.
go run "./$mod/valider" "$outils" "$donnees" || die "fichiers invalides, rien n'a été créé"
if [[ -n "$etiquettes" ]]; then
	python3 "$mod/outillage.py" controle "$outils" "$etiquettes" || die "étiquettes invalides, rien n'a été créé"
fi

dir="servers/$nom"
mkdir -p "$dir"
cp "$donnees" "$dir/donnees.json"
cp "$outils" "$dir/outils.json"
sed "s/@NOM@/$nom/g" "$mod/gabarit/main.go.gabarit" >"$dir/main.go"
sed "s/@NOM@/$nom/g" "$mod/gabarit/serveur_test.go.gabarit" >"$dir/serveur_test.go"
echo '[]' >"$dir/schema.json" # provisoire, régénéré plus bas

if [[ -n "$etiquettes" ]]; then
	python3 "$mod/outillage.py" etiquettes "$racine" "$nom" "$outils" "$etiquettes"
fi
python3 "$mod/outillage.py" liste "$racine" "$nom"
python3 "$mod/outillage.py" supplement "$racine" "$nom"

regenerer "$nom"
premier="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["outils"][0]["nom"])' "$outils")"
echo "nouveau-serveur: servers/$nom créé (main.go, donnees.json, outils.json, serveur_test.go, schema.json)." >&2
echo "  Ses outils s'appellent ${nom}__<outil>, par exemple ${nom}__${premier}." >&2
echo "  Étape suivante : scripts/lancer-demo.sh (le gate doit compter ce serveur parmi les attestés)." >&2
echo "  Pour que le compilateur (et la console) connaissent ce serveur, avant de lancer la console :" >&2
echo "    export COMPILATEUR_CATALOGUE=$racine/demo/execution/extension/catalogue.json" >&2
echo "    export COMPILATEUR_VOCABULAIRE=$racine/demo/execution/extension/vocabulaire.json" >&2
