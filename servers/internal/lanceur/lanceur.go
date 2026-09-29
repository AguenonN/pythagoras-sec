// Package lanceur factorise le main des serveurs de données : nom logique,
// mode de rejeu, répertoire d'enregistrements, transport stdio.
package lanceur

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/nommage"
	"mcprogue/servers/internal/rejeu"
)

// Config est ce que le main transmet au constructeur du serveur.
type Config struct {
	Mode            rejeu.Mode
	Enregistrements string
}

// Client crée un client de rejeu sur le répertoire du serveur.
func (c Config) Client() *rejeu.Client { return rejeu.Nouveau(c.Enregistrements, c.Mode) }

// Lancer lit les options, construit le serveur et le sert sur stdio.
// Il ne rend la main qu'à la fin de la session ; il quitte le processus en
// cas d'erreur.
func Lancer(nom string, construire func(Config) (*mcp.Server, error)) {
	if err := nommage.ValiderNomLogique(nom); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	dir := flag.String("enregistrements", "servers/"+nom+"/enregistrements",
		"répertoire des enregistrements (versionné)")
	flag.Parse()

	mode, err := rejeu.ModeDepuisEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	srv, err := construire(Config{Mode: mode, Enregistrements: *dir})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", nom, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "%s: mode %s, enregistrements %s\n", nom, mode, *dir)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", nom, err)
		os.Exit(1)
	}
}
