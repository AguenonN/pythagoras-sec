// Commande referentiel-rh : serveur MCP du référentiel des groupes RH d'une
// entreprise FICTIVE (salaries-proteges, contentieux-en-cours), pour la
// démonstration. Transports : stdio, ou HTTP streamable TLS (-http,
// -tls-cert, -tls-key).
package main

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/lanceur"
)

func main() {
	lanceur.Lancer(Nom, func(lanceur.Config) (*mcp.Server, error) {
		r, err := Charger()
		if err != nil {
			return nil, err
		}
		return Serveur(r), nil
	})
}
