// Commande gels-avoirs : serveur MCP adaptateur du registre national des
// gels des avoirs (DG Trésor). Mode fixé par DONNEES_MODE, ou par -mode,
// prioritaire : le registre de démo lance gels-avoirs avec -mode=REJEU, sur
// le registre complet téléchargé le matin (« preparer »), pour qu'aucun
// appel n'attende le réseau (12 Mo, jusqu'à 90 s) pendant la démo ; la date
// de publication du registre lu figure dans chaque résultat.
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

var modeImpose = flag.String("mode", "", "mode imposé (DIRECT, REJEU ou MIXTE), prioritaire sur DONNEES_MODE")

func main() {
	if len(os.Args) > 1 && os.Args[1] == "preparer" {
		if err := preparer(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "preparer:", err)
			os.Exit(1)
		}
		return
	}
	lanceur.LancerDepuis(Nom, DossierLocal, func(c lanceur.Config) (*mcp.Server, error) {
		if *modeImpose != "" {
			m, err := rejeu.ParseMode(*modeImpose)
			if err != nil {
				return nil, err
			}
			fmt.Fprintf(os.Stderr, "%s: mode imposé par -mode : %s (DONNEES_MODE ignoré)\n", Nom, m)
			c.Mode = m
		}
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
