// Command directive vérifie et relit un jeu de directives lu sur l'entrée
// standard. C'est le seul juge de ce que le compilateur propose : aucune
// sortie de modèle n'est acceptée sans passer par le chargeur strict.
// Seul le format v2 est admis (docs/schema-directive-v2.md) : une directive
// v1 (clé verrou, ou sans déclencheur) est refusée par verifier et relire,
// une erreur par directive fautive, désignée par sa position.
//
//	directive verifier [--exiger-brouillon] < jeu.json   {"ok":true,...} ou {"ok":false,"erreurs":[...]}
//	directive relire   [--directives D-009,D-010] < jeu.json
//	                                                    relecture en français, par gabarit ; avec
//	                                                    --directives, seulement celles-ci (ajout à un jeu)
//	directive fuseaux                                   fuseaux admis par le chargeur, en JSON
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

const usage = "usage : directive verifier [--exiger-brouillon] | relire [--directives id,id] < jeu.json | fuseaux"

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
	var seulement *string
	switch sous {
	case "verifier":
		exigerBrouillon = fs.Bool("exiger-brouillon", false, "refuser toute directive qui n'est pas en brouillon")
	case "relire":
		seulement = fs.String("directives", "", "identifiants des seules directives à relire, séparés par des virgules")
	case "fuseaux":
		// La liste fermée du chargeur : le compilateur en tire l'énumération
		// offerte au modèle, plutôt que d'en tenir une copie.
		if len(args) != 1 {
			fmt.Fprintln(stderr, usage)
			return 2
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(directive.Fuseaux())
		return 0
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
	var ids []string
	if *seulement != "" {
		ids = strings.Split(*seulement, ",")
	}
	return relire(donnees, ids, stdout, stderr)
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
	r.Erreurs = append(r.Erreurs, formatV1(donnees, r.Erreurs)...)
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

func relire(donnees []byte, ids []string, stdout, stderr io.Writer) int {
	jeu, err := directive.Charger(donnees)
	var refus []string
	if err != nil {
		refus = erreurs(err)
	}
	refus = append(refus, formatV1(donnees, refus)...)
	if len(refus) > 0 {
		fmt.Fprintln(stderr, "directive: jeu refusé, pas de relecture :")
		for _, e := range refus {
			fmt.Fprintln(stderr, "  -", e)
		}
		return 1
	}
	if ids == nil {
		io.WriteString(stdout, relecture.Jeu(jeu))
		return 0
	}
	texte, err := relecture.Ajout(jeu, ids)
	if err != nil {
		fmt.Fprintln(stderr, "directive:", err)
		return 2
	}
	io.WriteString(stdout, texte)
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

// marqueV1 est la phrase du refus d'une directive v1, celle du chargeur une
// fois le v1 retiré de internal/directive (session v2-seul).
const marqueV1 = "format de directive v1 non pris en charge"

// formatV1 rend une erreur par directive au format v1 (clé verrou, ou pas de
// déclencheur), désignée par sa position, sans rien citer du document ;
// rien si le document est illisible ou si le chargeur l'a déjà dit.
func formatV1(donnees []byte, deja []string) []string {
	for _, e := range deja {
		if strings.Contains(e, marqueV1) {
			return nil
		}
	}
	var brut struct {
		Directives []map[string]json.RawMessage `json:"directives"`
	}
	if json.Unmarshal(donnees, &brut) != nil {
		return nil
	}
	var out []string
	for i, d := range brut.Directives {
		_, verrou := d["verrou"]
		_, declencheur := d["declencheur"]
		if verrou || !declencheur {
			out = append(out, fmt.Sprintf("directive: directive %d: %s : réécrire en v2", i+1, marqueV1))
		}
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
