package main

import (
	"strings"
	"testing"
)

func TestLireCle(t *testing.T) {
	cas := []struct{ valeur, veut string }{
		{"", "absente"},
		{"zz", "hexadécimal"},
		{strings.Repeat("ab", 16), "trop courte"},
	}
	for _, c := range cas {
		t.Setenv(VarCle, c.valeur)
		if _, err := lireCle(); err == nil || !strings.Contains(err.Error(), c.veut) {
			t.Errorf("%q : %v", c.veut, err)
		} else if c.valeur != "" && strings.Contains(err.Error(), c.valeur) {
			t.Errorf("l'erreur ne doit jamais citer la clé : %v", err)
		}
	}
	t.Setenv(VarCle, strings.Repeat("ab", 32))
	if cle, err := lireCle(); err != nil || len(cle) != 32 {
		t.Fatalf("%d %v", len(cle), err)
	}
}
