package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"context"
	"crypto/tls"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"

	"mcprogue/internal/backend"
	"mcprogue/internal/backendtest"
	"mcprogue/internal/identitytest"
	"mcprogue/internal/registre"
)

const (
	idBanque = "spiffe://lab.test/banque"
	idGate   = "spiffe://lab.test/gate"
)

// Le gate (backend http-spiffe) joint le serveur à travers le sidecar : la
// SVID du sidecar est l'identité du serveur, pour les seuls appelants listés.
func TestProxy_GateAttesteLeServeur(t *testing.T) {
	srv := backendtest.Serveur(backendtest.Banque)
	amont := httptest.NewUnstartedServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	amont.StartTLS()
	t.Cleanup(amont.Close)
	h, err := Proxy(amont.URL)
	if err != nil {
		t.Fatal(err)
	}
	ac := identitytest.Nouvelle(t, "lab.test")
	autorisant := tlsconfig.AuthorizeID(spiffeid.RequireFromString(idGate))
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hs := &http.Server{Handler: h}
	go hs.Serve(tls.NewListener(l, ac.Source(t, idBanque).ServerTLS(autorisant)))
	t.Cleanup(func() { hs.Close() })
	serveur := registre.Serveur{
		Nom: "banque-synthetique", Transport: registre.HTTPSpiffe, URL: "https://" + l.Addr().String() + "/mcp",
		Attestation: registre.Attestation{Mode: registre.ModeSpiffe, Attendu: idBanque},
		Outils:      backendtest.Outils(backendtest.Banque),
	}
	ctx, annuler := context.WithTimeout(context.Background(), 20*time.Second)
	defer annuler()

	b, err := backend.Connecter(ctx, serveur, backend.Options{Spiffe: ac.Source(t, idGate)})
	t.Cleanup(func() { b.Fermer() })
	if err != nil || !b.Attestation().Valide() {
		t.Fatalf("%v %+v", err, b.Attestation())
	}
	// Un autre appelant du même trust domain n'est pas admis par le sidecar.
	b2, err := backend.Connecter(ctx, serveur, backend.Options{Spiffe: ac.Source(t, "spiffe://lab.test/intrus")})
	t.Cleanup(func() { b2.Fermer() })
	if !errors.Is(err, backend.ErrNonConnecte) || b2.Attestation().Valide() {
		t.Fatalf("intrus admis : %v", err)
	}
}

func TestProxy_AmontEnBoucleLocaleSeulement(t *testing.T) {
	for _, a := range []string{"https://10.0.0.5:9443", "http://127.0.0.1:9443", "https://banque.example:9443", ""} {
		if _, err := Proxy(a); err == nil {
			t.Errorf("amont %q accepté", a)
		}
	}
	for _, a := range []string{"https://127.0.0.1:9443", "https://localhost:9443", "https://[::1]:9443"} {
		if _, err := Proxy(a); err != nil {
			t.Errorf("amont %q refusé : %v", a, err)
		}
	}
}

func TestGencert(t *testing.T) {
	dir := t.TempDir()
	if err := genererCert(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := tls.LoadX509KeyPair(dir+"/tls.crt", dir+"/tls.key"); err != nil {
		t.Fatal(err)
	}
}
