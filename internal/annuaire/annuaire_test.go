package annuaire

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)

func monter(t *testing.T) (*Local, *Cles, []byte) {
	t.Helper()
	src, err := LireSourceDemo(filepath.Join("..", "..", FichierSourceDemo))
	if err != nil {
		t.Fatal(err)
	}
	d, cles, err := Generer(*src, t0, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encoder(d)
	if err != nil {
		t.Fatal(err)
	}
	l, err := Charger(b, cles.Autorite.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	return l, cles, b
}

func TestCharger_SignatureEtContenu(t *testing.T) {
	l, cles, b := monter(t)
	if l.Nom() != SourceLocaleDemo || l.Reference().ID != "annuaire-demo" || l.Reference().Version != 1 ||
		!strings.HasPrefix(l.Reference().Empreinte, "sha256:") {
		t.Fatalf("référence %+v", l.Reference())
	}
	// Une autre autorité : refusé.
	pub, _, _ := ed25519.GenerateKey(nil)
	if _, err := Charger(b, pub); !errors.Is(err, ErrSignature) {
		t.Fatalf("autre autorité : %v", err)
	}
	// Contenu modifié après signature : l'empreinte ne correspond plus.
	modifie := strings.Replace(string(b), "analystes-credit", "direction-risques", 1)
	if _, err := Charger([]byte(modifie), cles.Autorite.Public().(ed25519.PublicKey)); err == nil {
		t.Fatal("annuaire modifié accepté")
	}
	if _, err := Charger(b, nil); err == nil {
		t.Fatal("sans clé d'autorité")
	}
}

func TestResoudre_RolesDates(t *testing.T) {
	l, _, _ := monter(t)
	f, ok := l.Resoudre("diane.risques@exemple.test", t0)
	if !ok || !f.DetientRole("directeur-risques") || strings.Join(f.Groupes, ",") != "direction-risques" || len(f.Cle) != 32 {
		t.Fatalf("diane %+v", f)
	}
	// Marc a perdu le rôle le 30 juin 2026.
	if f, _ := l.Resoudre("marc.ancien@exemple.test", t0); f.DetientRole("directeur-risques") {
		t.Fatal("rôle échu encore détenu")
	}
	if f, _ := l.Resoudre("marc.ancien@exemple.test", time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)); !f.DetientRole("directeur-risques") {
		t.Fatal("rôle non détenu avant son échéance")
	}
	if _, ok := l.Resoudre("inconnu@exemple.test", t0); ok {
		t.Fatal("identité inconnue résolue")
	}
}

func TestValider_Refus(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	cle := base64.StdEncoding.EncodeToString(pub)
	fin := t0.Add(-time.Hour)
	cas := map[string]Document{
		"vide":        {Meta: Meta{ID: "a", Version: 1}},
		"identifiant": {Meta: Meta{ID: "a", Version: 1}, Identites: []Identite{{Identite: "A B", Nom: "x", ClePublique: cle}}},
		"double":      {Meta: Meta{ID: "a", Version: 1}, Identites: []Identite{{Identite: "a", Nom: "x", ClePublique: cle}, {Identite: "a", Nom: "y", ClePublique: cle}}},
		"clé":         {Meta: Meta{ID: "a", Version: 1}, Identites: []Identite{{Identite: "a", Nom: "x", ClePublique: "xx"}}},
		"rôle daté":   {Meta: Meta{ID: "a", Version: 1}, Identites: []Identite{{Identite: "a", Nom: "x", ClePublique: cle, Roles: []Role{{Role: "r", Depuis: t0, JusquA: &fin}}}}},
		"groupe":      {Meta: Meta{ID: "a", Version: 1}, Identites: []Identite{{Identite: "a", Nom: "x", ClePublique: cle, Groupes: []string{"G"}}}},
		"version":     {Meta: Meta{ID: "a"}, Identites: []Identite{{Identite: "a", Nom: "x", ClePublique: cle}}},
		"même clé":    {Meta: Meta{ID: "a", Version: 1}, Identites: []Identite{{Identite: "a", Nom: "x", ClePublique: cle}, {Identite: "b", Nom: "y", ClePublique: cle}}},
	}
	for nom, d := range cas {
		if err := valider(&d); err == nil {
			t.Errorf("%s accepté", nom)
		}
	}
}

func TestJetonIdentite(t *testing.T) {
	l, cles, _ := monter(t)
	claire := "claire.analyste@exemple.test"
	j, err := SignerIdentite(cles.Identites[claire], claire, claire, t0, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	id, err := VerifierIdentite(l, j, t0.Add(time.Minute))
	if err != nil || id.Demandeur != claire || id.Emetteur != claire {
		t.Fatalf("%+v %v", id, err)
	}
	// Le portail peut signer pour autrui ; Bruno ne le peut pas.
	j, _ = SignerIdentite(cles.Identites["portail@exemple.test"], "portail@exemple.test", claire, t0, time.Hour)
	if id, err := VerifierIdentite(l, j, t0); err != nil || id.Demandeur != claire {
		t.Fatalf("portail : %+v %v", id, err)
	}
	bruno := "bruno.prestataire@exemple.test"
	j, _ = SignerIdentite(cles.Identites[bruno], bruno, claire, t0, time.Hour)
	if _, err := VerifierIdentite(l, j, t0); !errors.Is(err, ErrHabilitation) {
		t.Fatalf("usurpation : %v", err)
	}
	// Clé d'un autre que l'émetteur déclaré.
	j, _ = SignerIdentite(cles.Identites[bruno], claire, claire, t0, time.Hour)
	if _, err := VerifierIdentite(l, j, t0); !errors.Is(err, ErrSignataire) {
		t.Fatalf("mauvaise clé : %v", err)
	}
	// Expiré, trop long, futur.
	j, _ = SignerIdentite(cles.Identites[claire], claire, claire, t0, time.Hour)
	if _, err := VerifierIdentite(l, j, t0.Add(2*time.Hour)); !errors.Is(err, ErrJetonPerime) {
		t.Fatalf("expiré : %v", err)
	}
	j, _ = SignerIdentite(cles.Identites[claire], claire, claire, t0, 48*time.Hour)
	if _, err := VerifierIdentite(l, j, t0); !errors.Is(err, ErrJetonPerime) {
		t.Fatalf("trop long : %v", err)
	}
	j, _ = SignerIdentite(cles.Identites[claire], claire, claire, t0.Add(time.Hour), time.Hour)
	if _, err := VerifierIdentite(l, j, t0); !errors.Is(err, ErrJetonPerime) {
		t.Fatalf("futur : %v", err)
	}
	// Un jeton d'approbation n'est pas un jeton d'identité.
	j, _ = SignerApprobation(cles.Identites[claire], claire, "x", "e", t0, time.Minute)
	if _, err := VerifierIdentite(l, j, t0); !errors.Is(err, ErrAudience) {
		t.Fatalf("audience : %v", err)
	}
	if _, err := VerifierIdentite(l, "pas un jeton", t0); !errors.Is(err, ErrJetonIllisible) {
		t.Fatalf("illisible : %v", err)
	}
}

func TestJetonApprobation(t *testing.T) {
	l, cles, _ := monter(t)
	diane, marc := "diane.risques@exemple.test", "marc.ancien@exemple.test"
	j, _ := SignerApprobation(cles.Identites[diane], diane, "directeur-risques", "abc", t0, 5*time.Minute)
	a, err := VerifierApprobation(l, j, "abc", t0.Add(time.Minute))
	if err != nil || a.Valideur != diane || a.Role != "directeur-risques" || !a.Date.Equal(t0) || a.EmpreinteJeton != EmpreinteJeton(j) {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := VerifierApprobation(l, j, "autre", t0); !errors.Is(err, ErrAutreAppel) {
		t.Fatalf("autre appel : %v", err)
	}
	// Rôle échu à la date de validation.
	j, _ = SignerApprobation(cles.Identites[marc], marc, "directeur-risques", "abc", t0, 5*time.Minute)
	if _, err := VerifierApprobation(l, j, "abc", t0); !errors.Is(err, ErrHabilitation) {
		t.Fatalf("rôle échu : %v", err)
	}
	// Rôle que la valideuse n'a pas.
	j, _ = SignerApprobation(cles.Identites[diane], diane, "drh", "abc", t0, 5*time.Minute)
	if _, err := VerifierApprobation(l, j, "abc", t0); !errors.Is(err, ErrHabilitation) {
		t.Fatalf("rôle absent : %v", err)
	}
	// Plus de quinze minutes : refusé.
	j, _ = SignerApprobation(cles.Identites[diane], diane, "directeur-risques", "abc", t0, time.Hour)
	if _, err := VerifierApprobation(l, j, "abc", t0); !errors.Is(err, ErrJetonPerime) {
		t.Fatalf("durée : %v", err)
	}
}

func TestCles_EcritureLecture(t *testing.T) {
	_, cles, _ := monter(t)
	dir := filepath.Join(t.TempDir(), "cles")
	if err := EcrireCles(dir, cles); err != nil {
		t.Fatal(err)
	}
	k, err := LireCle(filepath.Join(dir, FichierCle("diane.risques@exemple.test")))
	if err != nil || !k.Equal(cles.Identites["diane.risques@exemple.test"]) {
		t.Fatal("clé relue différente")
	}
	p, err := LireClePublique(filepath.Join(dir, FichierAutoritePublic))
	if err != nil || !p.Equal(cles.Autorite.Public()) {
		t.Fatal("clé d'autorité relue différente")
	}
}
