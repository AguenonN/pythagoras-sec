package backend

import (
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"

	"mcprogue/internal/backendtest"
	"mcprogue/internal/identity"
	"mcprogue/internal/identitytest"
	"mcprogue/internal/registre"
)

const (
	idServeur = "spiffe://lab.test/serveur/banque-synthetique"
	idGate    = "spiffe://lab.test/gate"
)

// serveurSpiffe sert la banque en HTTP streamable derrière mTLS SPIFFE, avec
// la source d'identité donnée ; rend l'url, le compteur de requêtes HTTP et
// l'identité SPIFFE du dernier client qui a présenté une SVID.
func serveurSpiffe(t *testing.T, src *identity.Source) (url string, requetes *atomic.Int64, client func() string) {
	t.Helper()
	srv := backendtest.Serveur(backendtest.Banque)
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	requetes = new(atomic.Int64)
	var mu sync.Mutex
	vu := ""
	hs := &http.Server{
		ErrorLog: log.New(io.Discard, "", 0), // poignées de main refusées exprès
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requetes.Add(1)
			if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 && len(r.TLS.PeerCertificates[0].URIs) > 0 {
				mu.Lock()
				vu = r.TLS.PeerCertificates[0].URIs[0].String()
				mu.Unlock()
			}
			h.ServeHTTP(w, r)
		}),
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go hs.Serve(tls.NewListener(l, src.ServerTLS(tlsconfig.AuthorizeAny())))
	t.Cleanup(func() { hs.Close() })
	return "https://" + l.Addr().String() + "/mcp", requetes, func() string { mu.Lock(); defer mu.Unlock(); return vu }
}

func serveurRegistre(url, attendu string) registre.Serveur {
	return registre.Serveur{
		Nom: "banque-synthetique", Transport: registre.HTTPSpiffe, URL: url,
		Attestation: registre.Attestation{Mode: registre.ModeSpiffe, Attendu: attendu},
		Outils:      backendtest.Outils(backendtest.Banque),
	}
}

func TestSpiffe_Conforme(t *testing.T) {
	ac := identitytest.Nouvelle(t, "lab.test")
	url, _, client := serveurSpiffe(t, ac.Source(t, idServeur))
	b, err := Connecter(ctxTest(t), serveurRegistre(url, idServeur), Options{Spiffe: ac.Source(t, idGate)})
	t.Cleanup(func() { b.Fermer() })
	if err != nil {
		t.Fatal(err)
	}
	a := b.Attestation()
	if !a.Valide() || a.Mode != "spiffe" || a.Observe != idServeur || !a.Schemas {
		t.Fatalf("attestation %+v", a)
	}
	// L'attestation consigne le trust domain et l'expiration de la SVID.
	if !strings.Contains(a.Detail, "trust domain lab.test") || !strings.Contains(a.Detail, "SVID du serveur expire le ") {
		t.Fatalf("détail %q", a.Detail)
	}
	r, err := b.Appeler(ctxTest(t), "attribuer_pret", map[string]any{"montant": 1000.0, "siren": "552100554"}, nil)
	if err != nil || r.IsError || !strings.Contains(texte(t, r), "prêt de 1000 accordé à 552100554") {
		t.Fatalf("%+v %v", r, err)
	}
	// mTLS : le serveur a vu l'identité du gate.
	if client() != idGate {
		t.Fatalf("client vu par le serveur : %q", client())
	}
}

// Une SVID valide du même trust domain, mais une autre identité : refusée,
// aucune requête HTTP n'atteint le serveur, l'identité observée est consignée.
func TestSpiffe_IdentiteDifferente(t *testing.T) {
	ac := identitytest.Nouvelle(t, "lab.test")
	autre := "spiffe://lab.test/serveur/imposteur"
	url, requetes, _ := serveurSpiffe(t, ac.Source(t, autre))
	b, err := Connecter(ctxTest(t), serveurRegistre(url, idServeur), Options{Spiffe: ac.Source(t, idGate)})
	t.Cleanup(func() { b.Fermer() })
	if !errors.Is(err, ErrNonConnecte) {
		t.Fatalf("erreur %v", err)
	}
	a := b.Attestation()
	if a.Valide() || a.Observe != autre || a.Attendu != idServeur || a.Detail != "connexion MCP impossible" {
		t.Fatalf("attestation %+v", a)
	}
	if n := requetes.Load(); n != 0 {
		t.Fatalf("%d requête(s) HTTP ont atteint un serveur d'une autre identité", n)
	}
	if _, err := b.Appeler(ctxTest(t), "attribuer_pret", nil, nil); !errors.Is(err, ErrNonAttest) {
		t.Fatalf("erreur %v", err)
	}
}

// Le bon SPIFFE ID, mais une SVID émise par une autorité que le gate ne
// connaît pas : la chaîne ne remonte pas à son bundle.
func TestSpiffe_AutoriteInconnue(t *testing.T) {
	ac, rogue := identitytest.Nouvelle(t, "lab.test"), identitytest.Nouvelle(t, "lab.test")
	url, requetes, _ := serveurSpiffe(t, rogue.Source(t, idServeur, ac))
	b, err := Connecter(ctxTest(t), serveurRegistre(url, idServeur), Options{Spiffe: ac.Source(t, idGate)})
	t.Cleanup(func() { b.Fermer() })
	if !errors.Is(err, ErrNonConnecte) {
		t.Fatalf("erreur %v", err)
	}
	if a := b.Attestation(); a.Valide() || a.Observe != "" {
		t.Fatalf("attestation %+v", a)
	}
	if requetes.Load() != 0 {
		t.Fatal("requête HTTP vers un serveur d'une autorité inconnue")
	}
}

// Le bon SPIFFE ID et la bonne autorité, mais une SVID expirée.
func TestSpiffe_SVIDExpiree(t *testing.T) {
	ac := identitytest.Nouvelle(t, "lab.test")
	vieille := ac.SVID(t, idServeur, time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))
	url, requetes, _ := serveurSpiffe(t, ac.SourceSVID(t, vieille))
	b, err := Connecter(ctxTest(t), serveurRegistre(url, idServeur), Options{Spiffe: ac.Source(t, idGate)})
	t.Cleanup(func() { b.Fermer() })
	if !errors.Is(err, ErrNonConnecte) {
		t.Fatalf("erreur %v", err)
	}
	if a := b.Attestation(); a.Valide() {
		t.Fatalf("attestation %+v", a)
	}
	if requetes.Load() != 0 {
		t.Fatal("requête HTTP vers un serveur à SVID expirée")
	}
}

// Une SVID d'un autre trust domain (bundle inconnu) ne passe pas non plus.
func TestSpiffe_AutreTrustDomain(t *testing.T) {
	ac, autre := identitytest.Nouvelle(t, "lab.test"), identitytest.Nouvelle(t, "ailleurs.test")
	url, _, _ := serveurSpiffe(t, autre.Source(t, "spiffe://ailleurs.test/serveur/banque-synthetique", ac))
	b, err := Connecter(ctxTest(t), serveurRegistre(url, idServeur), Options{Spiffe: ac.Source(t, idGate)})
	t.Cleanup(func() { b.Fermer() })
	if !errors.Is(err, ErrNonConnecte) || b.Attestation().Valide() {
		t.Fatalf("erreur %v, attestation %+v", err, b.Attestation())
	}
}

// Sans source d'identité, un serveur http-spiffe ne se connecte pas.
func TestSpiffe_SansSource(t *testing.T) {
	b, err := Connecter(ctxTest(t), serveurRegistre("https://127.0.0.1:1/mcp", idServeur), Options{})
	if !errors.Is(err, ErrNonConnecte) || b.Attestation().Valide() {
		t.Fatalf("erreur %v", err)
	}
}
