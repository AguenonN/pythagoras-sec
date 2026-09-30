// Commande appeler appelle un outil d'un serveur MCP, pour les essais
// manuels : binaire lancé sur stdio (le mode DONNEES_MODE et les autres
// variables sont hérités), ou URL HTTP streamable sur TLS.
//
//	DONNEES_MODE=DIRECT go run ./servers/appeler bin/recherche-entreprises rechercher_entreprise '{"siren":"405750753"}'
//	go run ./servers/appeler -ca cert.pem https://127.0.0.1:8443/mcp evaluer_dossier '{"dossier_id":"DOS-FICTIF-001"}'
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	ca := flag.String("ca", "", "certificat (PEM) à reconnaître pour une URL https (sinon magasin du système)")
	flag.Parse()
	if flag.NArg() != 3 {
		fmt.Fprintln(os.Stderr, "usage: appeler [-ca cert.pem] <binaire | https://hôte/mcp> <outil> '<arguments JSON>'")
		os.Exit(2)
	}
	cible, outil := flag.Arg(0), flag.Arg(1)
	var args map[string]any
	if err := json.Unmarshal([]byte(flag.Arg(2)), &args); err != nil {
		fmt.Fprintln(os.Stderr, "arguments:", err)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var transport mcp.Transport
	if strings.HasPrefix(cible, "https://") {
		conf := &tls.Config{MinVersion: tls.VersionTLS12}
		if *ca != "" {
			pem, err := os.ReadFile(*ca)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			conf.RootCAs = x509.NewCertPool()
			if !conf.RootCAs.AppendCertsFromPEM(pem) {
				fmt.Fprintln(os.Stderr, "-ca: aucun certificat PEM")
				os.Exit(2)
			}
		}
		transport = &mcp.StreamableClientTransport{Endpoint: cible, HTTPClient: &http.Client{Transport: &http.Transport{TLSClientConfig: conf}}}
	} else if strings.Contains(cible, "://") {
		fmt.Fprintln(os.Stderr, "seul https:// est accepté")
		os.Exit(2)
	} else {
		cmd := exec.Command(cible)
		cmd.Stderr = os.Stderr
		transport = &mcp.CommandTransport{Command: cmd}
	}

	c := mcp.NewClient(&mcp.Implementation{Name: "appeler", Version: "0"}, nil)
	cs, err := c.Connect(ctx, transport, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer cs.Close()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: outil, Arguments: args})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, _ := json.MarshalIndent(map[string]any{"isError": res.IsError, "structuredContent": res.StructuredContent}, "", "  ")
	fmt.Println(string(out))
}
