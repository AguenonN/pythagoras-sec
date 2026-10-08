# The MCP go-sdk requires Go >= 1.25. The lab targets run everything in
# containers (golang:1.25); `make test` needs a local Go >= 1.25 (or
# GOTOOLCHAIN=auto to fetch it).

COMPOSE = docker compose -f deploy/docker-compose.yml

.PHONY: test build agent network-config rogue mcp-observability agent-sre checkout-api mcp-proxy \
        lab-up lab-register lab-naive-rogue lab-zerotrust-rogue lab-zerotrust-legit lab-down clean carte

# ---- Proof of the thesis, no Docker needed ----
# Runs the real MCP-over-mTLS gate through all three scenarios with an in-memory
# CA standing in for SPIRE. This is the executable heart of the project.
test:
	GOTOOLCHAIN=auto go test ./... -count=1

# ---- Carte des briques ----
# Relit le graphe d'imports du code, le fusionne avec carte/annotations.json
# et écrit la page autonome. À relancer après tout ajout ou retrait de paquet.
carte:
	python3 scripts/carte-paquets.py
	python3 -c "import pathlib; t=pathlib.Path('carte/page.html').read_text(encoding='utf-8'); d=pathlib.Path('carte/graphe.json').read_text(encoding='utf-8'); pathlib.Path('carte/carte.html').write_text(t.replace('/*__DATA__*/', d), encoding='utf-8')"
	@echo 'carte/carte.html — ouvrir dans un navigateur'

build: agent network-config rogue mcp-observability agent-sre checkout-api mcp-proxy

agent network-config rogue mcp-observability agent-sre checkout-api mcp-proxy:
	GOTOOLCHAIN=auto go build -o bin/$@ ./cmd/$@

# ---- Containerised lab (SPIRE + real workloads) ----

# Start ONLY the SPIRE server and wait until it is healthy.
lab-up:
	$(COMPOSE) up -d --build --wait spire-server

# Generate the join token, create entries (legit + agent, never the rogue),
# then start the agent and both MCP servers.
lab-register:
	bash deploy/register-entries.sh

# NAIVE agent -> ROGUE: robbed (see the poisoned reply and /tmp/stolen.log).
lab-naive-rogue:
	$(COMPOSE) run --rm \
	  -e TRUST_MODE=naive -e MCP_ENDPOINT=https://rogue:8443 agent

# ZERO-TRUST agent -> ROGUE: refused at the handshake, no SVID.
lab-zerotrust-rogue:
	$(COMPOSE) run --rm \
	  -e TRUST_MODE=zerotrust -e MCP_ENDPOINT=https://rogue:8443 \
	  -e MCP_EXPECTED_ID=spiffe://mcp.lab/network-config agent

# ZERO-TRUST agent -> LEGIT: identity proven, real router.conf returned.
lab-zerotrust-legit:
	$(COMPOSE) run --rm \
	  -e TRUST_MODE=zerotrust -e MCP_ENDPOINT=https://network-config:8443 \
	  -e MCP_EXPECTED_ID=spiffe://mcp.lab/network-config agent

lab-down:
	$(COMPOSE) --profile agent --profile manual down -v
	rm -f deploy/spire/bootstrap/agent-token

clean:
	rm -rf bin stolen.log
