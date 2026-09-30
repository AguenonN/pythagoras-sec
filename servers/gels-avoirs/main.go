// Commande gels-avoirs : serveur MCP adaptateur du registre national des
// gels des avoirs (DG Trésor). Mode fixé par DONNEES_MODE.
//
//	gels-avoirs            serveur MCP sur stdio ; registre lu dans
//	                       servers/gels-avoirs/enregistrements_locaux/
//	gels-avoirs preparer   télécharge le registre complet (réseau) et
//	                       l'enregistre dans enregistrements_locaux/,
//	                       ignoré par Git
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/lanceur"
	"mcprogue/servers/internal/rejeu"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "preparer" {
		if err := preparer(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "preparer:", err)
			os.Exit(1)
		}
		return
	}
	lanceur.LancerDepuis(Nom, DossierLocal, func(c lanceur.Config) (*mcp.Server, error) {
		return Serveur(c.Client()), nil
	})
}

// preparer enregistre la dernière publication complète du registre. Il
// n'affiche aucun nom, seulement des comptes.
func preparer(args []string) error {
	fs := flag.NewFlagSet("preparer", flag.ContinueOnError)
	dir := fs.String("enregistrements", DossierLocal, "répertoire local (ignoré par Git)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c := rejeu.Nouveau(*dir, rejeu.Direct)
	c.Restreindre(HoteRegistre)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	r, err := c.Obtenir(ctx, rejeu.Requete{URL: URLRegistre, Entetes: map[string][]string{"Accept": {"application/json"}}})
	if err != nil {
		return err
	}
	reg, err := (&index{}).charger(r)
	if err != nil {
		return err
	}
	natures := map[string]int{}
	for _, f := range reg.fiches {
		natures[f.Nature]++
	}
	fmt.Fprintf(os.Stderr, "registre publié le %s : %d entrées %v\nenregistré dans %s (sha256 %s)\n",
		reg.date, len(reg.fiches), natures, *dir, r.Source.SHA256)
	return nil
}
