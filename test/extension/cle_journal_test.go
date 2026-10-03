package extension

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcprogue/internal/journal"
)

// Un journal de demo/execution/sessions signé avec une autre clé arrête le
// lanceur avant le gate, avec un message clair (et non « le serveur MCP
// s'est arrêté pendant initialize ») ; « archiver-sessions » le déplace
// dans demo/execution/sessions-archivees/<date>/ sans rien supprimer, et
// laisse en place les journaux qui se vérifient.
func TestJournalAutreCle(t *testing.T) {
	if imbrique() {
		t.Skip("test imbriqué")
	}
	r := racine(t)
	for _, f := range []string{"bin/mcp-gate", "bin/mcp-approve", "bin/journal", "demo/registre.json", "demo/annuaire.json", "demo/execution/cles/autorite.pub"} {
		if _, err := os.Stat(filepath.Join(r, f)); err != nil {
			t.Skipf("%s absent : lancer d'abord scripts/lancer-demo.sh", f)
		}
	}
	cle := os.Getenv("MCP_GATE_CLE_JOURNAL")
	if cle == "" {
		t.Skip("MCP_GATE_CLE_JOURNAL absente")
	}
	env := "MCP_GATE_CLE_JOURNAL=" + cle

	nom := "00000000-000000-essai-autre-cle"
	session := filepath.Join(r, "demo", "execution", "sessions", nom)
	if err := os.MkdirAll(session, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(session) })
	autre := bytes.Repeat([]byte{7}, 32)
	e, err := journal.Ouvrir(filepath.Join(session, "journal.jsonl"), autre)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Ecrire(journal.Entree{Type: "demarrage", Message: "essai"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Fermer(); err != nil {
		t.Fatal(err)
	}

	sortie, err := essayer(r, "scripts/lancer-demo.sh demande 'Bonjour' --demandeur camille.conseiller@exemple.test", env)
	if err == nil {
		t.Fatalf("la demande aurait dû s'arrêter :\n%s", sortie)
	}
	if !strings.Contains(sortie, "journaux signés avec une autre clé : scripts/lancer-demo.sh archiver-sessions") {
		t.Fatalf("message attendu absent :\n%s", sortie)
	}
	if strings.Contains(sortie, "initialize") {
		t.Fatalf("le gate a été lancé :\n%s", sortie)
	}

	sortie = bash(t, r, "scripts/lancer-demo.sh archiver-sessions", env)
	if _, err := os.Stat(session); !os.IsNotExist(err) {
		t.Fatalf("session non déplacée :\n%s", sortie)
	}
	archives, _ := filepath.Glob(filepath.Join(r, "demo", "execution", "sessions-archivees", "*", nom, "journal.jsonl"))
	if len(archives) != 1 {
		t.Fatalf("archive introuvable (%v) :\n%s", archives, sortie)
	}
	t.Cleanup(func() { os.RemoveAll(filepath.Dir(archives[0])) })
	// Les journaux restants se vérifient : rien d'autre à archiver.
	if s := bash(t, r, "scripts/lancer-demo.sh archiver-sessions", env); !strings.Contains(s, "rien à archiver") {
		t.Fatalf("seconde archive non vide :\n%s", s)
	}
}
