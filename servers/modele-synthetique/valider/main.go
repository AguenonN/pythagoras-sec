// Commande valider contrôle un couple outils.json / donnees.json avant la
// création d'un serveur synthétique (scripts/nouveau-serveur.sh l'appelle).
//
//	go run ./servers/modele-synthetique/valider outils.json donnees.json
package main

import (
	"fmt"
	"os"

	modele "mcprogue/servers/modele-synthetique"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage : valider <outils.json> <donnees.json>")
		os.Exit(2)
	}
	o, d, err := modele.LireFichiers(os.Args[1], os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, "valider :", err)
		os.Exit(1)
	}
	for _, a := range modele.Avertissements(o) {
		fmt.Fprintln(os.Stderr, "AVERTISSEMENT :", a)
	}
	n := 0
	for _, l := range d.Collections {
		n += len(l)
	}
	fmt.Fprintf(os.Stderr, "valider : %d outil(s), %d collection(s), %d enregistrement(s) : cohérent\n", len(o.Outils), len(d.Collections), n)
}
