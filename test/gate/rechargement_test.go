package gatee2e

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/journal"
	"mcprogue/internal/rejeu"
)

// jeuProvenanceV2 ajoute au jeu du point de contrôle une directive de
// refus : le taux officiel ne doit plus être consulté.
var jeuProvenanceV2 = strings.Replace(
	strings.Replace(jeuProvenance, `"id": "credit-pme-gate", "version": 1`, `"id": "credit-pme-gate", "version": 2`, 1),
	"\n  ]\n}", `,
    {"id": "D-009", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Le taux officiel ne doit plus être consulté.",
     "fondement": {"texte": "Circulaire interne", "article": "3"},
     "verrou": "interdiction", "cible": {"serveur": "taux-officiel", "outil": "get_taux"}, "effet": "refuser"}
  ]
}`, 1)

// ecrireAtomique remplace le fichier par renommage, comme le ferait
// l'opérateur qui publie un jeu validé.
func ecrireAtomique(t *testing.T, chemin, contenu string) {
	t.Helper()
	tmp := chemin + ".tmp"
	if err := os.WriteFile(tmp, []byte(contenu), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, chemin); err != nil {
		t.Fatal(err)
	}
}

// attendreEntree relit le journal jusqu'à y trouver n entrées qui
// satisfont ok. La dernière ligne peut être en cours d'écriture : une
// lecture qui échoue est simplement refaite.
func attendreEntree(t *testing.T, chemin string, n int, ok func(journal.Entree) bool) {
	t.Helper()
	fin := time.Now().Add(10 * time.Second)
	for time.Now().Before(fin) {
		if brut, err := os.ReadFile(chemin); err == nil {
			if es, err := journal.Lire(bytes.NewReader(brut)); err == nil {
				c := 0
				for _, e := range es {
					if ok(e) {
						c++
					}
				}
				if c >= n {
					return
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("le journal n'a pas atteint %d entrée(s) attendue(s)", n)
}

// Le binaire mcp-gate recharge son jeu sur SIGHUP, pendant une session :
// bascule vers une version qui ajoute un refus, document invalide refusé
// (l'ancien reste actif), puis retour arrière.
func TestRechargement_SIGHUP(t *testing.T) {
	dir := t.TempDir()
	cheminJ, cheminC := filepath.Join(dir, "journal.jsonl"), filepath.Join(dir, "coffre.jsonl")
	cheminR, cheminD := filepath.Join(dir, "registre.json"), filepath.Join(dir, "directives.json")
	reg := registreE2E(t)
	b, err := json.Marshal(reg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cheminR, b, 0o600); err != nil {
		t.Fatal(err)
	}
	ecrireAtomique(t, cheminD, jeuProvenance)

	cmd := exec.Command(binaireGate, "-registre", cheminR, "-directives", cheminD, "-journal", cheminJ, "-coffre", cheminC)
	cmd.Env = []string{"MCP_GATE_CLE_JOURNAL=" + hex.EncodeToString(cleTest)}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	ctx, annuler := context.WithTimeout(context.Background(), 60*time.Second)
	defer annuler()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "0"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	defer cs.Close()
	hup := func() {
		t.Helper()
		if err := cmd.Process.Signal(syscall.SIGHUP); err != nil {
			t.Fatal(err)
		}
	}
	chargeV := func(v int) func(journal.Entree) bool {
		return func(e journal.Entree) bool { return e.Type == journal.JeuCharge && e.Jeu.Version == v }
	}
	taux := func() *mcp.CallToolResult {
		return appeler(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"})
	}

	// Sous v1 : autorisé.
	if r := taux(); r.IsError {
		t.Fatalf("v1 : %q", texte(r))
	}
	// Bascule vers v2 : l'appel suivant est refusé par la directive ajoutée.
	ecrireAtomique(t, cheminD, jeuProvenanceV2)
	hup()
	attendreEntree(t, cheminJ, 1, chargeV(2))
	if r := taux(); !r.IsError || !strings.Contains(texte(r), "D-009 v1") {
		t.Fatalf("v2 : %q", texte(r))
	}
	// Document invalide : refusé, v2 reste active.
	ecrireAtomique(t, cheminD, `{"jeu": {"id": "credit-pme-gate"}}`)
	hup()
	attendreEntree(t, cheminJ, 1, func(e journal.Entree) bool {
		return e.Type == journal.AlerteT && strings.Contains(e.Message, "credit-pme-gate v2 reste actif")
	})
	if r := taux(); !r.IsError || !strings.Contains(texte(r), "D-009 v1") {
		t.Fatalf("v2 après refus : %q", texte(r))
	}
	// Retour arrière vers v1.
	ecrireAtomique(t, cheminD, jeuProvenance)
	hup()
	attendreEntree(t, cheminJ, 2, chargeV(1))
	if r := taux(); r.IsError {
		t.Fatalf("retour à v1 : %q", texte(r))
	}
	if err := cs.Close(); err != nil {
		t.Fatalf("fermeture : %v\n%s", err, stderr.String())
	}

	brut, err := os.ReadFile(cheminJ)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Verifier(bytes.NewReader(brut), cleTest); err != nil {
		t.Fatal(err)
	}
	entrees, err := journal.Lire(bytes.NewReader(brut))
	if err != nil {
		t.Fatal(err)
	}
	var versions []int
	for _, e := range entrees {
		if e.Type == journal.AppelT {
			versions = append(versions, e.Jeu.Version)
		}
	}
	if len(versions) != 4 || versions[0] != 1 || versions[1] != 2 || versions[2] != 2 || versions[3] != 1 {
		t.Fatalf("versions citées %v", versions)
	}
	coffre, err := journal.OuvrirCoffre(cheminC)
	if err != nil {
		t.Fatal(err)
	}
	defer coffre.Fermer()
	rs, err := rejeu.RejouerJournal(entrees, coffre,
		rejeu.Catalogue{charger(t, jeuProvenance), charger(t, jeuProvenanceV2)}, rejeu.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rs {
		if !r.Rejouable || len(r.Ecarts) != 0 {
			t.Fatalf("rejeu seq %d : %+v", r.Seq, r.Ecarts)
		}
	}
}
