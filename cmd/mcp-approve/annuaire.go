package main

// Sous-commandes de l'annuaire des habilitations (internal/annuaire) :
//
//	mcp-approve annuaire-demo [-source demo/annuaire-source.json] \
//	    [-sortie demo/annuaire.json] [-cles demo/execution/cles]
//	mcp-approve identite -cles DIR -signataire ID [-demandeur ID] [-duree 8h]
//	mcp-approve valider  -cles DIR -valideur ID -role ROLE -empreinte HEX [-duree 10m]
//
// annuaire-demo tire une clé par identité fictive et une pour l'autorité,
// écrit l'annuaire signé et les clés (0600, répertoire ignoré par Git).
// identite signe le jeton d'identité du demandeur, à passer à mcp-gate
// (-demandeur-jeton ou MCP_GATE_JETON_DEMANDEUR) ; valider signe le jeton
// d'approbation d'un appel en attente, que l'agent passe à la reprise dans
// _meta.approvalToken. Les jetons sont écrits sur stdout, seuls.

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"mcprogue/internal/annuaire"
)

var sousCommandes = map[string]func(args []string, out io.Writer) error{
	"annuaire-demo": annuaireDemo,
	"identite":      identite,
	"valider":       valider,
}

func annuaireDemo(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("annuaire-demo", flag.ContinueOnError)
	source := fs.String("source", annuaire.FichierSourceDemo, "source sans clé (identités fictives)")
	sortie := fs.String("sortie", "demo/annuaire.json", "annuaire signé")
	cles := fs.String("cles", "demo/execution/cles", "répertoire des clés privées (créé en 0700)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	src, err := annuaire.LireSourceDemo(*source)
	if err != nil {
		return err
	}
	d, c, err := annuaire.Generer(*src, time.Now(), nil)
	if err != nil {
		return err
	}
	b, err := annuaire.Encoder(d)
	if err != nil {
		return err
	}
	if err := annuaire.EcrireCles(*cles, c); err != nil {
		return err
	}
	if err := os.WriteFile(*sortie, b, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "annuaire: %s (%s)\nautorité: %s\nclés: %s\n", *sortie, d.Meta.Empreinte,
		filepath.Join(*cles, annuaire.FichierAutoritePublic), *cles)
	return nil
}

func identite(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("identite", flag.ContinueOnError)
	cles := fs.String("cles", "demo/execution/cles", "répertoire des clés")
	signataire := fs.String("signataire", "", "identité qui signe (le demandeur, ou un émetteur d'identité)")
	demandeur := fs.String("demandeur", "", "demandeur (défaut : le signataire)")
	duree := fs.Duration("duree", 8*time.Hour, "durée de vie (12h au plus)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *signataire == "" {
		return errors.New("-signataire obligatoire")
	}
	if *demandeur == "" {
		*demandeur = *signataire
	}
	cle, err := annuaire.LireCle(filepath.Join(*cles, annuaire.FichierCle(*signataire)))
	if err != nil {
		return err
	}
	j, err := annuaire.SignerIdentite(cle, *signataire, *demandeur, time.Now(), *duree)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, j)
	return nil
}

func valider(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("valider", flag.ContinueOnError)
	cles := fs.String("cles", "demo/execution/cles", "répertoire des clés")
	valideur := fs.String("valideur", "", "identité du valideur")
	role := fs.String("role", "", "rôle au titre duquel il valide")
	empreinte := fs.String("empreinte", "", "empreinte_transmise de l'appel en attente")
	duree := fs.Duration("duree", 10*time.Minute, "durée de vie (15m au plus)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *valideur == "" || *role == "" || *empreinte == "" {
		return errors.New("-valideur, -role et -empreinte obligatoires")
	}
	cle, err := annuaire.LireCle(filepath.Join(*cles, annuaire.FichierCle(*valideur)))
	if err != nil {
		return err
	}
	j, err := annuaire.SignerApprobation(cle, *valideur, *role, *empreinte, time.Now(), *duree)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, j)
	return nil
}
