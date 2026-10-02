package relecture

import (
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"mcprogue/internal/directive"
)

var maj = flag.Bool("update", false, "réécrire les fichiers attendus de testdata")

func charger(t *testing.T, chemin string) *directive.Jeu {
	t.Helper()
	b, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatal(err)
	}
	jeu, err := directive.Charger(b)
	if err != nil {
		t.Fatal(err)
	}
	return jeu
}

func TestStatutEtFondement(t *testing.T) {
	statuts := map[directive.Statut]string{
		directive.Brouillon:   "brouillon (non appliquée tant que le juriste ne l'a pas passée en observation ou active)",
		directive.Observation: "en observation (journalisée, non appliquée)",
		directive.Active:      "active (appliquée)",
		directive.Retiree:     "retirée (non appliquée)",
	}
	for s, veut := range statuts {
		if got := statut(s); got != veut {
			t.Errorf("statut(%s) = %q, attendu %q", s, got, veut)
		}
	}
	fondements := []struct {
		f    directive.Fondement
		veut string
	}{
		{directive.Fondement{Texte: "RGPD"}, "RGPD"},
		{directive.Fondement{Texte: "RGPD", Article: "22"}, "l'art. 22 RGPD"},
		{directive.Fondement{Texte: "RGPD", Article: "art. 22 §1"}, "l'art. 22 §1 RGPD"},
		{directive.Fondement{Texte: "Code civil", Article: "1240", URL: "https://x"}, "l'art. 1240 Code civil (source : https://x)"},
	}
	for _, c := range fondements {
		if got := fondement(c.f); got != c.veut {
			t.Errorf("fondement(%+v) = %q, attendu %q", c.f, got, c.veut)
		}
	}
}

func TestDate_UTC(t *testing.T) {
	paris := time.FixedZone("CEST", 2*3600)
	cases := []struct {
		t    time.Time
		veut string
	}{
		{time.Date(2026, 10, 4, 10, 0, 0, 0, paris), "04/10/2026 à 08:00 UTC"},
		{time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), "01/01/2027 à 00:00 UTC"},
		{time.Date(2027, 1, 1, 0, 0, 7, 0, time.UTC), "01/01/2027 à 00:00:07 UTC"},
	}
	for _, c := range cases {
		if got := date(c.t); got != c.veut {
			t.Errorf("date(%v) = %q, attendu %q", c.t, got, c.veut)
		}
	}
	if !strings.Contains(revision(&cases[1].t), "reste appliquée") {
		t.Error("l'échéance de relecture doit dire que la directive reste appliquée")
	}
}
