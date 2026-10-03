// Commande bourse : serveur MCP de démonstration, données FICTIVES. Aucun code
// propre : les outils sont déclarés dans outils.json, les données dans
// donnees.json (servers/modele-synthetique). Transports : stdio, ou HTTP
// streamable TLS (-http, -tls-cert, -tls-key), par le lanceur commun.
package main

import (
	_ "embed"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/lanceur"
	modele "mcprogue/servers/modele-synthetique"
)

// Nom est le nom logique du serveur (internal/nommage).
const Nom = "bourse"

//go:embed outils.json
var outils []byte

//go:embed donnees.json
var donnees []byte

func main() {
	lanceur.Lancer(Nom, func(lanceur.Config) (*mcp.Server, error) {
		return modele.Serveur(Nom, outils, donnees)
	})
}
