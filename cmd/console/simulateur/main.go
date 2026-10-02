// Command simulateur est un orchestrateur et un gate simulés : il exécute une
// demande de bout en bout selon les contrats de la démonstration (dossier de
// session, état, jetons d'approbation relus toutes les secondes), pour mettre
// au point la console sans la démo réelle.
//
//	simulateur preparer -racine /tmp/essai        # règles, annuaire signé et clés fictifs
//	MCP_GATE_CLE_JOURNAL=<hex> simulateur demande -racine /tmp/essai --demandeur claire.analyste@exemple.test "Peut-on accorder le prêt demandé ?"
//
// La console le lance comme elle lancerait scripts/lancer-demo.sh :
//
//	console -lanceur "bin/simulateur"
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"time"

	"mcprogue/internal/console/simulateur"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "simulateur:", err)
		os.Exit(1)
	}
}

func run() error {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "preparer" {
		fs := flag.NewFlagSet("preparer", flag.ContinueOnError)
		racine := fs.String("racine", ".", "racine du dépôt (ou d'un dossier de mise au point)")
		demo := fs.String("demo", "demo", "dossier de la démonstration, sous la racine")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return simulateur.PreparerDemo(*racine, *demo)
	}
	if len(args) == 0 || args[0] != "demande" {
		return fmt.Errorf("usage : simulateur preparer [-racine DIR] | demande --demandeur <identité> [-pause 800ms] <texte>")
	}
	fs := flag.NewFlagSet("demande", flag.ContinueOnError)
	demandeur := fs.String("demandeur", "", "identité du demandeur (annuaire)")
	racine := fs.String("racine", ".", "racine du dépôt")
	demo := fs.String("demo", "demo", "dossier de la démonstration, sous la racine")
	pauseDefaut := 800 * time.Millisecond
	if v, err := time.ParseDuration(os.Getenv("SIMULATEUR_PAUSE")); err == nil {
		pauseDefaut = v
	}
	pause := fs.Duration("pause", pauseDefaut, "pause entre deux appels (défaut : SIMULATEUR_PAUSE, sinon 800 ms)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *demandeur == "" || fs.NArg() != 1 {
		return fmt.Errorf("usage : simulateur demande --demandeur <identité> <texte>")
	}
	cle, err := hex.DecodeString(os.Getenv("MCP_GATE_CLE_JOURNAL"))
	if err != nil || len(cle) == 0 {
		return fmt.Errorf("MCP_GATE_CLE_JOURNAL absente ou illisible : pas de journal sans clé")
	}
	dossier, err := simulateur.Config{Racine: *racine, Demo: *demo, Cle: cle, Pause: *pause}.Demande(*demandeur, fs.Arg(0))
	if dossier != "" {
		fmt.Fprintln(os.Stderr, "simulateur: session", dossier)
	}
	return err
}
