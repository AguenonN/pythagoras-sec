#!/usr/bin/env bash
# Construit les serveurs MCP de données de façon reproductible, vérifie que
# chaque schema.json est la sortie brute de tools/list du binaire, et écrit
# bin/manifeste.json (nom logique, binaire, SHA-256, schéma, SHA-256 du
# schéma). Deux exécutions successives produisent le même manifeste.
#
#	scripts/construire-serveurs.sh
#
# Construction : -trimpath -buildvcs=false, CGO_ENABLED=0 (binaires
# statiques, sans dépendre de la chaîne C de la machine), -mod=readonly
# (go.mod n'est jamais modifié). Aucun réseau : tools/list est lu en REJEU.
# bin/ est ignoré par Git.
set -euo pipefail

racine="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$racine"

# Ordre fixe : c'est l'ordre du manifeste.
serveurs=(
	taux-officiel
	taux-imposteur
	recherche-entreprises
	gels-avoirs
	eurlex
	legifrance
	banque-synthetique
	rh-synthetique
	referentiel-rh
	data-room
	messagerie
	bourse
	communication
)

export CGO_ENABLED=0
export GOFLAGS=-mod=readonly
drapeaux=(-trimpath -buildvcs=false)

mkdir -p bin
travail="$(mktemp -d)"
trap 'rm -rf "$travail"' EXIT

# Outil d'export des schémas (hors manifeste).
go build "${drapeaux[@]}" -o "$travail/exporter-schema" ./servers/exporter-schema

nom_valide='^[a-z0-9]([a-z0-9-]*[a-z0-9])?$' # internal/nommage.ValiderNomLogique
entrees=()
for s in "${serveurs[@]}"; do
	if ! [[ "$s" =~ $nom_valide ]] || [[ "$s" == *__* ]]; then
		echo "construire-serveurs: nom logique invalide : $s" >&2
		exit 1
	fi
	binaire="bin/$s"
	schema="servers/$s/schema.json"
	go build "${drapeaux[@]}" -o "$binaire" "./servers/$s"

	# tools/list brut du binaire, lu sur stdio en REJEU, comparé octet pour
	# octet au schéma versionné.
	DONNEES_MODE=REJEU "$travail/exporter-schema" -o "$travail/$s.json" "$binaire"
	if ! cmp -s "$travail/$s.json" "$schema"; then
		echo "construire-serveurs: $schema diffère de tools/list de $binaire" >&2
		echo "  régénérer : go run ./servers/exporter-schema -o $schema $binaire" >&2
		exit 1
	fi

	sha_bin="$(sha256sum "$binaire" | cut -d' ' -f1)"
	sha_schema="$(sha256sum "$schema" | cut -d' ' -f1)"
	entrees+=("$(printf '    {"nom": "%s", "binaire": "%s", "sha256": "%s", "schema": "%s", "schema_sha256": "%s"}' \
		"$s" "$binaire" "$sha_bin" "$schema" "$sha_schema")")
	echo "$s  $sha_bin" >&2
done

{
	printf '{\n'
	printf '  "construction": {"go": "%s", "goos": "%s", "goarch": "%s", "cgo": "0", "drapeaux": "%s"},\n' \
		"$(go env GOVERSION)" "$(go env GOOS)" "$(go env GOARCH)" "${drapeaux[*]}"
	printf '  "serveurs": [\n'
	for i in "${!entrees[@]}"; do
		if (( i + 1 < ${#entrees[@]} )); then
			printf '%s,\n' "${entrees[$i]}"
		else
			printf '%s\n' "${entrees[$i]}"
		fi
	done
	printf '  ]\n}\n'
} >"$travail/manifeste.json"
mv "$travail/manifeste.json" bin/manifeste.json
echo "construire-serveurs: bin/manifeste.json ($(sha256sum bin/manifeste.json | cut -d' ' -f1))" >&2
