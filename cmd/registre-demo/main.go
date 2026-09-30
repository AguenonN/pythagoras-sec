// Commande registre-demo écrit le registre de démonstration et le
// registre d'attaque depuis le manifeste de construction des serveurs.
//
//	scripts/construire-serveurs.sh      # bin/ et bin/manifeste.json
//	go run ./cmd/registre-demo          # demo/registre.json, demo/registre-attaque.json
//
// Les deux fichiers portent des chemins absolus propres à la machine et
// les empreintes des binaires construits : ils sont régénérés, jamais
// versionnés. Deux exécutions sur les mêmes binaires écrivent les mêmes
// octets. Voir internal/registredemo.
//
// Avec -catalogue, écrit aussi le catalogue du compilateur de directives
// dérivé du registre (serveurs, outils, arguments en pointeurs JSON). Lui
// ne dépend que des schémas épinglés : il est versionné, et
// internal/registredemo vérifie qu'il ne diverge pas du registre.
//
//	go run ./cmd/registre-demo -catalogue agents/compilateur/exemples/catalogue-demo.json
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"mcprogue/internal/registredemo"
)

func main() {
	if err := lancer(); err != nil {
		fmt.Fprintln(os.Stderr, "registre-demo:", err)
		os.Exit(1)
	}
}

func lancer() error {
	racine := flag.String("racine", ".", "racine du dépôt")
	manifeste := flag.String("manifeste", "bin/manifeste.json", "manifeste de construction, relatif à la racine")
	sortie := flag.String("sortie", "demo", "répertoire des registres, relatif à la racine")
	gels := flag.String("gels", registredemo.DossierGelsLocal, "enregistrements de gels-avoirs, relatif à la racine")
	enr := flag.String("enregistrements", "", "répertoire des copies de travail des enregistrements (<dir>/<serveur>), relatif à la racine ; vide : les enregistrements versionnés")
	catalogue := flag.String("catalogue", "", "écrit aussi le catalogue du compilateur à ce chemin, relatif à la racine")
	flag.Parse()
	if flag.NArg() > 0 {
		return errors.New("arguments inattendus")
	}
	r, err := filepath.Abs(*racine)
	if err != nil {
		return err
	}
	m, err := registredemo.LireManifeste(filepath.Join(r, *manifeste))
	if err != nil {
		return err
	}
	demo, attaque, err := registredemo.Generer(m, registredemo.Options{Racine: r, Gels: *gels, Enregistrements: *enr})
	if err != nil {
		return err
	}
	dir := filepath.Join(r, *sortie)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := registredemo.Encoder(demo)
	if err != nil {
		return err
	}
	if err := ecrire(filepath.Join(dir, "registre.json"), b); err != nil {
		return err
	}
	if b, err = registredemo.Encoder(attaque); err != nil {
		return err
	}
	if err := ecrire(filepath.Join(dir, "registre-attaque.json"), b); err != nil {
		return err
	}
	if *catalogue != "" {
		if b, err = registredemo.EncoderCatalogue(registredemo.CatalogueDepuis(demo)); err != nil {
			return err
		}
		if err := ecrire(filepath.Join(r, *catalogue), b); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "registre-demo: %s (%s), %s (%s)\n",
		filepath.Join(*sortie, "registre.json"), demo.Meta.Empreinte,
		filepath.Join(*sortie, "registre-attaque.json"), attaque.Meta.Empreinte)
	return nil
}

// ecrire remplace le fichier d'un coup : écrit à côté, puis renommé.
func ecrire(chemin string, b []byte) error {
	tmp := chemin + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, chemin)
}
