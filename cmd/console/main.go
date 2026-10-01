// Command console sert les écrans de démonstration sur 127.0.0.1. Tout
// est embarqué dans le binaire (HTML, CSS, JavaScript) : aucune ressource
// n'est chargée depuis le réseau. La console lit les fichiers (journal,
// coffre, jeux de directives) et ne les écrit jamais.
//
//	MCP_GATE_CLE_JOURNAL=<hex> console -donnees demo/execution -jeux demo/directives
//
// La clé du journal n'est lue que dans l'environnement, comme le fait le
// gate. Sans clé, les écrans s'affichent mais la chaîne est déclarée non
// vérifiable, en rouge.
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"mcprogue/internal/console"
)

// VarCle est la variable qui porte la clé du journal, la même que le gate.
const VarCle = "MCP_GATE_CLE_JOURNAL"

func main() {
	if err := lancer(); err != nil {
		fmt.Fprintln(os.Stderr, "console:", err)
		os.Exit(1)
	}
}

func lancer() error {
	addr := flag.String("addr", "127.0.0.1:8088", "adresse d'écoute (boucle locale seulement)")
	donnees := flag.String("donnees", "demo/execution", "répertoire des journaux (un sous-répertoire par journal) ou fichier de journal")
	jeux := flag.String("jeux", "demo/directives", "répertoires ou fichiers de jeux de directives, séparés par des virgules")
	produit := flag.String("produit", "Console de gouvernance", "nom du produit affiché dans l'en-tête")
	flag.Parse()

	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return fmt.Errorf("adresse %q : %w", *addr, err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("adresse %q refusée : la console n'écoute que sur la boucle locale", *addr)
	}
	var cle []byte
	if v := os.Getenv(VarCle); v != "" {
		if cle, err = hex.DecodeString(v); err != nil {
			return fmt.Errorf("%s n'est pas de l'hexadécimal", VarCle)
		}
	} else {
		fmt.Fprintf(os.Stderr, "console: %s absente : les chaînes ne pourront pas être vérifiées.\n", VarCle)
	}
	dep := &console.Depot{Donnees: *donnees, Cle: cle}
	for _, j := range strings.Split(*jeux, ",") {
		if j = strings.TrimSpace(j); j != "" {
			dep.Jeux = append(dep.Jeux, j)
		}
	}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           (&console.Serveur{Depot: dep, Produit: *produit}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	fmt.Fprintf(os.Stderr, "console: http://%s (lecture seule ; journaux : %s)\n", *addr, *donnees)
	return srv.ListenAndServe()
}
