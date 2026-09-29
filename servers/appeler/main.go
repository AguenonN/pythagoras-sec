// Commande appeler lance un binaire de serveur MCP sur stdio et appelle un
// outil, pour les essais manuels. Le mode (DONNEES_MODE) et les autres
// variables sont hérités de l'environnement.
//
//	DONNEES_MODE=DIRECT go run ./servers/appeler bin/entreprises rechercher_entreprise '{"siren":"405750753"}'
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: appeler <binaire> <outil> '<arguments JSON>'")
		os.Exit(2)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(os.Args[3]), &args); err != nil {
		fmt.Fprintln(os.Stderr, "arguments:", err)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.Command(os.Args[1])
	cmd.Stderr = os.Stderr
	c := mcp.NewClient(&mcp.Implementation{Name: "appeler", Version: "0"}, nil)
	cs, err := c.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer cs.Close()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: os.Args[2], Arguments: args})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, _ := json.MarshalIndent(map[string]any{"isError": res.IsError, "structuredContent": res.StructuredContent}, "", "  ")
	fmt.Println(string(out))
}
