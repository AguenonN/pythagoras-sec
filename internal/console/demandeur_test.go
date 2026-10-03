package console

import (
	"encoding/json"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"
)

func conseillerDe(t *testing.T, b *banc) string {
	t.Helper()
	_, corps := get(t, b.Srv, "/api/etat")
	var v struct {
		Conseiller string `json:"conseiller"`
	}
	must(t, json.Unmarshal([]byte(corps), &v))
	return v.Conseiller
}

// Point 6 (console-3) : l'identité connectée proposée comme demandeur.
func TestConseillerParDefaut(t *testing.T) {
	b := nouveauBanc(t)
	if c := conseillerDe(t, b); c != "" {
		t.Errorf("sans demande : %q", c)
	}
	b.demande(bruno, "Produis le rapport de synthèse du dossier DOS-FICTIF-002")
	b.attendreSecondeApresJournaux()
	b.demande(claire, "Peut-on accorder un prêt sans gel vérifié ?")
	if c := conseillerDe(t, b); c != claire {
		t.Errorf("demandeur le plus récent : %q", c)
	}
	b.Dep.Conseiller = anna
	if c := conseillerDe(t, b); c != anna {
		t.Errorf("-conseiller : %q", c)
	}
	b.Dep.Conseiller = "inconnu@exemple.test"
	if c := conseillerDe(t, b); c != claire {
		t.Errorf("identité hors annuaire : %q", c)
	}
}

// Point 6 (console-3), dans le navigateur : le demandeur ne change pas seul.
func TestNavigateurDemandeur(t *testing.T) {
	py := navigateur(t)
	b := nouveauBanc(t)
	sb := b.demande(bruno, "Produis le rapport de synthèse du dossier DOS-FICTIF-002")
	b.attendreSecondeApresJournaux()
	sc := b.demande(claire, "Peut-on accorder un prêt sans gel vérifié ?")
	b.Dep.Conseiller = anna
	web, _ := filepath.Abs("web")
	b.Srv.Dev = web
	srv := httptest.NewServer(b.Srv.Handler())
	defer srv.Close()
	script, _ := filepath.Abs("testdata/navigateur/demandeur.py")
	out, err := exec.Command(py, script, srv.URL, sb, sc).CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("demandeur instable : %v", err)
	}
}
