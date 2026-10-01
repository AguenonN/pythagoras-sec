package gate

import (
	"crypto/ed25519"
	"path/filepath"
	"testing"
	"time"

	"mcprogue/internal/annuaire"
	"mcprogue/internal/journal"
)

// annuaireTest génère l'annuaire de démonstration, avec des clés neuves.
func annuaireTest(t *testing.T) (*annuaire.Local, *annuaire.Cles) {
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
	return l, cles
}

// Au démarrage, la source d'identité et la référence de l'annuaire sont
// consignées au journal.
func TestAnnuaire_ConsigneAuDemarrage(t *testing.T) {
	l, _ := annuaireTest(t)
	b := monter(t, func(b *banc) { b.config = func(c *Config) { c.Annuaire = l } })
	var vu *journal.Entree
	for _, e := range b.entrees(t) {
		if e.Type == journal.AnnuaireCharge {
			vu = &e
		}
	}
	ref := l.Reference()
	if vu == nil || vu.SourceIdentite != "annuaire-local-demo" || vu.Annuaire == nil ||
		*vu.Annuaire != (journal.RefAnnuaire{ID: ref.ID, Version: ref.Version, Empreinte: ref.Empreinte}) {
		t.Fatalf("entrée annuaire_charge %+v", vu)
	}
	// Sans annuaire, pas d'entrée.
	for _, e := range monter(t, nil).entrees(t) {
		if e.Type == journal.AnnuaireCharge {
			t.Fatal("annuaire_charge sans annuaire")
		}
	}
}
