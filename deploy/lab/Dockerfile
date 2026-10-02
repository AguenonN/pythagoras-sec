# Image du lab SPIFFE : le gate, le serveur de démo, son sidecar, la sonde et
# l'outil de relecture du journal. Construite depuis la racine du dépôt :
#   docker build -t mcp-gate-lab:local -f deploy/lab/Dockerfile .
# Go >= 1.25 (go-sdk MCP).
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN for p in cmd/mcp-gate cmd/journal servers/banque-synthetique deploy/lab/spiffe-proxy deploy/lab/sonde; do \
      CGO_ENABLED=0 go build -trimpath -o /out/$(basename $p) ./$p || exit 1; \
    done

# Variante « debug » de distroless : un shell busybox, pour que verifier.sh
# puisse lire le journal du gate dans son pod.
FROM gcr.io/distroless/static-debian12:debug
COPY --from=build /out/ /usr/local/bin/
COPY servers/taux-officiel/instantane_trimestre_precedent.json /lab/instantane.json
