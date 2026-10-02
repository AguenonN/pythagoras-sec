package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"mcprogue/internal/annuaire"
	"mcprogue/internal/registredemo"
)

const racineDepot = "../.."

// Le vocabulaire versionné est celui que ses sources donnent, octet pour
// octet. S'il échoue : go run ./cmd/registre-demo -vocabulaire
// agents/compilateur/exemples/vocabulaire-demo.json (les enregistrements
// Mistral pris sur l'ancien vocabulaire gardent leur copie gelée).
func TestVocabulaireVersionneConformeAuxSources(t *testing.T) {
	v, err := GenererVocabulaire(racineDepot)
	if err != nil {
		t.Fatal(err)
	}
	attendu, err := EncoderVocabulaire(v)
	if err != nil {
		t.Fatal(err)
	}
	versionne, err := os.ReadFile(filepath.Join(racineDepot, cheminVocabulaire))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(attendu, versionne) {
		t.Fatalf("%s diverge de ses sources ; régénérer avec go run ./cmd/registre-demo -vocabulaire %s",
			cheminVocabulaire, cheminVocabulaire)
	}
}

func TestVocabulaireContenu(t *testing.T) {
	v, err := GenererVocabulaire(racineDepot)
	if err != nil {
		t.Fatal(err)
	}
	noms := func(l []Nom) map[string]string {
		m := map[string]string{}
		for _, n := range l {
			m[n.Nom] = n.Description
		}
		return m
	}
	et, roles, groupes := noms(v.Etiquettes), noms(v.Roles), noms(v.Groupes)
	for _, e := range []string{"donnees_client", "donnees_personnelles", "production_rapport"} {
		if _, ok := et[e]; !ok {
			t.Errorf("étiquette %s absente", e)
		}
	}
	// Les rôles que citent les directives du jeu de démo (contrats §1).
	for _, r := range []string{"analyste-risques", "directeur-risques", "responsable-rh", "direction-juridique"} {
		if _, ok := roles[r]; !ok {
			t.Errorf("rôle %s absent", r)
		}
	}
	for _, g := range []string{"prestataires-externes", "salaries-proteges", "contentieux-en-cours"} {
		if _, ok := groupes[g]; !ok {
			t.Errorf("groupe %s absent", g)
		}
	}
	if d := et["production_rapport"]; d != "étiquette du registre, portée par : outil remettre_rapport (tout serveur)" {
		t.Errorf("description de production_rapport : %q", d)
	}
}

func TestVocabulaireDivergenceDetectee(t *testing.T) {
	et, err := registredemo.LireEtiquettes(filepath.Join(racineDepot, registredemo.FichierEtiquettes))
	if err != nil {
		t.Fatal(err)
	}
	an, err := annuaire.LireSourceDemo(filepath.Join(racineDepot, fichierAnnuaire))
	if err != nil {
		t.Fatal(err)
	}
	avant, err := vocabulaireDepuis(et, an, groupesRH{})
	if err != nil {
		t.Fatal(err)
	}
	an.Identites = append(an.Identites, annuaire.IdentiteSource{Identite: "x@exemple.test",
		Roles: []annuaire.Role{{Role: "role-ajoute"}}})
	apres, err := vocabulaireDepuis(et, an, groupesRH{})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := EncoderVocabulaire(avant)
	b, _ := EncoderVocabulaire(apres)
	if bytes.Equal(a, b) {
		t.Fatal("un rôle ajouté à l'annuaire ne change pas le vocabulaire")
	}
}

func TestVocabulaireSansManifeste(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{registredemo.FichierEtiquettes, fichierAnnuaire, fichierGroupesRH} {
		b, err := os.ReadFile(filepath.Join(racineDepot, f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	v, err := GenererVocabulaire(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Roles) == 0 || len(v.Groupes) == 0 || len(v.Etiquettes) == 0 {
		t.Fatalf("vocabulaire vide : %+v", v)
	}
}
