package simulateur

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"mcprogue/internal/annuaire"
)

// ReglesExemple sont des règles de démonstration à huit directives (la table
// des contrats §1, version 1), pour les tests et la mise au point de la
// console. Les vraies règles sont demo/directives/demo.json, tenues par la
// session « démo réelle ».
//
//go:embed regles_exemple.json
var ReglesExemple []byte

//go:embed annuaire_exemple.json
var annuaireExemple []byte

// PropositionExemple est une proposition de l'Analyste (contrats §6), écrite
// par PreparerDemo pour la mise au point de l'écran de rédaction.
//
//go:embed proposition_exemple.json
var PropositionExemple []byte

// PreparerDemo écrit sous racine/demo une démonstration de mise au point :
// règles (version 1), annuaire signé et clés (identités fictives, rôles de
// la démonstration), dossiers d'exécution. Rien n'est écrit si les règles
// existent déjà.
func PreparerDemo(racine, demo string) error {
	d := filepath.Join(racine, demo)
	if _, err := os.Stat(filepath.Join(d, "directives", "demo.json")); err == nil {
		return fmt.Errorf("%s existe déjà : rien n'est écrit", filepath.Join(d, "directives", "demo.json"))
	}
	for _, s := range []string{"directives", "execution/sessions", "execution/propositions"} {
		if err := os.MkdirAll(filepath.Join(d, s), 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(d, "directives", "demo.json"), ReglesExemple, 0o644); err != nil {
		return err
	}
	var src annuaire.SourceDemo
	if err := jsonStrict(annuaireExemple, &src); err != nil {
		return err
	}
	doc, cles, err := annuaire.Generer(src, time.Now(), nil)
	if err != nil {
		return err
	}
	b, err := annuaire.Encoder(doc)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(d, "annuaire.json"), b, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(d, "execution", "propositions", "P-2026-10-02-1.json"), PropositionExemple, 0o644); err != nil {
		return err
	}
	return annuaire.EcrireCles(filepath.Join(d, "execution", "cles"), cles)
}
