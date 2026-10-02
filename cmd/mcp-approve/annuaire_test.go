package main

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mcprogue/internal/annuaire"
)

const (
	departSimule  = "2026-09-15T22:00:00Z"
	valideurTest  = "diane.risques@exemple.test"
	empreinteTest = "sha256:abc"
)

// annuaireSimule : annuaire de démo émis avant l'instant simulé, clés écrites
// dans un répertoire temporaire.
func annuaireSimule(t *testing.T) (string, *annuaire.Local) {
	t.Helper()
	src, err := annuaire.LireSourceDemo(filepath.Join("..", "..", annuaire.FichierSourceDemo))
	if err != nil {
		t.Fatal(err)
	}
	d, cles, err := annuaire.Generer(*src, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := annuaire.Encoder(d)
	if err != nil {
		t.Fatal(err)
	}
	l, err := annuaire.Charger(b, cles.Autorite.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "cles")
	if err := annuaire.EcrireCles(dir, cles); err != nil {
		t.Fatal(err)
	}
	return dir, l
}

func signer(t *testing.T, f func([]string, *bytes.Buffer) error, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := f(args, &out); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}

// Un jeton signé à l'heure simulée est accepté par un gate dont l'horloge
// part du même instant (et avance) ; signé à l'heure réelle, il est périmé.
func TestHorloge_JetonsALHeureSimulee(t *testing.T) {
	dir, ann := annuaireSimule(t)
	depart, _ := time.Parse(time.RFC3339, departSimule)
	// l'horloge du gate, un instant après son départ
	gate := depart.Add(90 * time.Second)

	id := signer(t, func(a []string, o *bytes.Buffer) error { return identite(a, o) },
		"-cles", dir, "-signataire", "claire.analyste@exemple.test", "-horloge", departSimule)
	ident, err := annuaire.VerifierIdentite(ann, id, gate)
	if err != nil {
		t.Fatalf("identité à l'heure simulée refusée : %v", err)
	}
	if !ident.Emis.Equal(depart) {
		t.Fatalf("iat = %v, attendu %v", ident.Emis, depart)
	}

	ap := signer(t, func(a []string, o *bytes.Buffer) error { return valider(a, o) },
		"-cles", dir, "-valideur", valideurTest, "-role", "directeur-risques",
		"-empreinte", empreinteTest, "-horloge", departSimule)
	if _, err := annuaire.VerifierApprobation(ann, ap, empreinteTest, gate); err != nil {
		t.Fatalf("approbation à l'heure simulée refusée : %v", err)
	}

	// sans -horloge : heure réelle, postérieure au départ simulé : iat dans le futur du gate
	reel := signer(t, func(a []string, o *bytes.Buffer) error { return identite(a, o) },
		"-cles", dir, "-signataire", "claire.analyste@exemple.test")
	if _, err := annuaire.VerifierIdentite(ann, reel, gate); !errors.Is(err, annuaire.ErrJetonPerime) {
		t.Fatalf("jeton à l'heure réelle : %v", err)
	}
}

func TestHorloge_Illisible(t *testing.T) {
	dir, _ := annuaireSimule(t)
	var out bytes.Buffer
	if err := identite([]string{"-cles", dir, "-signataire", "x", "-horloge", "demain"}, &out); err == nil {
		t.Fatal("horloge illisible acceptée (identite)")
	}
	if err := valider([]string{"-cles", dir, "-valideur", "x", "-role", "r", "-empreinte", "e", "-horloge", "demain"}, &out); err == nil {
		t.Fatal("horloge illisible acceptée (valider)")
	}
	if out.Len() != 0 {
		t.Fatalf("sortie malgré l'erreur : %q", out.String())
	}
}
