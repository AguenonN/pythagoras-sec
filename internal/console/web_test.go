package console

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Rien ne doit être chargé depuis le réseau : aucune URL absolue dans les
// fichiers embarqués, hors texte affiché.
func TestAucuneRessourceExterne(t *testing.T) {
	re := regexp.MustCompile(`(?i)(src|href|action)\s*=\s*["']?(https?:)?//|url\(\s*["']?(https?:)?//|@import|fetch\(\s*["'\x60]https?:|new WebSocket|importScripts`)
	n := 0
	err := fs.WalkDir(webFS, "web", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		n++
		b, _ := webFS.ReadFile(p)
		if m := re.Find(b); m != nil {
			t.Errorf("%s charge une ressource externe : %q", p, m)
		}
		return nil
	})
	if err != nil || n < 3 {
		t.Fatalf("fichiers embarqués : %d, %v", n, err)
	}
}

// Le vocabulaire de l'écran : ni « jeu », ni « v1 » / « v2 ». Les lignes de
// commentaire sont ignorées ; le code lui-même n'emploie pas ces mots dans
// ses textes. La CSP interdit l'attribut style : le code doit passer par le
// CSSOM, et jamais par innerHTML.
func TestVocabulaireEtCSP(t *testing.T) {
	mot := regexp.MustCompile(`(?i)\bjeux?\b|\bv[12]\b`)
	err := fs.WalkDir(webFS, "web", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !(strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".html")) {
			return err
		}
		b, _ := webFS.ReadFile(p)
		for i, l := range strings.Split(string(b), "\n") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "//") || strings.HasPrefix(l, "/*") || strings.HasPrefix(l, "*") {
				continue
			}
			if m := mot.FindString(l); m != "" {
				t.Errorf("%s:%d emploie %q : %s", p, i+1, m, l)
			}
			if strings.Contains(l, "innerHTML") || strings.Contains(l, "setAttribute('style'") || strings.Contains(l, "outerHTML") {
				t.Errorf("%s:%d : innerHTML ou attribut style interdits (CSP) : %s", p, i+1, l)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func requete(t *testing.T, srv *httptest.Server, methode, chemin, host, corps string, entetes map[string]string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(methode, srv.URL+chemin, strings.NewReader(corps))
	if host != "" {
		req.Host = host
	}
	for k, v := range entetes {
		req.Header.Set(k, v)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var b bytes.Buffer
	b.ReadFrom(r.Body)
	return r, b.Bytes()
}

func TestHTTPEtGardes(t *testing.T) {
	b := nouveauBanc(t)
	id := b.demande(claire, "Peut-on accorder un prêt sans gel ?")
	srv := httptest.NewServer(b.Srv.Handler())
	defer srv.Close()
	js := map[string]string{"Content-Type": "application/json"}
	if r, _ := requete(t, srv, "GET", "/", "", "", nil); r.StatusCode != 200 || !strings.Contains(r.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatalf("page : %d %v", r.StatusCode, r.Header)
	}
	if r, _ := requete(t, srv, "GET", "/api/etat", "evil.example", "", nil); r.StatusCode != http.StatusForbidden {
		t.Fatalf("hôte étranger : %d", r.StatusCode)
	}
	_, corps := requete(t, srv, "GET", "/api/journal/"+id+"/session/"+Sessions(b.etat(id))[0].ID+"/chaine", "", "", nil)
	var v VueChaine
	if err := json.Unmarshal(corps, &v); err != nil || len(v.Etapes) != 3 || !v.Verif.OK {
		t.Fatalf("chaîne : %v %s", err, corps)
	}
	if r, _ := requete(t, srv, "GET", "/api/journal/..%2Fx/session/S1/chaine", "", "", nil); r.StatusCode != http.StatusNotFound {
		t.Fatalf("identifiant hors liste : %d", r.StatusCode)
	}
	for _, c := range []string{"/api/etat", "/api/regles", "/api/regles?directive=D-002", "/api/avant-apres", "/api/avant-apres?directive=D-001", "/api/file",
		"/api/notifications", "/api/identites", "/api/redaction", "/api/lancements", "/api/journal/" + id + "/session/" + Sessions(b.etat(id))[0].ID + "/poste"} {
		if r, body := requete(t, srv, "GET", c, "", "", nil); r.StatusCode != 200 {
			t.Fatalf("%s : %d %s", c, r.StatusCode, body)
		}
	}
	// Écritures : corps JSON, origine de la console, champs connus.
	if r, _ := requete(t, srv, "POST", "/api/validation/rejeter", "", `{}`, map[string]string{"Content-Type": "text/plain"}); r.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("type de contenu : %d", r.StatusCode)
	}
	mauvaise := map[string]string{"Content-Type": "application/json", "Origin": "http://evil.example"}
	if r, _ := requete(t, srv, "POST", "/api/validation/rejeter", "", `{}`, mauvaise); r.StatusCode != http.StatusForbidden {
		t.Fatalf("origine étrangère : %d", r.StatusCode)
	}
	if r, _ := requete(t, srv, "POST", "/api/validation/rejeter", "", `{"inconnu": 1}`, js); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("champ inconnu : %d", r.StatusCode)
	}
	if r, _ := requete(t, srv, "POST", "/api/validation/valider", "evil.example", `{}`, js); r.StatusCode != http.StatusForbidden {
		t.Fatalf("écriture depuis un hôte étranger : %d", r.StatusCode)
	}
	if r, _ := requete(t, srv, "POST", "/api/validation/valider", "", `{"journal":"`+id+`","valideur":"`+anna+`","role":"analyste-risques"}`, js); r.StatusCode != http.StatusConflict {
		t.Fatalf("session qui n'attend rien : %d", r.StatusCode)
	}
}

// -dev : les fichiers sont servis depuis le disque, sans recompiler.
func TestModeDev(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<p>v-disque-1</p>"), 0o644))
	b := nouveauBanc(t)
	b.Srv.Dev = dir
	srv := httptest.NewServer(b.Srv.Handler())
	defer srv.Close()
	_, c1 := requete(t, srv, "GET", "/", "", "", nil)
	must(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<p>v-disque-2</p>"), 0o644))
	_, c2 := requete(t, srv, "GET", "/", "", "", nil)
	if !strings.Contains(string(c1), "disque-1") || !strings.Contains(string(c2), "disque-2") {
		t.Fatalf("le disque n'est pas relu : %q %q", c1, c2)
	}
}

func httptestServeur(t *testing.T, b *banc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(b.Srv.Handler())
	t.Cleanup(srv.Close)
	return srv
}
