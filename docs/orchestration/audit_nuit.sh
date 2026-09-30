#!/usr/bin/env bash
# Audit des trois branches de nuit — lecture seule, ne modifie rien.
# Usage : bash audit_nuit.sh 2>&1 | tee ~/audit_nuit.txt
set -u
BASE=69e32ee                      # commit de départ commun des trois worktrees
RACINE=~/cybercar
PROTEGES="internal/directive internal/journal internal/session internal/rejeu internal/chaine internal/nommage internal/proxy internal/toolpolicy internal/approval cmd/mcp-proxy"

# Secrets chargés uniquement pour les rechercher, jamais affichés
[ -f ~/.config/hacklaw/mistral.env ] && source ~/.config/hacklaw/mistral.env
[ -f ~/.config/hacklaw/piste.env ]   && source ~/.config/hacklaw/piste.env

for w in gate donnees agents; do
  d=$RACINE/mcp_rogue-$w
  echo
  echo "######################## $w ########################"

  echo "--- 1. Commits depuis $BASE"
  git -C "$d" log --oneline "$BASE"..HEAD

  echo "--- 2. Taille du travail"
  git -C "$d" diff --shortstat "$BASE"..HEAD

  echo "--- 3. Fichiers MODIFIÉS ou SUPPRIMÉS (attendu : aucun, sauf go.mod / go.sum)"
  git -C "$d" diff --name-status "$BASE"..HEAD | grep -v '^A' || echo "(aucun : que des ajouts)"

  echo "--- 4. Paquets protégés touchés (attendu : aucun)"
  git -C "$d" diff --name-only "$BASE"..HEAD -- $PROTEGES || true

  echo "--- 5. Fichiers suspects commités (attendu : aucun)"
  git -C "$d" diff --name-only "$BASE"..HEAD | grep -Ei '\.env$|\.key$|\.pem$|secret|__pycache__|\.pyc$|/bin/' || echo "(aucun)"

  echo "--- 6. Fichiers de plus de 500 Ko"
  git -C "$d" diff --name-only "$BASE"..HEAD | while read -r f; do
    [ -f "$d/$f" ] && s=$(stat -c%s "$d/$f") && [ "$s" -gt 512000 ] && echo "$s  $f"
  done
  true

  echo "--- 7. Valeurs réelles des secrets présentes dans le worktree (attendu : aucune)"
  for v in MISTRAL_API_KEY PISTE_CLIENT_ID PISTE_CLIENT_SECRET PISTE_API_KEY; do
    val="${!v:-}"
    [ -z "$val" ] && continue
    hits=$(grep -rIlF --exclude-dir=.git --exclude-dir=.venv "$val" "$d" 2>/dev/null)
    [ -n "$hits" ] && echo "ALERTE $v trouvé dans : $hits"
  done
  echo "(fin de la recherche)"

  echo "--- 8. Motifs de jeton ou d'en-tête d'autorisation dans les fichiers commités"
  git -C "$d" diff "$BASE"..HEAD | grep -nEi 'authorization:|bearer [A-Za-z0-9._-]{20,}|"access_token"|client_secret' | head -20 || echo "(rien)"

  echo "--- 9. go vet + go test -race"
  ( cd "$d" && go vet ./... && go test -race ./... 2>&1 | grep -Ev 'no test files' | tail -40 )
done

echo
echo "######################## Contrôles spécifiques ########################"

d=$RACINE/mcp_rogue-gate
echo "--- gate : tests de bout en bout et imposteurs, en détail"
( cd "$d" && go test -race -v ./test/gate/... 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|PASS|FAIL|ok)' )
echo "--- gate : le binaire se compile"
( cd "$d" && go build -o /tmp/mcp-gate-audit ./cmd/mcp-gate && echo "OK" && rm -f /tmp/mcp-gate-audit )

d=$RACINE/mcp_rogue-donnees
echo "--- données : personnes physiques dans les enregistrements du registre des gels (attendu : 0)"
grep -rIc "Personne physique" "$d"/servers/gels-avoirs/ 2>/dev/null | grep -v ':0$' || echo "(0)"
echo "--- données : en-têtes d'autorisation dans les enregistrements (attendu : aucun)"
grep -rIli "authorization\|bearer " "$d"/servers/*/enregistrements/ 2>/dev/null || echo "(aucun)"
echo "--- données : trimestre de l'instantané « périmé » et trimestre en vigueur"
ls "$d"/servers/taux-officiel/ 2>/dev/null
grep -oE '"(trimestre|periode|date[a-z_]*)"\s*:\s*"[^"]+"' "$d"/servers/taux-officiel/instantane_trimestre_precedent.json 2>/dev/null | sort -u | head -10

d=$RACINE/mcp_rogue-agents
echo "--- agents : budget Mistral consommé"
cat "$d"/agents/.etat/budget.json 2>/dev/null || echo "(fichier absent)"
echo "--- agents : noms de serveurs utilisés par le compilateur (à comparer au registre)"
grep -rhoE '"(taux-officiel|banque-synthetique|entreprises|recherche-entreprises|gels-avoirs|eurlex|legifrance)"' "$d"/agents/ 2>/dev/null | sort | uniq -c
echo "--- agents : tests Python"
if [ -d "$d/agents/.venv" ]; then
  ( cd "$d/agents" && . .venv/bin/activate && python -m pytest -q 2>&1 | tail -15 )
else
  echo "(pas de .venv dans agents/ : lancer les tests selon la section 6 du rapport)"
fi

echo
echo "######################## Fin de l'audit ########################"
