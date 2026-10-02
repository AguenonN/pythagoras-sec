package main

// Sous-commandes de l'annuaire des habilitations (internal/annuaire) :
//
//	mcp-approve annuaire-demo [-source demo/annuaire-source.json] \
//	    [-sortie demo/annuaire.json] [-cles demo/execution/cles]
//	mcp-approve identite -cles DIR -signataire ID [-demandeur ID] [-duree 8h] [-horloge RFC3339]
//	mcp-approve valider  -cles DIR -valideur ID -role ROLE -empreinte HEX [-duree 10m] [-horloge RFC3339]
//
// annuaire-demo tire une clé par identité fictive et une pour l'autorité,
// écrit l'annuaire signé et les clés (0600, répertoire ignoré par Git).
// identite signe le jeton d'identité du demandeur, à passer à mcp-gate
// (-demandeur-jeton ou MCP_GATE_JETON_DEMANDEUR) ; valider signe le jeton
// d'approbation d'un appel en attente, que l'agent passe à la reprise dans
// _meta.approvalToken. Les jetons sont écrits sur stdout, seuls.
//
// -horloge signe à l'instant simulé donné (iat), celui de mcp-gate
// -horloge-depart : le gate, parti de cet instant, accepte le jeton. Sans
// -horloge, l'heure réelle. Le jeton porte alors une heure qui n'est pas
// celle de la machine : démonstration et tests seulement.

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

// instantSignature lit la valeur de -horloge : l'instant simulé, ou
// maintenant si elle est vide.
func instantSignature(horloge string, maintenant func() time.Time) (time.Time, error) {
	if horloge == "" {
		return maintenant(), nil
	}
	t, err := time.Parse(time.RFC3339Nano, horloge)
	if err != nil {
		return time.Time{}, fmt.Errorf("-horloge : %w", err)
	}
	return t, nil
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
	horloge := fs.String("horloge", "", "démonstration et tests : signe à cet instant simulé (RFC 3339), cohérent avec mcp-gate -horloge-depart")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *signataire == "" {
		return errors.New("-signataire obligatoire")
	}
	emis, err := instantSignature(*horloge, time.Now)
	if err != nil {
		return err
	}
	if *demandeur == "" {
		*demandeur = *signataire
	}
	cle, err := annuaire.LireCle(filepath.Join(*cles, annuaire.FichierCle(*signataire)))
	if err != nil {
		return err
	}
	j, err := annuaire.SignerIdentite(cle, *signataire, *demandeur, emis, *duree)
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
	horloge := fs.String("horloge", "", "démonstration et tests : signe à cet instant simulé (RFC 3339), cohérent avec mcp-gate -horloge-depart")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *valideur == "" || *role == "" || *empreinte == "" {
		return errors.New("-valideur, -role et -empreinte obligatoires")
	}
	emis, err := instantSignature(*horloge, time.Now)
	if err != nil {
		return err
	}
	cle, err := annuaire.LireCle(filepath.Join(*cles, annuaire.FichierCle(*valideur)))
	if err != nil {
		return err
	}
	j, err := annuaire.SignerApprobation(cle, *valideur, *role, *empreinte, emis, *duree)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, j)
	return nil
}
