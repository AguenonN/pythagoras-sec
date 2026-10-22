# Console Pythagoras sur Aiven Runtime, en lecture seule.
# Contexte de build : la racine du dépôt.
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /out/console ./cmd/console

FROM python:3.12-slim
WORKDIR /app
COPY --from=build /out/console /app/console
COPY demo/directives /app/demo/directives
COPY demo/scenarios /app/demo/scenarios
COPY agents/compilateur/exemples/catalogue-demo.json /app/agents/compilateur/exemples/catalogue-demo.json
# Instantané des passages (journaux signés, fictifs) : scripts/instantane-runtime.sh
COPY deploy/console-runtime/instantane/ /app/demo/
COPY deploy/console-runtime/proxy.py deploy/console-runtime/demarrer.sh /app/
RUN useradd --system --uid 10001 pythagoras && mkdir -p /app/demo/execution && chown -R 10001 /app/demo/execution
USER 10001
ENV PORT=8080
EXPOSE 8080
CMD ["/app/demarrer.sh"]
