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
	b, err := os.ReadFile("../../internal/directive/testdata/v2/credit.json")
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
		{"plusieurs défauts", `{"jeu":{"id":"t","version":1,"date_effet":"2026-10-04T08:00:00Z","auteur":"j"},"directives":[{"id":"D-1","version":1,"statut":"proposee","date_effet":"2026-10-04T08:00:00Z","texte_fr":"t","fondements":[],"declencheur":{"evenement":"appel_outil","cible":{"serveur":"*","outil":"*"}},"effets":[{"effet":"autoriser"}]}]}`,
			[]string{`statut "proposee" inconnu`, "fondements", "autoriser n'est pas un effet"}},
		{"format v1", `{"jeu":{"id":"t","version":1,"date_effet":"2026-10-04T08:00:00Z","auteur":"j"},"directives":[{"id":"D-1","version":1,"statut":"active","date_effet":"2026-10-04T08:00:00Z","texte_fr":"t","fondement":{"texte":"RGPD"},"verrou":"interdiction","cible":{"serveur":"*","outil":"*"},"effet":"refuser"}]}`,
			[]string{"directive 1: format de directive v1 non pris en charge : réécrire en v2"}},
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
	attendu, err := os.ReadFile("../../internal/relecture/testdata/v2/credit.txt")
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

func TestVerifier_ExigerBrouillon(t *testing.T) {
	// Le jeu d'exemple est actif : quatre refus, un par directive.
	rc, out, _ := lancer(t, fixture(t), "verifier", "--exiger-brouillon")
	var r Rapport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	if rc != 1 || r.OK || len(r.Erreurs) != 4 || r.Empreinte != "" {
		t.Fatalf("rc=%d rapport=%+v", rc, r)
	}
	if r.Erreurs[0] != `directive: directive C-001: statut "active", brouillon exigé` {
		t.Errorf("erreur 1 = %q", r.Erreurs[0])
	}

	brouillon := strings.ReplaceAll(fixture(t), `"statut": "active"`, `"statut": "brouillon"`)
	if rc, out, _ := lancer(t, brouillon, "verifier", "--exiger-brouillon"); rc != 0 {
		t.Fatalf("jeu en brouillon refusé : %s", out)
	}
	// Sans l'option, le statut n'est pas contrôlé.
	if rc, _, _ := lancer(t, fixture(t), "verifier"); rc != 0 {
		t.Fatalf("rc = %d sans --exiger-brouillon", rc)
	}
}

// Un jeu que le chargeur refuse voit aussi ses statuts contrôlés : la liste
// reste complète.
func TestVerifier_ExigerBrouillon_JeuInvalide(t *testing.T) {
	doc := `{"jeu":{"id":"t","version":1,"date_effet":"2026-10-04T08:00:00Z","auteur":"j"},"directives":[{"id":"D-1","version":1,"statut":"observation","date_effet":"2026-10-04T08:00:00Z","texte_fr":"t","fondements":[{"texte":"RGPD"}],"declencheur":{"evenement":"appel_outil","cible":{"serveur":"*","outil":"*"}},"effets":[{"effet":"caviarder","champs_autorises":["/siren"],"mode":"caviarder"}]}]}`
	rc, out, _ := lancer(t, doc, "verifier", "--exiger-brouillon")
	var r Rapport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	if rc != 1 || len(r.Erreurs) != 2 || !strings.Contains(r.Erreurs[0], "caviarder") ||
		!strings.Contains(r.Erreurs[1], `statut "observation", brouillon exigé`) {
		t.Fatalf("rc=%d erreurs=%q", rc, r.Erreurs)
	}
}

// Le format v1 est retiré : verifier et relire le refusent, une erreur par
// directive fautive, désignée par sa position, sans rien citer du document.
func TestV1_Refuse(t *testing.T) {
	v2 := `{"id":"D-1","version":1,"statut":"active","date_effet":"2026-10-04T08:00:00Z","texte_fr":"t","fondements":[{"texte":"RGPD"}],"declencheur":{"evenement":"appel_outil","cible":{"serveur":"*","outil":"*"}},"effets":[{"effet":"refuser"}]}`
	v1 := `{"id":"D-2","version":1,"statut":"active","date_effet":"2026-10-04T08:00:00Z","texte_fr":"secret","fondement":{"texte":"RGPD"},"verrou":"interdiction","cible":{"serveur":"*","outil":"*"},"effet":"refuser"}`
	doc := `{"jeu":{"id":"t","version":1,"date_effet":"2026-10-04T08:00:00Z","auteur":"j"},"directives":[` + v2 + `,` + v1 + `]}`
	rc, out, _ := lancer(t, doc, "verifier")
	r := Rapport{}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	if rc != 1 || r.OK || r.Empreinte != "" {
		t.Fatalf("rc=%d rapport=%+v", rc, r)
	}
	n := 0
	for _, e := range r.Erreurs {
		if strings.Contains(e, "format de directive v1") {
			n++
			if e != "directive: directive 2: format de directive v1 non pris en charge : réécrire en v2" {
				t.Errorf("erreur %q", e)
			}
		}
	}
	if n != 1 {
		t.Fatalf("une erreur v1 attendue : %q", r.Erreurs)
	}
	rc, out, errOut := lancer(t, doc, "relire")
	if rc != 1 || out != "" || !strings.Contains(errOut, "directive 2: format de directive v1") || strings.Contains(errOut, "secret") {
		t.Fatalf("relire : rc=%d out=%q err=%q", rc, out, errOut)
	}
}

func TestUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"compiler"}, {"verifier", "x"}, {"verifier", "--inconnu"},
		{"relire", "--exiger-brouillon"}} {
		if rc, _, errOut := lancer(t, "{}", args...); rc != 2 || !strings.Contains(errOut, "usage") {
			t.Errorf("args %v : rc=%d err=%q", args, rc, errOut)
		}
	}
}
