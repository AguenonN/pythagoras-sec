package backend

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/backendtest"
	"mcprogue/internal/directive"
)

// serveurHTTP sert un profil en HTTP streamable derrière TLS et rend son
// url, le pin SPKI de son certificat et un compteur des requêtes HTTP
// reçues.
func serveurHTTP(t *testing.T, profil string) (url, spki string, requetes *atomic.Int64) {
	t.Helper()
	srv := backendtest.Serveur(profil)
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	requetes = new(atomic.Int64)
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requetes.Add(1)
		h.ServeHTTP(w, r)
	}))
	ts.EnableHTTP2 = true
	ts.Config.ErrorLog = log.New(io.Discard, "", 0) // poignées de main refusées exprès
	ts.StartTLS()
	t.Cleanup(ts.Close)
	somme := sha256.Sum256(ts.Certificate().RawSubjectPublicKeyInfo)
	return ts.URL + "/mcp", "sha256:" + hex.EncodeToString(somme[:]), requetes
}

// Le certificat de httptest couvre example.com et 127.0.0.1.
const nomTLS = "example.com"

func TestHTTP_Conforme(t *testing.T) {
	url, spki, _ := serveurHTTP(t, backendtest.Banque)
	b, err := Connecter(ctxTest(t), backendtest.HTTP("banque-synthetique", backendtest.Banque, url, spki, nomTLS), Options{})
	t.Cleanup(func() { b.Fermer() })
	if err != nil {
		t.Fatal(err)
	}
	a := b.Attestation()
	if !a.Valide() || a.Mode != "tls-pin" || a.Observe != spki || !a.Schemas {
		t.Fatalf("attestation %+v", a)
	}
	r, err := b.Appeler(ctxTest(t), "attribuer_pret", map[string]any{"montant": 1000.0, "siren": "552100554"}, nil)
	if err != nil || r.IsError || !strings.Contains(texte(t, r), "prêt de 1000 accordé à 552100554") {
		t.Fatalf("%+v %v", r, err)
	}
	if err := b.Fermer(); err != nil {
		t.Fatal(err)
	}
}

// Une clé TLS qui n'est pas celle du registre : la poignée de main échoue,
// aucune requête HTTP n'atteint le serveur.
func TestHTTP_PinDifferent(t *testing.T) {
	url, spki, requetes := serveurHTTP(t, backendtest.Banque)
	autre := "sha256:" + strings.Repeat("0", 64)
	b, err := Connecter(ctxTest(t), backendtest.HTTP("banque-synthetique", backendtest.Banque, url, autre, nomTLS), Options{})
	t.Cleanup(func() { b.Fermer() })
	if !errors.Is(err, ErrNonConnecte) {
		t.Fatalf("erreur %v", err)
	}
	a := b.Attestation()
	if a.Valide() || a.Observe != spki || a.Attendu != autre || a.Detail != "connexion MCP impossible" {
		t.Fatalf("attestation %+v", a)
	}
	if n := requetes.Load(); n != 0 {
		t.Fatalf("%d requête(s) HTTP ont atteint un serveur non épinglé", n)
	}
	if _, err := b.Appeler(ctxTest(t), "attribuer_pret", nil, nil); !errors.Is(err, ErrNonAttest) {
		t.Fatalf("erreur %v", err)
	}
}

// La bonne clé, mais un nom imposé que le certificat ne couvre pas.
func TestHTTP_NomNonCouvert(t *testing.T) {
	url, spki, requetes := serveurHTTP(t, backendtest.Banque)
	b, err := Connecter(ctxTest(t), backendtest.HTTP("banque-synthetique", backendtest.Banque, url, spki, "banque.lab"), Options{})
	t.Cleanup(func() { b.Fermer() })
	if !errors.Is(err, ErrNonConnecte) || !strings.Contains(err.Error(), "nom imposé") {
		t.Fatalf("erreur %v", err)
	}
	if b.Attestation().Valide() || requetes.Load() != 0 {
		t.Fatalf("attestation %+v, %d requête(s)", b.Attestation(), requetes.Load())
	}
}

// Le bon serveur TLS qui annonce un catalogue non épinglé.
func TestHTTP_CatalogueNonConforme(t *testing.T) {
	url, spki, _ := serveurHTTP(t, backendtest.Imposteur)
	b, err := Connecter(ctxTest(t), backendtest.HTTP("taux-officiel", backendtest.Imposteur, url, spki, nomTLS), Options{})
	t.Cleanup(func() { b.Fermer() })
	if !errors.Is(err, ErrNonAttest) {
		t.Fatalf("erreur %v", err)
	}
	if a := b.Attestation(); a.Valide() || a.Observe != spki || a.Schemas {
		t.Fatalf("attestation %+v", a)
	}
}

// tools/list_changed parvient aussi par HTTP streamable.
func TestHTTP_DeriveQuarantaine(t *testing.T) {
	url, spki, _ := serveurHTTP(t, backendtest.Taux)
	changements := make(chan directive.Attestation, 8)
	b, err := Connecter(ctxTest(t), backendtest.HTTP("taux-officiel", backendtest.Taux, url, spki, nomTLS), Options{
		AuChangement: func(_ string, a directive.Attestation) { changements <- a },
	})
	t.Cleanup(func() { b.Fermer() })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Appeler(ctxTest(t), "get_taux", map[string]any{"devise": "DERIVE"}, nil); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case a := <-changements:
			if strings.Contains(a.Detail, "quarantaine") {
				if _, err := b.Appeler(ctxTest(t), "get_taux", map[string]any{"devise": "EUR"}, nil); !errors.Is(err, ErrNonAttest) {
					t.Fatalf("erreur %v", err)
				}
				return
			}
		case <-time.After(10 * time.Second):
			t.Fatal("pas de quarantaine")
		}
	}
}
