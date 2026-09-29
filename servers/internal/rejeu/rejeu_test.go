package rejeu

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Ces tests n'utilisent que httptest : aucun accès à une API réelle.

func client(t *testing.T, dir string, mode Mode) *Client {
	t.Helper()
	c := Nouveau(dir, mode)
	c.Intervalle = 0
	c.Attente = time.Millisecond
	c.Journal = io.Discard
	return c
}

func TestModeDepuisEnv(t *testing.T) {
	t.Setenv(VariableMode, "")
	if m, err := ModeDepuisEnv(); err != nil || m != Rejeu {
		t.Fatalf("défaut = %q, %v ; attendu REJEU", m, err)
	}
	t.Setenv(VariableMode, "mixte")
	if m, err := ModeDepuisEnv(); err != nil || m != Mixte {
		t.Fatalf("mixte = %q, %v", m, err)
	}
	t.Setenv(VariableMode, "DIRCT")
	if _, err := ModeDepuisEnv(); err == nil {
		t.Fatal("mode inconnu accepté")
	}
}

func TestNettoyerURL(t *testing.T) {
	got, err := NettoyerURL("https://u:p@exemple.fr/a?q=1&api_key=S3&access_token=S4&Signature=x#f")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://exemple.fr/a?q=1" {
		t.Fatalf("got %q", got)
	}
	if _, err := NettoyerURL("file:///etc/passwd"); err == nil {
		t.Fatal("schéma file accepté")
	}
}

func TestDirectPuisRejeu(t *testing.T) {
	var appels atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appels.Add(1)
		if r.Header.Get("Authorization") != "Bearer JETON" {
			t.Errorf("entête non transmis")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"v":1}`)
	}))
	defer srv.Close()
	dir := t.TempDir()
	d := client(t, dir, Direct)
	fixe := time.Date(2026, 9, 29, 22, 0, 0, 0, time.UTC)
	d.Maintenant = func() time.Time { return fixe }

	req := Requete{URL: srv.URL + "/x?q=1&token=SECRET", Entetes: http.Header{"Authorization": {"Bearer JETON"}}}
	rep, err := d.Obtenir(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Source.Origine != OrigineDirect || string(rep.Corps) != `{"v":1}` {
		t.Fatalf("réponse directe inattendue: %+v", rep.Source)
	}
	// Mémoïsé : pas de second appel réseau.
	if _, err := d.Obtenir(context.Background(), req); err != nil || appels.Load() != 1 {
		t.Fatalf("appels = %d, err %v ; attendu 1 appel", appels.Load(), err)
	}

	fichiers, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(fichiers) != 1 {
		t.Fatalf("%d enregistrements, attendu 1", len(fichiers))
	}
	brut, _ := os.ReadFile(fichiers[0])
	for _, s := range []string{"SECRET", "JETON", "token"} {
		if strings.Contains(string(brut), s) {
			t.Fatalf("l'enregistrement contient %q", s)
		}
	}

	srv.Close() // plus aucun réseau
	r := client(t, dir, Rejeu)
	rep2, err := r.Obtenir(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Source.Origine != OrigineRejeu || rep2.Source.Date != "2026-09-29T22:00:00Z" || string(rep2.Corps) != `{"v":1}` {
		t.Fatalf("rejeu inattendu: %+v", rep2.Source)
	}
	if rep2.Source.SHA256 != Empreinte([]byte(`{"v":1}`)) {
		t.Fatal("empreinte fausse")
	}
}

func TestRejeuSansEnregistrementNeTouchePasAuReseau(t *testing.T) {
	var appels atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { appels.Add(1) }))
	defer srv.Close()
	r := client(t, t.TempDir(), Rejeu)
	_, err := r.Obtenir(context.Background(), Requete{URL: srv.URL})
	if !errors.Is(err, ErrIndisponible) {
		t.Fatalf("err = %v, attendu indisponible", err)
	}
	if appels.Load() != 0 {
		t.Fatal("le mode REJEU a appelé le réseau")
	}
}

func TestMixteBasculeSurEnregistrement(t *testing.T) {
	enPanne := atomic.Bool{}
	var appels atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appels.Add(1)
		if enPanne.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		io.WriteString(w, "ok")
	}))
	defer srv.Close()
	dir := t.TempDir()
	if _, err := client(t, dir, Direct).Obtenir(context.Background(), Requete{URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	enPanne.Store(true)
	appels.Store(0)
	m := client(t, dir, Mixte)
	rep, err := m.Obtenir(context.Background(), Requete{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Source.Origine != OrigineRejeu || !strings.Contains(rep.Source.Motif, "503") {
		t.Fatalf("source = %+v", rep.Source)
	}
	if appels.Load() != 3 {
		t.Fatalf("%d tentatives, attendu 3", appels.Load())
	}

	// Sans enregistrement : indisponible, pas de plantage.
	if _, err := m.Obtenir(context.Background(), Requete{URL: srv.URL + "/autre"}); !errors.Is(err, ErrIndisponible) {
		t.Fatalf("err = %v", err)
	}
}

func TestStatuts(t *testing.T) {
	var appels atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appels.Add(1)
		switch r.URL.Path {
		case "/404":
			w.WriteHeader(404)
		case "/400":
			w.WriteHeader(400)
		case "/401":
			w.WriteHeader(401)
		}
	}))
	defer srv.Close()
	c := client(t, t.TempDir(), Mixte)
	cas := map[string]error{"/404": ErrIntrouvable, "/400": ErrRequeteRefusee, "/401": ErrIndisponible}
	for p, attendu := range cas {
		appels.Store(0)
		_, err := c.Obtenir(context.Background(), Requete{URL: srv.URL + p})
		if !errors.Is(err, attendu) {
			t.Errorf("%s: err = %v, attendu %v", p, err, attendu)
		}
		if appels.Load() != 1 {
			t.Errorf("%s: %d appels, attendu 1 (pas de nouvelle tentative)", p, appels.Load())
		}
	}
}

func TestFiltreEtNePasEnregistrer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "public+prive")
	}))
	defer srv.Close()
	dir := t.TempDir()
	d := client(t, dir, Direct)
	f := &Filtre{Description: "retire prive", Appliquer: func(b []byte) ([]byte, error) {
		return []byte(strings.TrimSuffix(string(b), "+prive")), nil
	}}
	rep, err := d.Obtenir(context.Background(), Requete{URL: srv.URL + "/f", Filtre: f})
	if err != nil || string(rep.Corps) != "public+prive" {
		t.Fatalf("direct: %q %v", rep.Corps, err)
	}
	if _, err := d.Obtenir(context.Background(), Requete{URL: srv.URL + "/secret", NePasEnregistrer: true}); err != nil {
		t.Fatal(err)
	}
	refus := func([]byte) error { return errors.New("personne physique") }
	if _, err := d.Obtenir(context.Background(), Requete{URL: srv.URL + "/pp", Enregistrable: refus}); err != nil {
		t.Fatal(err)
	}
	r := client(t, dir, Rejeu)
	rej, err := r.Obtenir(context.Background(), Requete{URL: srv.URL + "/f"})
	if err != nil || string(rej.Corps) != "public" || rej.Source.Filtre != "retire prive" {
		t.Fatalf("rejeu filtré: %q %+v %v", rej.Corps, rej.Source, err)
	}
	if _, err := r.Obtenir(context.Background(), Requete{URL: srv.URL + "/secret"}); !errors.Is(err, ErrIndisponible) {
		t.Fatalf("réponse non enregistrable rejouée: %v", err)
	}
	if _, err := r.Obtenir(context.Background(), Requete{URL: srv.URL + "/pp"}); !errors.Is(err, ErrIndisponible) {
		t.Fatalf("réponse refusée par Enregistrable rejouée: %v", err)
	}
}

func TestEnregistrementAltereRefuse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "vrai") }))
	defer srv.Close()
	dir := t.TempDir()
	if _, err := client(t, dir, Direct).Obtenir(context.Background(), Requete{URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	fichiers, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	b, _ := os.ReadFile(fichiers[0])
	os.WriteFile(fichiers[0], []byte(strings.Replace(string(b), `"corps": "vrai"`, `"corps": "faux"`, 1)), 0o644)
	if _, err := client(t, dir, Rejeu).Obtenir(context.Background(), Requete{URL: srv.URL}); !errors.Is(err, ErrIndisponible) {
		t.Fatalf("enregistrement altéré servi: %v", err)
	}
}
