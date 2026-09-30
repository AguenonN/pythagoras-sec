// Commande eurlex : serveur MCP adaptateur de CELLAR (Office des
// publications de l'UE) pour les actes et leurs articles. Mode fixé par
// DONNEES_MODE.
package main

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/lanceur"
)

func main() {
	lanceur.Lancer(Nom, func(c lanceur.Config) (*mcp.Server, error) {
		return Serveur(c.Client()), nil
	})
}
