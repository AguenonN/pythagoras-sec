// Package identitytest fournit aux tests une autorité de certification qui
// émet des SVID X.509 SPIFFE, sans SPIRE : les mêmes sources go-spiffe que
// la Workload API, en mémoire.
package identitytest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"

	"mcprogue/internal/identity"
)

// Autorite est une AC de test d'un trust domain.
type Autorite struct {
	TrustDomain spiffeid.TrustDomain
	cert        *x509.Certificate
	key         *ecdsa.PrivateKey
}

// Nouvelle crée une AC pour le trust domain.
func Nouvelle(t testing.TB, trustDomain string) *Autorite {
	t.Helper()
	td, err := spiffeid.TrustDomainFromString(trustDomain)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	modele := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "AC de test " + trustDomain},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, modele, modele, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &Autorite{TrustDomain: td, cert: cert, key: key}
}

// Bundle est le bundle de confiance de l'AC.
func (a *Autorite) Bundle() *x509bundle.Bundle {
	return x509bundle.FromX509Authorities(a.TrustDomain, []*x509.Certificate{a.cert})
}

// SVID émet une SVID pour l'identité, valable de debut à fin.
func (a *Autorite) SVID(t testing.TB, id string, debut, fin time.Time) *x509svid.SVID {
	t.Helper()
	u, err := url.Parse(id)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	modele := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		NotBefore:    debut,
		NotAfter:     fin,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		URIs:         []*url.URL{u},
	}
	der, err := x509.CreateCertificate(rand.Reader, modele, a.cert, &key.PublicKey, a.key)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	s, err := x509svid.ParseRaw(der, pkcs8)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Source rend la source d'identité d'une charge : sa SVID (valable une
// heure) et les bundles auxquels elle fait confiance (le sien, plus ceux
// des autres autorités données).
func (a *Autorite) Source(t testing.TB, id string, aussi ...*Autorite) *identity.Source {
	t.Helper()
	return a.SourceSVID(t, a.SVID(t, id, time.Now().Add(-time.Minute), time.Now().Add(time.Hour)), aussi...)
}

// SourceSVID est Source avec une SVID donnée (expirée, par exemple).
func (a *Autorite) SourceSVID(t testing.TB, s *x509svid.SVID, aussi ...*Autorite) *identity.Source {
	t.Helper()
	bundles := []*x509bundle.Bundle{a.Bundle()}
	for _, o := range aussi {
		bundles = append(bundles, o.Bundle())
	}
	return identity.New(s, x509bundle.NewSet(bundles...))
}

// Servir sert h derrière mTLS SPIFFE avec la source donnée (qui accepte tout
// client de ses bundles) et rend l'adresse « 127.0.0.1:port ». Pas de
// httptest : son StartTLS impose son propre certificat quand l'adresse est
// une IP, et masquerait la SVID.
func Servir(t testing.TB, src *identity.Source, h http.Handler) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hs := &http.Server{Handler: h, ErrorLog: log.New(io.Discard, "", 0)} // poignées de main refusées exprès
	go hs.Serve(tls.NewListener(l, src.ServerTLS(tlsconfig.AuthorizeAny())))
	t.Cleanup(func() { hs.Close() })
	return l.Addr().String()
}
