package console

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mcprogue/internal/console/simulateur"
)

var (
	cleTest = bytes.Repeat([]byte{0x42}, 32)
	// binaires construits une fois pour tous les tests
	binApprouver, binDirective, binSimulateur string
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "console-bins")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	for _, b := range []struct{ nom, pkg string }{{"mcp-approve", "./cmd/mcp-approve"}, {"directive", "./cmd/directive"}, {"simulateur", "./cmd/console/simulateur"}} {
		cmd := exec.Command("go", "build", "-o", filepath.Join(dir, b.nom), b.pkg)
		cmd.Dir = "../.."
		if out, err := cmd.CombinedOutput(); err != nil {
			panic(string(out))
		}
	}
	binApprouver, binDirective, binSimulateur = filepath.Join(dir, "mcp-approve"), filepath.Join(dir, "directive"), filepath.Join(dir, "simulateur")
	os.Exit(m.Run())
}

// banc est une démonstration complète dans un répertoire temporaire : règles,
// annuaire signé et clés, simulateur d'orchestrateur, simulateur de
// compilateur.
type banc struct {
	t       *testing.T
	racine  string
	Dep     *Depot
	Srv     *Serveur
	Sondage time.Duration
}

func nouveauBanc(t *testing.T) *banc {
	t.Helper()
	racine := t.TempDir()
	demo := filepath.Join(racine, "demo")
	must(t, simulateur.PreparerDemo(racine, "demo"))
	py, _ := filepath.Abs("testdata/compilateur_simule.py")
	dep := &Depot{Donnees: filepath.Join(demo, "execution", "sessions"), Demo: "demo", Cle: cleTest, Juriste: "juriste@exemple.test",
		Commandes: Commandes{Racine: racine, Approuver: []string{binApprouver}, Compilateur: []string{"python3", py}}}
	return &banc{t: t, racine: racine, Dep: dep, Srv: &Serveur{Depot: dep}, Sondage: 20 * time.Millisecond}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (b *banc) config() simulateur.Config {
	return simulateur.Config{Racine: b.racine, Demo: "demo", Cle: cleTest, Sondage: b.Sondage, Attente: 20 * time.Second}
}

// demande exécute une demande jusqu'au bout (sans validation attendue).
func (b *banc) demande(demandeur, texte string) string {
	b.t.Helper()
	dossier, err := b.config().Demande(demandeur, texte)
	if err != nil {
		b.t.Fatalf("demande %q : %v", texte, err)
	}
	return filepath.Base(dossier)
}

// demandeAsync lance une demande qui va attendre une validation ; rend le
// canal de fin. Les sessions étant nommées à la seconde, on attend la
// session créée.
func (b *banc) demandeAsync(demandeur, texte string) (id string, fin chan error) {
	b.t.Helper()
	avant := map[string]bool{}
	for _, j := range b.Dep.Journaux() {
		avant[j.ID] = true
	}
	fin = make(chan error, 1)
	go func() {
		_, err := b.config().Demande(demandeur, texte)
		fin <- err
	}()
	limite := time.Now().Add(10 * time.Second)
	for time.Now().Before(limite) {
		for _, j := range b.Dep.Journaux() {
			if !avant[j.ID] {
				return j.ID, fin
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	b.t.Fatal("la session n'est pas apparue")
	return "", nil
}

// attendreStatut attend que etat.json de la session porte ce statut.
func (b *banc) attendreStatut(id, statut string) *Etat {
	b.t.Helper()
	limite := time.Now().Add(15 * time.Second)
	for time.Now().Before(limite) {
		if j, ok := b.Dep.Journal(id); ok {
			if st, err := b.Dep.Lire(j); err == nil && st.Dossier != nil && st.Dossier.Statut == statut {
				return st
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	b.t.Fatalf("la session %s n'a pas atteint le statut %s", id, statut)
	return nil
}

func (b *banc) etat(id string) *Etat {
	b.t.Helper()
	j, ok := b.Dep.Journal(id)
	if !ok {
		b.t.Fatalf("session %s introuvable", id)
	}
	st, err := b.Dep.Lire(j)
	must(b.t, err)
	return st
}

func (b *banc) jeux() []JeuCharge {
	j, refus := b.Dep.ChargerJeux()
	if len(refus) != 0 {
		b.t.Fatalf("règles refusées : %v", refus)
	}
	return j
}

func (b *banc) tous() []*Etat { return b.Srv.tousEtats() }

const (
	claire = "claire.analyste@exemple.test"
	bruno  = "bruno.prestataire@exemple.test"
	anna   = "anna.risques@exemple.test"
	diane  = "diane.risques@exemple.test"
	marc   = "marc.ancien@exemple.test"
)

func contient(t *testing.T, s string, mots ...string) {
	t.Helper()
	for _, m := range mots {
		if !strings.Contains(s, m) {
			t.Errorf("%q absent de : %s", m, s)
		}
	}
}

// signerCmd appelle « mcp-approve valider » comme la console le fait.
func (b *banc) signerCmd(args ...string) (string, error) {
	out, err := exec.Command(binApprouver, append([]string{"valider"}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

func hexa(b []byte) string { return fmt.Sprintf("%x", b) }

// attendreSecondeApresJournaux rend la main quand la seconde en cours (celle
// où s'écrira une date d'effet tronquée à la seconde) est strictement
// postérieure à toute entrée de tous les journaux : indépendant de la charge
// de la machine, contrairement à une pause fixe.
func (b *banc) attendreSecondeApresJournaux() {
	b.t.Helper()
	var dernier time.Time
	for _, st := range b.tous() {
		for _, e := range st.Entrees {
			if e.TS.After(dernier) {
				dernier = e.TS
			}
		}
	}
	for !time.Now().UTC().Truncate(time.Second).After(dernier) {
		time.Sleep(20 * time.Millisecond)
	}
}
