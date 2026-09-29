package nommage

import (
	"strings"
	"testing"
)

func TestValiderNomLogique(t *testing.T) {
	ok := []string{"taux-officiel", "banque-synthetique", "a", "x1", "recherche-entreprises"}
	for _, n := range ok {
		if err := ValiderNomLogique(n); err != nil {
			t.Errorf("%q refusé: %v", n, err)
		}
	}
	bad := map[string]string{
		"":                      "vide",
		"taux__officiel":        "séparateur",
		"taux_officiel":         "invalide",
		"Taux":                  "invalide",
		"-taux":                 "invalide",
		"taux-":                 "invalide",
		"taux officiel":         "invalide",
		strings.Repeat("a", 62): "trop long",
	}
	for n, want := range bad {
		err := ValiderNomLogique(n)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: attendu une erreur contenant %q, obtenu %v", n, want, err)
		}
	}
}

func TestExposerDecomposer(t *testing.T) {
	expose, err := Exposer("taux-officiel", "get__taux")
	if err != nil {
		t.Fatal(err)
	}
	if expose != "taux-officiel__get__taux" {
		t.Fatalf("exposé %q", expose)
	}
	s, o, err := Decomposer(expose)
	if err != nil {
		t.Fatal(err)
	}
	if s != "taux-officiel" || o != "get__taux" {
		t.Fatalf("décomposé en (%q, %q)", s, o)
	}
	if _, _, err := Decomposer("sans-separateur"); err == nil {
		t.Fatal("un nom sans séparateur doit être refusé")
	}
	if _, err := Exposer("srv", strings.Repeat("o", 62)); err == nil {
		t.Fatal("un nom exposé au-delà de LongueurMax doit être refusé")
	}
}
