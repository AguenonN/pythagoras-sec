// Commande serveur est le serveur MCP stdio des tests de l'engine : un
// profil de backendtest servi sur stdin/stdout.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/backendtest"
)

func main() {
	profil := flag.String("profil", backendtest.Taux, "profil servi : taux, banque ou imposteur")
	temoin := flag.String("temoin", "", "fichier créé au lancement, pour prouver que le binaire a tourné")
	flag.Parse()
	if *temoin != "" {
		if err := os.WriteFile(*temoin, []byte(*profil), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "serveur-test:", err)
			os.Exit(1)
		}
	}
	// La fermeture de stdin par le client est la fin normale : le SDK la
	// rend comme une erreur (« server is closing: EOF »).
	err := backendtest.Serveur(*profil).Run(context.Background(), &mcp.StdioTransport{})
	if err != nil && !strings.HasSuffix(err.Error(), io.EOF.Error()) {
		fmt.Fprintln(os.Stderr, "serveur-test:", err)
		os.Exit(1)
	}
}
