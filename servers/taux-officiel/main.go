// Commande taux-officiel : serveur MCP adaptateur des seuils de l'usure
// publiés par la Banque de France (export public Webstat). Mode fixé par
// DONNEES_MODE.
//
//	taux-officiel                      serveur MCP sur stdio
//	taux-officiel instantane [-date]   écrit instantane_trimestre_precedent.json
//	                                   depuis les enregistrements (sans réseau)
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/lanceur"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "instantane" {
		if err := commandeInstantane(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "instantane:", err)
			os.Exit(1)
		}
		return
	}
	lanceur.Lancer(Nom, func(c lanceur.Config) (*mcp.Server, error) {
		return Serveur(c.Client(), time.Now), nil
	})
}
