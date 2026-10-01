// Commande rapports est un serveur MCP fictif, propre aux tests
// d'intégration : un seul outil, remettre_rapport (action), qui accuse
// réception d'un rapport sans rien en faire. Il donne au registre de test
// un outil que la règle */remettre_rapport de demo/etiquettes.json
// étiquette production_rapport.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type entree struct {
	Titre   string `json:"titre" jsonschema:"titre du rapport"`
	Contenu string `json:"contenu" jsonschema:"texte du rapport"`
}

type sortie struct {
	Statut string `json:"statut"`
}

func main() {
	s := mcp.NewServer(&mcp.Implementation{Name: "rapports", Version: "0.1.0"}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "remettre_rapport",
		Description: "Remet un rapport au client (serveur fictif de test : accuse réception, ne transmet rien).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in entree) (*mcp.CallToolResult, sortie, error) {
		return nil, sortie{Statut: "recu"}, nil
	})
	fmt.Fprintln(os.Stderr, "rapports: prêt")
	if err := s.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "rapports:", err)
	}
}
