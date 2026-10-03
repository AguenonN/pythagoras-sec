package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const racine = "../../.."

func generer(t *testing.T) map[string][]byte {
	t.Helper()
	ref, err := os.ReadFile(filepath.Join(racine, Referentiel))
	if err != nil {
		t.Fatal(err)
	}
	f, err := Generer(ref)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// Même graine ⇒ même sortie, octet pour octet, et ce qui est versionné est
// cette sortie.
func TestDeterministeEtVersionne(t *testing.T) {
	a, b := generer(t), generer(t)
	for k := range a {
		if !bytes.Equal(a[k], b[k]) {
			t.Errorf("%s : deux générations diffèrent", k)
		}
		v, err := os.ReadFile(filepath.Join(racine, k))
		if err != nil || !bytes.Equal(v, a[k]) {
			t.Errorf("%s diffère de la génération ; régénérer : go run ./servers/banque-synthetique/population", k)
		}
	}
}

var (
	reEmail = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+`)
	reIBAN  = regexp.MustCompile(`FR76[0-9 ]{23,}`)
	reTel   = regexp.MustCompile(`"telephone": "([^"]*)"`)
)

// Aucun e-mail hors exemple.test, tout IBAN a une clé invalide, tout
// téléphone en 01 99 00 : dans la population et dans les clients écrits à
// la main.
func TestCredibiliteSansDonneeReelle(t *testing.T) {
	fichiers := []string{SortieClients, SortieSalaries, "servers/banque-synthetique/clients_fictifs.json",
		"servers/rh-synthetique/salaries_fictifs.json"}
	n := 0
	for _, f := range fichiers {
		b, err := os.ReadFile(filepath.Join(racine, f))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range reEmail.FindAllString(string(b), -1) {
			if !strings.HasSuffix(e, "exemple.test") {
				t.Errorf("%s : e-mail %s hors exemple.test", f, e)
			}
		}
		for _, i := range reIBAN.FindAllString(string(b), -1) {
			n++
			if ibanValide(strings.TrimSpace(i)) {
				t.Errorf("%s : IBAN %s à clé valide", f, i)
			}
		}
		for _, m := range reTel.FindAllStringSubmatch(string(b), -1) {
			if !strings.HasPrefix(m[1], "01 99 00 ") {
				t.Errorf("%s : téléphone %s", f, m[1])
			}
		}
	}
	if n < 100 {
		t.Fatalf("%d IBAN seulement", n)
	}
	if !ibanValide("FR76 3000 6000 0112 3456 7890 189") {
		t.Fatal("contrôle de clé IBAN faux sur un IBAN d'exemple valide")
	}
}

func TestPopulation(t *testing.T) {
	f := generer(t)
	var c struct{ Clients []client }
	var s struct{ Salaries []salarie }
	if json.Unmarshal(f[SortieClients], &c) != nil || json.Unmarshal(f[SortieSalaries], &s) != nil {
		t.Fatal("illisible")
	}
	if len(c.Clients) != 70 || len(s.Salaries) != 50 {
		t.Fatalf("%d clients, %d salariés", len(c.Clients), len(s.Salaries))
	}
	if c.Clients[0].ClientID != "CLI-FICTIF-007" || s.Salaries[0].SalarieID != "SAL-FICTIF-031" {
		t.Fatal("les nouveaux enregistrements viennent à la suite des existants")
	}
	ppe, pros := 0, 0
	for _, x := range c.Clients {
		if x.PPE != nil {
			ppe++
		}
		if x.Type == "professionnel" {
			pros++
			if luhn(x.Siren) || len(x.Siren) != 9 {
				t.Errorf("%s : SIREN %s plausible", x.ClientID, x.Siren)
			}
		}
		if len(x.Operations) < 20 {
			t.Errorf("%s : %d opérations", x.ClientID, len(x.Operations))
		}
	}
	if ppe != 6 || pros == 0 {
		t.Fatalf("%d PPE, %d professionnels", ppe, pros)
	}
	plantes := map[string]bool{}
	for _, x := range c.Clients {
		for _, p := range x.CasPlantes {
			plantes[strings.SplitN(p, " ", 2)[0]] = true
		}
	}
	for _, x := range s.Salaries {
		for _, p := range x.CasPlantes {
			plantes[strings.SplitN(p, " ", 2)[0]] = true
		}
	}
	for _, k := range []string{"soupçon", "gel", "découvert", "clôture", "personne", "plus", "congé", "arrêt", "mandat", "sanction", "A5"} {
		if !plantes[k] {
			t.Errorf("cas planté absent : %s", k)
		}
	}
	// Tout client, compte et salarié est au référentiel (résolution des
	// personnes concernées par le gate).
	var r struct{ Appartenances map[string][]string }
	if err := json.Unmarshal(f[Referentiel], &r); err != nil {
		t.Fatal(err)
	}
	for _, x := range c.Clients {
		if _, ok := r.Appartenances[x.ClientID]; !ok {
			t.Errorf("%s absent du référentiel", x.ClientID)
		}
		for _, k := range x.Comptes {
			if g, ok := r.Appartenances[k.CompteID]; !ok || (x.PPE != nil) != (len(g) == 1 && g[0] == GroupePPE) {
				t.Errorf("%s : groupes %v", k.CompteID, g)
			}
		}
	}
}
