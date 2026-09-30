package essai

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/lanceur"
)

// CertificatAutoSigne crée en mémoire un certificat pour 127.0.0.1 et le
// magasin qui le reconnaît. Aucun fichier de clé n'est écrit.
func CertificatAutoSigne(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	cle, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	modele := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "essai"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, modele, modele, &cle.PublicKey, cle)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(c)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: cle}, pool
}

// ConnecterTLS sert srv en HTTP streamable sur TLS (lanceur.ServirTLS) sur
// un port local et ouvre une session client qui vérifie le certificat.
func ConnecterTLS(t *testing.T, srv *mcp.Server) *mcp.ClientSession {
	t.Helper()
	paire, pool := CertificatAutoSigne(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, annuler := context.WithCancel(context.Background())
	fini := make(chan error, 1)
	go func() { fini <- lanceur.ServirTLS(ctx, ln, srv, lanceur.ConfigTLS(paire)) }()
	hc := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	c := mcp.NewClient(&mcp.Implementation{Name: "essai-tls", Version: "0"}, nil)
	cs, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: "https://" + ln.Addr().String() + "/mcp", HTTPClient: hc}, nil)
	if err != nil {
		annuler()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cs.Close()
		annuler()
		if err := <-fini; err != nil {
			t.Errorf("serveur TLS: %v", err)
		}
	})
	return cs
}
