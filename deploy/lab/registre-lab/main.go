// Commande registre-lab écrit le registre du lab : un seul serveur,
// banque-synthetique, joint en HTTP streamable + mTLS SPIFFE, avec les outils
// épinglés tirés de son schema.json et l'identité SPIFFE attendue du serveur.
//
//	go run ./deploy/lab/registre-lab -url https://banque-synthetique.mcp-lab.svc.cluster.local:8443/mcp \
//	    -spiffe-id spiffe://mcp.gcp.lab/banque-synthetique -sortie registre.json
package main

import (
	"flag"
	"fmt"
	"os"

	"mcprogue/internal/registre"
	"mcprogue/internal/registredemo"
)

func main() {
	schema := flag.String("schema", "servers/banque-synthetique/schema.json", "tools/list de banque-synthetique")
	url := flag.String("url", "", "url https du serveur")
	id := flag.String("spiffe-id", "", "identité SPIFFE attendue du serveur")
	sortie := flag.String("sortie", "-", "fichier de sortie (- : stdout)")
	flag.Parse()
	b, err := Generer(*schema, *url, *id)
	if err == nil && *sortie == "-" {
		_, err = os.Stdout.Write(b)
	} else if err == nil {
		err = os.WriteFile(*sortie, b, 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "registre-lab:", err)
		os.Exit(1)
	}
}

// Generer rend le registre du lab, validé.
func Generer(schema, url, id string) ([]byte, error) {
	brut, err := os.ReadFile(schema)
	if err != nil {
		return nil, err
	}
	outils, err := registredemo.Convertir(brut)
	if err != nil {
		return nil, err
	}
	r := &registre.Registre{
		Meta: registre.MetaRegistre{ID: "lab", Version: 1, Auteur: "ops@exemple.test"},
		Serveurs: []registre.Serveur{{
			Nom: "banque-synthetique", Transport: registre.HTTPSpiffe, URL: url,
			Attestation: registre.Attestation{Mode: registre.ModeSpiffe, Attendu: id},
			Outils:      outils,
		}},
	}
	if err := registre.Valider(r); err != nil {
		return nil, err
	}
	return registredemo.Encoder(r)
}
