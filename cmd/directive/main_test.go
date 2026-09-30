package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func lancer(t *testing.T, entree string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	rc := run(args, strings.NewReader(entree), &out, &errOut)
	return rc, out.String(), errOut.String()
}

func fixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../internal/directive/testdata/credit-pme.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestVerifier_OK(t *testing.T) {
	rc, out, _ := lancer(t, fixture(t), "verifier")
	if rc != 0 {
		t.Fatalf("rc = %d, sortie %s", rc, out)
	}
	var r Rapport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	if !r.OK || !strings.HasPrefix(r.Empreinte, "sha256:") || len(r.Erreurs) != 0 {
		t.Fatalf("rapport inattendu : %+v", r)
	}
}

// Toutes les erreurs sont rendues, une par entrée, pas seulement la première.
func TestVerifier_ToutesLesErreurs(t *testing.T) {
	cases := []struct {
		nom, entree string
		veut        []string
	}{
		{"illisible", `{"jeu":`, []string{"document illisible"}},
		{"champ inconnu", `{"jeu":{},"directives":[],"bonus":1}`, []string{"unknown field"}},
		{"plusieurs défauts", `{"jeu":{"id":"t","version":1,"date_effet":"2026-10-04T08:00:00Z","auteur":"j"},"directives":[{"id":"D-1","version":1,"statut":"proposee","date_effet":"2026-10-04T08:00:00Z","texte_fr":"t","fondement":{"texte":"RGPD"},"verrou":"interdiction","cible":{"serveur":"","outil":"*"},"effet":"attendre_validation"}]}`,
			[]string{`statut "proposee" inconnu`, "cible.serveur", `effet "attendre_validation" non admis`}},
	}
	for _, c := range cases {
		t.Run(c.nom, func(t *testing.T) {
			rc, out, _ := lancer(t, c.entree, "verifier")
			if rc != 1 {
				t.Fatalf("rc = %d, attendu 1", rc)
			}
			var r Rapport
			if err := json.Unmarshal([]byte(out), &r); err != nil {
				t.Fatalf("sortie non JSON : %v\n%s", err, out)
			}
			if r.OK || len(r.Erreurs) != len(c.veut) {
				t.Fatalf("attendu %d erreurs, obtenu %+v", len(c.veut), r)
			}
			for i, v := range c.veut {
				if !strings.Contains(r.Erreurs[i], v) {
					t.Errorf("erreur %d = %q, attendu %q", i, r.Erreurs[i], v)
				}
			}
		})
	}
}

func TestRelire(t *testing.T) {
	rc, out, _ := lancer(t, fixture(t), "relire")
	if rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	attendu, err := os.ReadFile("../../internal/relecture/testdata/credit-pme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if out != string(attendu) {
		t.Fatalf("relecture différente de l'attendu :\n%s", out)
	}
	for i := 0; i < 3; i++ {
		if _, again, _ := lancer(t, fixture(t), "relire"); again != out {
			t.Fatal("relecture non déterministe")
		}
	}
}

func TestRelire_JeuRefuse(t *testing.T) {
	rc, out, errOut := lancer(t, `{"jeu":{},"directives":[]}`, "relire")
	if rc != 1 || out != "" || !strings.Contains(errOut, "jeu refusé") {
		t.Fatalf("rc=%d out=%q err=%q", rc, out, errOut)
	}
}

func TestUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"compiler"}, {"verifier", "x"}} {
		if rc, _, errOut := lancer(t, "{}", args...); rc != 2 || !strings.Contains(errOut, "usage") {
			t.Errorf("args %v : rc=%d err=%q", args, rc, errOut)
		}
	}
}
