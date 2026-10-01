package main

import (
	"strings"
	"testing"
	"time"
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

func TestHorlogeSimulee(t *testing.T) {
	reel := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	h, err := horlogeSimulee("2026-10-02T22:00:00Z", func() time.Time { return reel })
	if err != nil || h().Format(time.RFC3339) != "2026-10-02T22:00:00Z" {
		t.Fatal(err)
	}
	reel = reel.Add(90 * time.Second)
	if h().Format(time.RFC3339) != "2026-10-02T22:01:30Z" {
		t.Fatalf("%v", h())
	}
	if h, err := horlogeSimulee("", time.Now); h != nil || err != nil {
		t.Fatal("sans départ : horloge réelle")
	}
	if _, err := horlogeSimulee("demain", time.Now); err == nil {
		t.Fatal("départ illisible accepté")
	}
}
