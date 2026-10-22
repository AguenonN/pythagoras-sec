#!/usr/bin/env bash
# Copie les passages de démonstration dans deploy/console-runtime/instantane/
# pour la console publique sur Aiven Runtime : sessions (journaux signés,
# données fictives), annuaire signé et clé PUBLIQUE de l'autorité. Jamais de
# clé privée (*.cle) ni de jeton d'approbation.
#	scripts/instantane-runtime.sh [motif de session …]   (défaut : toutes)
set -euo pipefail
cd "$(dirname "$0")/.."
dest=deploy/console-runtime/instantane
rm -rf "$dest/execution" "$dest/annuaire.json"
mkdir -p "$dest/execution/sessions" "$dest/execution/cles"
motifs=("$@"); [[ ${#motifs[@]} -gt 0 ]] || motifs=("")
n=0
for s in demo/execution/sessions/*/; do
	nom=$(basename "$s")
	for m in "${motifs[@]}"; do
		if [[ -z "$m" || "$nom" == *"$m"* ]]; then
			cp -r "$s" "$dest/execution/sessions/$nom"; n=$((n + 1)); break
		fi
	done
done
find "$dest" \( -name '*.cle' -o -name '*.jwt' -o -name '*.pem' -o -name '*.key' \) -delete
[[ -f demo/annuaire.json ]] && cp demo/annuaire.json "$dest/annuaire.json"
[[ -f demo/execution/cles/autorite.pub ]] && cp demo/execution/cles/autorite.pub "$dest/execution/cles/autorite.pub"
echo "instantane: $n session(s) copiée(s) dans $dest"
du -sh "$dest"
