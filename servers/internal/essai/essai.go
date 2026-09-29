// Package essai regroupe les aides de test des serveurs de données.
package essai

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Connecter ouvre une session client en mémoire sur srv.
func Connecter(t *testing.T, srv *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	c := mcp.NewClient(&mcp.Implementation{Name: "essai", Version: "0"}, nil)
	cs, err := c.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// Appeler appelle un outil et décode son contenu structuré dans out. Il rend
// le résultat brut pour les vérifications d'IsError.
func Appeler(t *testing.T, cs *mcp.ClientSession, outil string, args any, out any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: outil, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", outil, err)
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("%s: contenu structuré %s: %v", outil, b, err)
	}
	return res
}

// VerifierSchema compare schema.json (liste tools brute) à tools/list.
// L'égalité est testée sur la valeur JSON : le fichier est produit par
// servers/exporter-schema, octet pour octet, depuis le binaire.
func VerifierSchema(t *testing.T, cs *mcp.ClientSession, chemin string) {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	vivant, err := json.Marshal(res.Tools)
	if err != nil {
		t.Fatal(err)
	}
	fige, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatalf("%s absent : lancer servers/exporter-schema (%v)", chemin, err)
	}
	if !memeJSON(t, vivant, fige) {
		t.Fatalf("%s ne correspond plus à tools/list ; régénérer avec servers/exporter-schema\nvivant: %s", chemin, vivant)
	}
}

func memeJSON(t *testing.T, a, b []byte) bool {
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatal(err)
	}
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	return bytes.Equal(xa, ya)
}
