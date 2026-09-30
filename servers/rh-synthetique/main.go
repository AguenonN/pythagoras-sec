// Commande rh-synthetique : serveur MCP des ressources humaines d'une
// entreprise FICTIVE (fiches salariés, primes et sanctions fictives), pour
// la démonstration. Transports : stdio, ou HTTP streamable TLS (-http,
// -tls-cert, -tls-key).
package main

import (
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/lanceur"
)

func main() {
	lanceur.Lancer(Nom, func(lanceur.Config) (*mcp.Server, error) {
		rh, err := NouveauRH(time.Now)
		if err != nil {
			return nil, err
		}
		return Serveur(rh), nil
	})
}
