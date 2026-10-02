// Commande sonde est le client du lab : une charge SPIRE qui appelle le gate
// en HTTP streamable + mTLS SPIFFE, avec sa propre SVID, et affiche ce que le
// gate répond. Sans entrée SPIRE (le rogue), elle n'obtient aucune SVID et
// s'arrête avant tout appel.
//
//	sonde -url https://gate.mcp-lab.svc.cluster.local:9443/mcp \
//	      -gate spiffe://mcp.gcp.lab/gate -outil banque-synthetique__attribuer_pret \
//	      -args '{"dossier_id":"...","montant":1000}'
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spiffe/go-spiffe/v2/spiffeid"

	"mcprogue/internal/identity"
)

func main() {
	url := flag.String("url", "", "url du gate")
	gate := flag.String("gate", "", "SPIFFE ID attendu du gate")
	outil := flag.String("outil", "", "outil à appeler (vide : liste seulement)")
	args := flag.String("args", "{}", "arguments JSON")
	socket := flag.String("spiffe-socket", "", "Workload API (défaut : SPIFFE_ENDPOINT_SOCKET)")
	flag.Parse()
	if err := lancer(*url, *gate, *outil, *args, *socket); err != nil {
		fmt.Fprintln(os.Stderr, "sonde:", err)
		os.Exit(1)
	}
}

func lancer(url, gate, outil, args, socket string) error {
	attendu, err := spiffeid.FromString(gate)
	if err != nil {
		return fmt.Errorf("-gate : %w", err)
	}
	ctx, annuler := context.WithTimeout(context.Background(), 30*time.Second)
	defer annuler()
	src, err := identity.NewSource(ctx, socket)
	if err != nil {
		return fmt.Errorf("aucune identité SPIFFE : %w", err)
	}
	defer src.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "sonde", Version: "0"}, nil).
		Connect(ctx, &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: src.ClientHTTP(attendu)}, nil)
	if err != nil {
		return fmt.Errorf("connexion au gate : %w", err)
	}
	defer cs.Close()
	for t, err := range cs.Tools(ctx, nil) {
		if err != nil {
			return err
		}
		fmt.Println("outil", t.Name)
	}
	if outil == "" {
		return nil
	}
	var a map[string]any
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return fmt.Errorf("-args : %w", err)
	}
	r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: outil, Arguments: a})
	if err != nil {
		return err
	}
	b, _ := json.Marshal(r)
	fmt.Println("résultat", string(b))
	return nil
}
