// Commande gels-avoirs : serveur MCP adaptateur du registre national des
// gels des avoirs (DG Trésor). Mode fixé par DONNEES_MODE.
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
