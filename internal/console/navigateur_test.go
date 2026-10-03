package console

import (
	"mcprogue/internal/console/simulateur"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// navigateur rend l'interpréteur Python qui porte Playwright
// (CONSOLE_NAVIGATEUR_PYTHON) ; sans lui, le test est sauté : go test reste
// autonome, sans navigateur.
func navigateur(t *testing.T) string {
	t.Helper()
	py := os.Getenv("CONSOLE_NAVIGATEUR_PYTHON")
	if py == "" {
		t.Skip("CONSOLE_NAVIGATEUR_PYTHON absent : test du navigateur sauté")
	}
	return py
}

// agitateur fait vivre la démonstration pendant un test du navigateur : une
// demande de prêt toutes les 1,2 s, qui fait quelques appels espacés, attend
// une validation quelques secondes, puis se bloque. Le sondage de chaque
// écran a donc toujours quelque chose de neuf. arreter attend la fin de
// toutes les demandes.
func (b *banc) agitateur() (arreter func()) {
	var wg sync.WaitGroup
	fin := make(chan struct{})
	cfg := b.config()
	cfg.Pause, cfg.Attente = 700*time.Millisecond, 5*time.Second
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = cfg.Demande(claire, "Peut-on accorder un prêt de 120 000 euros pour le dossier DOS-FICTIF-002 ?")
			}()
			select {
			case <-fin:
				return
			case <-time.After(1200 * time.Millisecond):
			}
		}
	}()
	return func() { close(fin); wg.Wait() }
}

// attenteLongue lance une demande de prêt qui attend sa validation deux
// minutes au plus ; rend son identifiant et le canal de fin.
func (b *banc) attenteLongue() (string, chan error) {
	b.t.Helper()
	id, fin := b.demandeAsyncCfg(func(c *simulateur.Config) { c.Attente = 2 * time.Minute }, claire,
		"Peut-on accorder un prêt de 150 000 euros pour le dossier DOS-FICTIF-002 ?")
	b.attendreStatut(id, StatutEnAttente)
	return id, fin
}

// Point 1 (console-3) : le sondage ne reconstruit jamais un formulaire.
// Saisie lente caractère par caractère sur plus de 6 s et choix dans une
// liste pendant un sondage, sur le Poste, la Validation et la Rédaction.
func TestNavigateurFormulaires(t *testing.T) {
	py := navigateur(t)
	b := nouveauBanc(t)
	b.demande(claire, "Peut-on accorder un prêt sans gel vérifié ?")
	web, _ := filepath.Abs("web")
	if d := os.Getenv("CONSOLE_NAVIGATEUR_WEB"); d != "" {
		web = d // pour vérifier que le test échoue sur une ancienne interface
	}
	b.Srv.Dev = web
	srv := httptest.NewServer(b.Srv.Handler())
	defer srv.Close()
	// Une attente qui dure le temps du test (rejetée à la fin, ce qui termine
	// la demande), en plus de celles, brèves, de l'agitateur.
	longue, finLongue := b.attenteLongue()
	arreter := b.agitateur()
	defer func() {
		arreter()
		if _, err := b.Dep.Rejeter(longue, anna, "fin du test"); err != nil {
			t.Error(err)
		}
		<-finLongue
	}()
	// Au moins une attente avant d'ouvrir la file.
	limite := time.Now().Add(20 * time.Second)
	for len(b.Dep.File(b.tous(), b.jeux(), time.Now()).Attentes) == 0 {
		if time.Now().After(limite) {
			t.Fatal("aucune attente apparue")
		}
		time.Sleep(100 * time.Millisecond)
	}
	script, _ := filepath.Abs("testdata/navigateur/formulaires.py")
	out, err := exec.Command(py, script, srv.URL, longue).CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("formulaires reconstruits ou saisie perdue : %v", err)
	}
}
