// Commande banque-synthetique : serveur MCP d'une banque FICTIVE (dossiers
// de crédit, base interne des taux, attribution de prêts fictifs), pour la
// démonstration. Transports : stdio, ou HTTP streamable TLS (-http,
// -tls-cert, -tls-key).
package main

import (
	"flag"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/lanceur"
)

var cheminInstantane = flag.String("instantane", "servers/taux-officiel/instantane_trimestre_precedent.json",
	"instantané des seuils servi comme « base interne »")

func main() {
	lanceur.Lancer(Nom, func(lanceur.Config) (*mcp.Server, error) {
		dossiers, err := ChargerDossiers()
		if err != nil {
			return nil, err
		}
		inst, err := ChargerInstantane(*cheminInstantane)
		if err != nil {
			return nil, err
		}
		return Serveur(NouvelleBanque(dossiers, inst, time.Now)), nil
	})
}
