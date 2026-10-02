#!/usr/bin/env bash
# Vérifie le lab monté par monter.sh. Code de sortie 0 si tout est conforme.
#
#  1. le gate a attesté le serveur en SPIFFE (journal du gate) ;
#  2. l'agent (SVID admise) appelle le gate, qui appelle le serveur : réponse ;
#  3. le journal du gate se vérifie, et l'appelant consigné est l'identité
#     SPIFFE de l'agent ;
#  4. le rogue (aucune SVID) n'obtient aucune identité : rien n'est appelé ;
#  5. une charge dotée d'une SVID valide mais non admise (le serveur de démo)
#     est refusée par le gate à la poignée de main.
set -uo pipefail

ns=mcp-lab
echec=0
ok()  { echo "   ok : $*"; }
ko()  { echo "   ÉCHEC : $*" >&2; echec=1; }

# sonde SA ARGS... : lance la sonde dans un pod éphémère sous le compte SA.
sonde() {
	local sa="$1"; shift
	local nom="sonde-${sa}-$$"
	kubectl -n "$ns" run "$nom" --rm -i --restart=Never --quiet --image=mcp-gate-lab:local --image-pull-policy=Never \
		--overrides="{\"spec\":{\"serviceAccountName\":\"${sa}\",\"containers\":[{\"name\":\"${nom}\",\"image\":\"mcp-gate-lab:local\",\"imagePullPolicy\":\"Never\",\"stdin\":true,\"command\":[\"/usr/local/bin/sonde\"],\"args\":$(printf '%s\n' "$@" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read().splitlines()))'),\"env\":[{\"name\":\"SPIFFE_ENDPOINT_SOCKET\",\"value\":\"unix:///spiffe/api.sock\"}],\"volumeMounts\":[{\"name\":\"spiffe\",\"mountPath\":\"/spiffe\",\"readOnly\":true}]}],\"volumes\":[{\"name\":\"spiffe\",\"hostPath\":{\"path\":\"/run/spire/sockets\",\"type\":\"Directory\"}}]}}" 2>&1
}
url="https://gate.mcp-lab.svc.cluster.local:9443/mcp"
gate="spiffe://mcp.gcp.lab/gate"
args='{"dossier_id":"DOS-FICTIF-001","siren":"405750753","montant":1000}'

echo ">> 1. attestation SPIFFE du serveur par le gate"
if kubectl -n "$ns" exec deploy/gate -- /busybox/cat /data/journal.jsonl 2>/dev/null | grep -q '"mode":"spiffe"'; then
	ok "attestation spiffe consignée"
else
	ko "aucune attestation spiffe au journal du gate"
fi

echo ">> 2. l'agent appelle le gate"
sortie="$(sonde sa-agent -url "$url" -gate "$gate" -outil banque-synthetique__attribuer_pret -args "$args")"
if grep -q "^résultat" <<<"$sortie" && ! grep -q '"isError":true' <<<"$sortie"; then ok "appel transmis, résultat reçu"; else ko "appel de l'agent : $sortie"; fi

echo ">> 3. journal du gate"
if kubectl -n "$ns" exec deploy/gate -- /usr/local/bin/journal verifier /data/journal.jsonl >/dev/null 2>&1; then ok "chaîne vérifiée"; else ko "chaîne du journal"; fi
if kubectl -n "$ns" exec deploy/gate -- /busybox/cat /data/journal.jsonl | grep -q '"appelant":"spiffe://mcp.gcp.lab/agent"'; then
	ok "appelant consigné : spiffe://mcp.gcp.lab/agent"
else
	ko "appelant SPIFFE absent du journal"
fi

echo ">> 4. le rogue n'a aucune identité"
sortie="$(sonde sa-rogue -url "$url" -gate "$gate" -outil banque-synthetique__attribuer_pret -args "$args")"
if grep -q "aucune identité SPIFFE" <<<"$sortie" && ! grep -q "^résultat" <<<"$sortie"; then ok "refusé : pas de SVID"; else ko "le rogue : $sortie"; fi

echo ">> 5. SVID valide mais non admise"
sortie="$(sonde sa-banque -url "$url" -gate "$gate" -outil banque-synthetique__attribuer_pret -args "$args")"
if grep -q "connexion au gate" <<<"$sortie" && ! grep -q "^résultat" <<<"$sortie"; then ok "refusé à la poignée de main"; else ko "charge non admise : $sortie"; fi

exit $echec
