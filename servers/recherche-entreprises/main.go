// Commande recherche-entreprises : serveur MCP adaptateur de l'API Recherche
// d'entreprises. Mode fixé par DONNEES_MODE (DIRECT, REJEU, MIXTE).
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
