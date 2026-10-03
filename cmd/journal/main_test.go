package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/backend"
	"mcprogue/internal/backendtest"
	"mcprogue/internal/directive"
	"mcprogue/internal/gate"
	"mcprogue/internal/journal"
	"mcprogue/internal/registre"
)

var cle = []byte(strings.Repeat("j", 32))

const h64 = "sha256:1111111111111111111111111111111111111111111111111111111111111111"

const jeuAvant = `{"jeu": {"id": "cmd-journal", "version": 1, "date_effet": "2026-01-01T00:00:00Z", "auteur": "juriste@exemple.test"},
 "directives": [{"id": "S-001", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
  "texte_fr": "Au-delà de 500 000 euros, refus.", "fondements": [{"texte": "Politique", "article": "1"}],
  "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"}},
  "condition": {"argument": {"champ": "/montant", "op": ">", "valeur": 500000}}, "effets": [{"effet": "refuser"}]}]}`

// jeuApres abaisse le seuil : l'appel de 100 000 € serait refusé.
var jeuApres = strings.NewReplacer(`"version": 1, "date_effet": "2026-01-01T00:00:00Z", "auteur"`, `"version": 2, "date_effet": "2026-01-01T00:00:00Z", "auteur"`,
	"500 000", "50 000", "500000", "50000").Replace(jeuAvant)

type faux struct{}

func (faux) Nom() string { return "banque-synthetique" }
func (faux) Attestation() directive.Attestation {
	return directive.Attestation{Mode: "tls-pin", Attendu: h64, Observe: h64, Schemas: true}
}
func (faux) Outils(context.Context) ([]*mcp.Tool, error) { return nil, nil }
func (faux) Fermer() error                               { return nil }
func (faux) Appeler(context.Context, string, map[string]any, mcp.Meta) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
}

// produire fait passer deux appels par un gate et rend le répertoire du
// journal, du coffre et des jeux.
func produire(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for nom, doc := range map[string]string{"avant.json": jeuAvant, "apres.json": jeuApres} {
		if err := os.WriteFile(filepath.Join(dir, nom), []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	jeu, err := directive.Charger([]byte(jeuAvant))
	if err != nil {
		t.Fatal(err)
	}
	reg := &registre.Registre{Meta: registre.MetaRegistre{ID: "test", Version: 1, Auteur: "ops"},
		Serveurs: []registre.Serveur{backendtest.HTTP("banque-synthetique", backendtest.Banque, "https://banque.lab/mcp", h64, "banque.lab")}}
	if err := registre.Valider(reg); err != nil {
		t.Fatal(err)
	}
	j, err := journal.Ouvrir(filepath.Join(dir, "journal.jsonl"), cle)
	if err != nil {
		t.Fatal(err)
	}
	coffre, err := journal.OuvrirCoffre(filepath.Join(dir, "coffre.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	h := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	g, err := gate.Demarrer(context.Background(), gate.Config{Registre: reg, Jeu: *jeu, Journal: j, Coffre: coffre,
		Horloge:   func() time.Time { h = h.Add(time.Second); return h },
		Connecter: func(context.Context, registre.Serveur, backend.Options) (backend.Backend, error) { return faux{}, nil }})
	if err != nil {
		t.Fatal(err)
	}
	tc, ts := mcp.NewInMemoryTransports()
	ss, _ := g.Serveur().Connect(context.Background(), ts, nil)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "a", Version: "0"}, nil).Connect(context.Background(), tc, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []float64{100000, 900000} {
		if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "banque-synthetique__attribuer_pret",
			Arguments: map[string]any{"montant": m, "siren": "1"}}); err != nil {
			t.Fatal(err)
		}
	}
	cs.Close()
	ss.Wait()
	g.Fermer()
	j.Fermer()
	coffre.Fermer()
	return dir
}

func lancer(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errw bytes.Buffer
	code := executer(args, &out, &errw, func(v string) string {
		if v == VarCle {
			return hex.EncodeToString(cle)
		}
		return ""
	})
	return code, out.String() + errw.String()
}

func TestVerifierEtRejouer(t *testing.T) {
	dir := produire(t)
	jl := filepath.Join(dir, "journal.jsonl")
	code, s := lancer(t, "verifier", jl)
	if code != 0 || !strings.Contains(s, "intègre") || !strings.Contains(s, "autoriser 1, refuser 1") || !strings.Contains(s, "Sessions : 1") {
		t.Fatalf("verifier %d :\n%s", code, s)
	}
	code, s = lancer(t, "rejouer", jl, "-directives", filepath.Join(dir, "avant.json"))
	if code != 0 || !strings.Contains(s, "Écarts   : aucun sur 2 appel(s)") || !strings.Contains(s, "refusé par S-001 v1") {
		t.Fatalf("rejouer %d :\n%s", code, s)
	}
	code, s = lancer(t, "rejouer", jl, "-directives", filepath.Join(dir, "avant.json"), filepath.Join(dir, "apres.json"))
	if code != 0 || !strings.Contains(s, "autoriser → refuser : 1") || !strings.Contains(s, "refuser → refuser : 1") {
		t.Fatalf("avant/après %d :\n%s", code, s)
	}

	// Journal altéré : chaîne rompue.
	b, _ := os.ReadFile(jl)
	if err := os.WriteFile(jl, bytes.Replace(b, []byte(`"refuser"`), []byte(`"autoriser"`), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, s = lancer(t, "verifier", jl); code != 1 || !strings.Contains(s, "ROMPUE") {
		t.Fatalf("altéré %d :\n%s", code, s)
	}
	if code, _ = lancer(t, "rejouer", jl, "-directives", filepath.Join(dir, "avant.json")); code != 1 {
		t.Fatalf("rejouer altéré : %d", code)
	}
}

func TestUsage(t *testing.T) {
	for _, args := range [][]string{{}, {"verifier"}, {"inconnu", "x"}, {"rejouer", "/nulle-part"}} {
		if code, _ := lancer(t, args...); code != 2 {
			t.Errorf("%v : %d", args, code)
		}
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "j.jsonl")
	os.WriteFile(f, nil, 0o600)
	for _, args := range [][]string{{"rejouer", f}, {"rejouer", f, "-directives"}, {"rejouer", f, "-directives", "a", "b", "c"},
		{"rejouer", f, "-directives", "a", "-annuaire", "x"}, {"rejouer", f, "-autre", "x"}} {
		if code, _ := lancer(t, args...); code != 2 {
			t.Errorf("%v : %d", args, code)
		}
	}
	var out, errw bytes.Buffer
	if code := executer([]string{"verifier", f}, &out, &errw, func(string) string { return "" }); code != 2 || !strings.Contains(errw.String(), "absente") {
		t.Fatalf("sans clé : %d %s", code, errw.String())
	}
}
