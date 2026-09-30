package lanceur

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
)

// certificat auto-signé pour 127.0.0.1, en mémoire.
func certificat(t *testing.T) (tls.Certificate, *x509.CertPool) {
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
	c, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(c)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: cle}, pool
}

func TestHTTPStreamableTLS(t *testing.T) {
	paire, pool := certificat(t)
	srv := mcp.NewServer(&mcp.Implementation{Name: "essai", Version: "0"}, nil)
	mcp.AddTool(srv, &mcp.Tool{Name: "ping"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct {
		Pong bool `json:"pong"`
	}, error) {
		return nil, struct {
			Pong bool `json:"pong"`
		}{true}, nil
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, annuler := context.WithCancel(context.Background())
	fini := make(chan error, 1)
	go func() { fini <- ServirTLS(ctx, ln, srv, ConfigTLS(paire)) }()

	url := "https://" + ln.Addr().String() + "/mcp"
	hc := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	c := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil)
	cs, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: hc}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "ping", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("ping: %v %+v", err, res)
	}
	cs.Close()

	// HTTP en clair : refusé.
	if resp, err := http.Get("http://" + ln.Addr().String() + "/mcp"); err == nil {
		if resp.StatusCode < 400 {
			t.Fatalf("HTTP en clair accepté: %d", resp.StatusCode)
		}
		resp.Body.Close()
	}
	// Certificat non reconnu : refusé par le client.
	if _, err := http.Get(url); err == nil {
		t.Fatal("certificat inconnu accepté par un client par défaut")
	}

	annuler()
	select {
	case err := <-fini:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("arrêt du serveur trop long")
	}
}
