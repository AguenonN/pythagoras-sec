// Commande exporter-schema lance un binaire de serveur MCP sur stdio, lui
// envoie initialize puis tools/list en JSON-RPC brut, et écrit le champ
// result.tools de la réponse tel qu'il a été reçu, octet pour octet.
//
//	go run ./servers/exporter-schema -o servers/entreprises/schema.json bin/entreprises
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

func main() {
	sortie := flag.String("o", "", "fichier de sortie (stdout par défaut)")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: exporter-schema [-o schema.json] <binaire>")
		os.Exit(2)
	}
	tools, err := exporter(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "exporter-schema:", err)
		os.Exit(1)
	}
	tools = append(tools, '\n')
	if *sortie == "" {
		os.Stdout.Write(tools)
		return
	}
	if err := os.WriteFile(*sortie, tools, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "exporter-schema:", err)
		os.Exit(1)
	}
}

func exporter(binaire string) ([]byte, error) {
	cmd := exec.Command(binaire)
	// Aucun réseau pour lister les outils.
	cmd.Env = append(os.Environ(), "DONNEES_MODE=REJEU")
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		in.Close()
		done := make(chan struct{})
		go func() { cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			cmd.Process.Kill()
		}
	}()

	messages := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"exporter-schema","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}
	for _, m := range messages {
		if _, err := io.WriteString(in, m+"\n"); err != nil {
			return nil, err
		}
	}
	lecteur := bufio.NewReader(out)
	for {
		ligne, err := lecteur.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("pas de réponse à tools/list: %v", err)
		}
		var rep struct {
			ID     json.RawMessage `json:"id"`
			Result struct {
				Tools      json.RawMessage `json:"tools"`
				NextCursor string          `json:"nextCursor"`
			} `json:"result"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(ligne, &rep); err != nil {
			continue
		}
		if string(rep.ID) != "2" {
			continue
		}
		if rep.Error != nil {
			return nil, fmt.Errorf("tools/list: %s", rep.Error)
		}
		if rep.Result.NextCursor != "" {
			return nil, fmt.Errorf("tools/list paginé : non géré")
		}
		return rep.Result.Tools, nil
	}
}
