// Command directive vérifie et relit un jeu de directives lu sur l'entrée
// standard. C'est le seul juge de ce que le compilateur propose : aucune
// sortie de modèle n'est acceptée sans passer par le chargeur strict.
//
//	directive verifier < jeu.json   {"ok":true,...} ou {"ok":false,"erreurs":[...]}
//	directive relire   < jeu.json   relecture en français, par gabarit
//
// Codes de sortie : 0 succès, 1 jeu refusé, 2 usage ou lecture impossible.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"mcprogue/internal/directive"
	"mcprogue/internal/relecture"
)

const usage = "usage : directive verifier|relire < jeu.json"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	donnees, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "directive: lecture de l'entrée standard:", err)
		return 2
	}
	switch args[0] {
	case "verifier":
		return verifier(donnees, stdout)
	case "relire":
		return relire(donnees, stdout, stderr)
	}
	fmt.Fprintln(stderr, usage)
	return 2
}

// Rapport est la sortie JSON de verifier.
type Rapport struct {
	OK        bool     `json:"ok"`
	Empreinte string   `json:"empreinte,omitempty"`
	Erreurs   []string `json:"erreurs,omitempty"`
}

func verifier(donnees []byte, stdout io.Writer) int {
	jeu, err := directive.Charger(donnees)
	r := Rapport{OK: err == nil}
	if err == nil {
		r.Empreinte = jeu.Meta.Empreinte
	} else {
		r.Erreurs = erreurs(err)
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(r)
	if !r.OK {
		return 1
	}
	return 0
}

func relire(donnees []byte, stdout, stderr io.Writer) int {
	jeu, err := directive.Charger(donnees)
	if err != nil {
		fmt.Fprintln(stderr, "directive: jeu refusé, pas de relecture :")
		for _, e := range erreurs(err) {
			fmt.Fprintln(stderr, "  -", e)
		}
		return 1
	}
	io.WriteString(stdout, relecture.Jeu(jeu))
	return 0
}

// erreurs déplie les erreurs jointes par le chargeur, une par défaut.
func erreurs(err error) []string {
	var out []string
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range j.Unwrap() {
			out = append(out, erreurs(e)...)
		}
		return out
	}
	for _, l := range strings.Split(err.Error(), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
