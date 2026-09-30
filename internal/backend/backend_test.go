package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/backendtest"
	"mcprogue/internal/directive"
)

var binaire, empreinte string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "backend-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binaire, empreinte, err = backendtest.Construire(dir)
	if err != nil {
		os.RemoveAll(dir)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func ctxTest(t *testing.T) context.Context {
	t.Helper()
	ctx, annuler := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(annuler)
	return ctx
}

func connecter(t *testing.T, nom, profil, attendu string, opts Options) (Backend, error) {
	t.Helper()
	b, err := Connecter(ctxTest(t), backendtest.Stdio(nom, profil, binaire, attendu), opts)
	if b == nil {
		t.Fatal("Connecter doit toujours rendre un backend")
	}
	t.Cleanup(func() { b.Fermer() })
	return b, err
}

func texte(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	var out []string
	for _, c := range r.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			out = append(out, tc.Text)
		}
	}
	return strings.Join(out, "\n")
}

func TestStdio_Conforme(t *testing.T) {
	b, err := connecter(t, "taux-officiel", backendtest.Taux, empreinte, Options{})
	if err != nil {
		t.Fatal(err)
	}
	a := b.Attestation()
	if !a.Valide() || !a.Verifiee || a.Mode != "binaire" || a.Observe != empreinte || !a.Schemas || a.Detail != "" {
		t.Fatalf("attestation %+v", a)
	}
	if b.Nom() != "taux-officiel" {
		t.Fatal(b.Nom())
	}
	outils, err := b.Outils(ctxTest(t))
	if err != nil || len(outils) != 1 || outils[0].Name != "get_taux" {
		t.Fatalf("outils %v %v", outils, err)
	}
	r, err := b.Appeler(ctxTest(t), "get_taux", map[string]any{"devise": "EUR"}, nil)
	if err != nil || r.IsError || !strings.Contains(texte(t, r), "taux EUR") {
		t.Fatalf("résultat %+v %v", r, err)
	}
	r, err = b.Appeler(ctxTest(t), "get_taux", map[string]any{"devise": "ERREUR"}, nil)
	if err != nil || !r.IsError {
		t.Fatalf("erreur d'outil attendue : %+v %v", r, err)
	}
}

func TestStdio_OutilHorsCatalogue(t *testing.T) {
	b, err := connecter(t, "taux-officiel", backendtest.Taux, empreinte, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Appeler(ctxTest(t), "exfiltrer", nil, nil); !errors.Is(err, ErrOutilInconnu) {
		t.Fatalf("erreur %v", err)
	}
}

// Un binaire dont l'empreinte diffère du registre n'est jamais lancé.
func TestStdio_EmpreinteDifferente_JamaisLance(t *testing.T) {
	dir := t.TempDir()
	temoin := filepath.Join(dir, "lance")
	script := filepath.Join(dir, "serveur.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n: > "+temoin+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	observe, _ := backendtest.Empreinte(script)
	s := backendtest.Stdio("taux-officiel", backendtest.Taux, script, empreinte)
	b, err := Connecter(ctxTest(t), s, Options{})
	if err == nil || !errors.Is(err, ErrNonConnecte) {
		t.Fatalf("erreur %v", err)
	}
	a := b.Attestation()
	if a.Valide() || a.Verifiee || a.Observe != observe || a.Attendu != empreinte || !strings.Contains(a.Detail, "non lancé") {
		t.Fatalf("attestation %+v", a)
	}
	if _, err := os.Stat(temoin); !os.IsNotExist(err) {
		t.Fatal("le binaire non conforme a été exécuté")
	}
	if _, err := b.Appeler(ctxTest(t), "get_taux", nil, nil); !errors.Is(err, ErrNonAttest) {
		t.Fatalf("erreur %v", err)
	}
	if _, err := b.Outils(ctxTest(t)); !errors.Is(err, ErrNonConnecte) {
		t.Fatalf("erreur %v", err)
	}
	if err := b.Fermer(); err != nil {
		t.Fatal(err)
	}
}

func TestStdio_BinaireIntrouvable(t *testing.T) {
	s := backendtest.Stdio("taux-officiel", backendtest.Taux, "/nexiste/pas", empreinte)
	b, err := Connecter(ctxTest(t), s, Options{})
	if err == nil || b.Attestation().Valide() || !strings.Contains(b.Attestation().Detail, "illisible") {
		t.Fatalf("%v %+v", err, b.Attestation())
	}
}

// Le bon binaire, mais un catalogue qui n'est pas celui épinglé : lancé,
// non attesté, rien n'est transmis.
func TestStdio_CatalogueNonConforme(t *testing.T) {
	b, err := connecter(t, "taux-officiel", backendtest.Imposteur, empreinte, Options{})
	if !errors.Is(err, ErrNonAttest) {
		t.Fatalf("erreur %v", err)
	}
	a := b.Attestation()
	if a.Valide() || a.Schemas || a.Observe != empreinte || !strings.Contains(a.Detail, "modifié(s) : get_taux") {
		t.Fatalf("attestation %+v", a)
	}
	if strings.Contains(a.Detail, "IMPORTANT") {
		t.Fatal("le détail ne doit rien citer de ce que le serveur annonce")
	}
	if _, err := b.Appeler(ctxTest(t), "get_taux", map[string]any{"devise": "EUR"}, nil); !errors.Is(err, ErrNonAttest) {
		t.Fatalf("erreur %v", err)
	}
	// La session d'un serveur non attesté est fermée dès la connexion.
	if _, err := b.Outils(ctxTest(t)); !errors.Is(err, ErrNonConnecte) {
		t.Fatalf("erreur %v", err)
	}
}

// Un outil épinglé que le serveur n'annonce pas, et un outil annoncé hors
// registre : les deux font échouer l'attestation.
func TestStdio_CatalogueIncomplet(t *testing.T) {
	s := backendtest.Stdio("taux-officiel", backendtest.Taux, binaire, empreinte)
	s.Outils = append(s.Outils, backendtest.Outils(backendtest.Banque)...)
	b, err := Connecter(ctxTest(t), s, Options{})
	t.Cleanup(func() { b.Fermer() })
	if !errors.Is(err, ErrNonAttest) || !strings.Contains(b.Attestation().Detail, "absent(s) : attribuer_pret") {
		t.Fatalf("%v %+v", err, b.Attestation())
	}

	s = backendtest.Stdio("taux-officiel", backendtest.Taux, binaire, empreinte)
	s.Outils = nil
	s.Outils = append(s.Outils, backendtest.Outils(backendtest.Banque)...)
	b2, err := Connecter(ctxTest(t), s, Options{})
	t.Cleanup(func() { b2.Fermer() })
	if !errors.Is(err, ErrNonAttest) || !strings.Contains(b2.Attestation().Detail, "1 outil(s) annoncé(s) hors registre") {
		t.Fatalf("%v %+v", err, b2.Attestation())
	}
}

// tools/list_changed suspend le backend, et une dérive le met en quarantaine
// pour de bon.
func TestStdio_DeriveQuarantaine(t *testing.T) {
	changements := make(chan directive.Attestation, 8)
	b, err := connecter(t, "taux-officiel", backendtest.Taux, empreinte, Options{
		AuChangement: func(nom string, a directive.Attestation) {
			if nom != "taux-officiel" {
				t.Errorf("nom %q", nom)
			}
			changements <- a
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Appeler(ctxTest(t), "get_taux", map[string]any{"devise": "DERIVE"}, nil); err != nil {
		t.Fatal(err)
	}
	var derniere directive.Attestation
	for fini := false; !fini; {
		select {
		case derniere = <-changements:
			fini = strings.Contains(derniere.Detail, "quarantaine")
		case <-time.After(10 * time.Second):
			t.Fatalf("pas de quarantaine ; dernière attestation %+v", derniere)
		}
	}
	if derniere.Valide() || derniere.Schemas || !strings.Contains(derniere.Detail, "1 outil(s) annoncé(s) hors registre") {
		t.Fatalf("attestation %+v", derniere)
	}
	if a := b.Attestation(); a.Valide() || !strings.Contains(a.Detail, "quarantaine") {
		t.Fatalf("attestation %+v", a)
	}
	if _, err := b.Appeler(ctxTest(t), "get_taux", map[string]any{"devise": "EUR"}, nil); !errors.Is(err, ErrNonAttest) {
		t.Fatalf("erreur %v", err)
	}
}

func TestStdio_FermerPuisAppeler(t *testing.T) {
	b, err := connecter(t, "taux-officiel", backendtest.Taux, empreinte, Options{Stderr: os.Stderr})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Fermer(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Appeler(ctxTest(t), "get_taux", nil, nil); !errors.Is(err, ErrNonAttest) {
		t.Fatalf("erreur %v", err)
	}
	if err := b.Fermer(); err != nil {
		t.Fatal("Fermer doit être idempotent")
	}
}

// Le client n'annonce aucune capacité et ne câble aucun gestionnaire de
// requête initiée par le serveur.
func TestClient_AucuneCapacite(t *testing.T) {
	b, err := connecter(t, "taux-officiel", backendtest.Taux, empreinte, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var vus []string
	opts := b.(*client).optionsClient()
	if opts.CreateMessageHandler != nil || opts.CreateMessageWithToolsHandler != nil {
		vus = append(vus, "sampling")
	}
	if opts.ElicitationHandler != nil {
		vus = append(vus, "elicitation")
	}
	if opts.Capabilities == nil || opts.Capabilities.Sampling != nil || opts.Capabilities.Elicitation != nil || opts.Capabilities.RootsV2 != nil {
		vus = append(vus, "capacités")
	}
	if opts.MultiRoundTrip == nil || !opts.MultiRoundTrip.Disabled {
		vus = append(vus, "multi-aller-retour")
	}
	if len(vus) > 0 {
		t.Fatalf("câblé : %v", vus)
	}
}

// Le serveur stdio ne reçoit que les variables que le registre nomme,
// avec leur valeur ; les autres, secrets compris, ne passent pas.
func TestStdio_Environnement(t *testing.T) {
	t.Setenv("MCP_TEST_TRANSMISE", "douze-octets")
	t.Setenv("MCP_TEST_SECRET", "ne-doit-pas-passer")
	s := backendtest.Stdio("taux-officiel", backendtest.Taux, binaire, empreinte)
	s.Commande.Env = []string{"MCP_TEST_TRANSMISE", "MCP_TEST_ABSENTE"}
	b, err := Connecter(ctxTest(t), s, Options{})
	t.Cleanup(func() { b.Fermer() })
	if err != nil {
		t.Fatal(err)
	}
	r, err := b.Appeler(ctxTest(t), "get_taux", map[string]any{"devise": "ENV"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := texte(t, r); got != "env MCP_TEST_TRANSMISE:12" {
		t.Fatalf("environnement reçu : %q", got)
	}

	// Sans liste, rien ne passe.
	b2, err := connecter(t, "taux-officiel", backendtest.Taux, empreinte, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r, err := b2.Appeler(ctxTest(t), "get_taux", map[string]any{"devise": "ENV"}, nil); err != nil || texte(t, r) != "env " {
		t.Fatalf("%q %v", texte(t, r), err)
	}
}
