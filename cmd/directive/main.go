// Command directive vérifie et relit un jeu de directives lu sur l'entrée
// standard. C'est le seul juge de ce que le compilateur propose : aucune
// sortie de modèle n'est acceptée sans passer par le chargeur strict.
//
//	directive verifier [--exiger-brouillon] < jeu.json   {"ok":true,...} ou {"ok":false,"erreurs":[...]}
//	directive relire   < jeu.json                       relecture en français, par gabarit
//
// --exiger-brouillon refuse aussi toute directive dont le statut n'est pas
// brouillon : c'est ainsi que le compilateur s'assure de ne jamais produire
// une directive appliquée.
//
// Codes de sortie : 0 succès, 1 jeu refusé, 2 usage ou lecture impossible.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"mcprogue/internal/directive"
	"mcprogue/internal/relecture"
)

const usage = "usage : directive verifier [--exiger-brouillon] | relire < jeu.json"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	sous := args[0]
	fs := flag.NewFlagSet(sous, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var exigerBrouillon *bool
	switch sous {
	case "verifier":
		exigerBrouillon = fs.Bool("exiger-brouillon", false, "refuser toute directive qui n'est pas en brouillon")
	case "relire":
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	donnees, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "directive: lecture de l'entrée standard:", err)
		return 2
	}
	if sous == "verifier" {
		return verifier(donnees, *exigerBrouillon, stdout)
	}
	return relire(donnees, stdout, stderr)
}

// Rapport est la sortie JSON de verifier.
type Rapport struct {
	OK        bool     `json:"ok"`
	Empreinte string   `json:"empreinte,omitempty"`
	Erreurs   []string `json:"erreurs,omitempty"`
}

func verifier(donnees []byte, exigerBrouillon bool, stdout io.Writer) int {
	jeu, err := directive.Charger(donnees)
	var r Rapport
	if err != nil {
		r.Erreurs = erreurs(err)
	}
	if exigerBrouillon {
		r.Erreurs = append(r.Erreurs, horsBrouillon(jeu, donnees)...)
	}
	r.OK = len(r.Erreurs) == 0
	if r.OK {
		r.Empreinte = jeu.Meta.Empreinte
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

// horsBrouillon liste les directives dont le statut n'est pas brouillon. Si
// le chargeur a refusé le jeu, les statuts sont relus sans contrôle, pour que
// la liste d'erreurs reste complète en une passe.
func horsBrouillon(jeu *directive.Jeu, donnees []byte) []string {
	type ligne struct {
		ID     string `json:"id"`
		Statut string `json:"statut"`
	}
	var lignes []ligne
	if jeu != nil {
		for _, d := range jeu.Directives {
			lignes = append(lignes, ligne{d.ID, string(d.Statut)})
		}
	} else {
		var brut struct {
			Directives []ligne `json:"directives"`
		}
		_ = json.Unmarshal(donnees, &brut)
		lignes = brut.Directives
	}
	var out []string
	for i, l := range lignes {
		if l.Statut == string(directive.Brouillon) {
			continue
		}
		nom := fmt.Sprintf("directive %d", i+1)
		if l.ID != "" {
			nom = "directive " + l.ID
		}
		out = append(out, fmt.Sprintf("directive: %s: statut %q, brouillon exigé", nom, l.Statut))
	}
	return out
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
