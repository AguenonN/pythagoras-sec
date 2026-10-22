# Console Pythagoras « live » sur Aiven Runtime : les agents Mistral, le gate
# et les 12 serveurs MCP tournent dans le conteneur. Contexte : racine du dépôt.
# La préparation (serveurs, gate, registre, annuaire et clés fictives,
# registre des gels du jour) se fait au build ; aucun secret n'y est utilisé.
FROM golang:1.25
RUN apt-get update && apt-get install -y --no-install-recommends python3 && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN scripts/lancer-demo.sh preparer \
 && CGO_ENABLED=0 go build -trimpath -buildvcs=false -o bin/console ./cmd/console \
 && mkdir -p demo/execution/sessions \
 && cp -r deploy/console-runtime/instantane/execution/sessions/. demo/execution/sessions/ \
 && rm -rf /root/.cache/go-build
ENV PORT=8080
EXPOSE 8080
CMD ["/app/deploy/console-live/demarrer.sh"]
