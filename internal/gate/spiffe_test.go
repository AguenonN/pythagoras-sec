package gate

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spiffe/go-spiffe/v2/spiffeid"

	"mcprogue/internal/backend"
	"mcprogue/internal/backendtest"
	"mcprogue/internal/identitytest"
	"mcprogue/internal/journal"
	"mcprogue/internal/registre"
)

// Un serveur http-spiffe, joint par le vrai backend : l'attestation figée à
// la connexion consigne le mode spiffe, l'identité observée, le trust domain
// et l'expiration de la SVID.
func TestSpiffe_AttestationConsignee(t *testing.T) {
	const idServeur = "spiffe://lab.test/serveur/banque-synthetique"
	ac := identitytest.Nouvelle(t, "lab.test")
	srv := backendtest.Serveur(backendtest.Banque)
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	adresse := identitytest.Servir(t, ac.Source(t, idServeur), h)
	b := monter(t, func(b *banc) {
		b.reg.Serveurs[1] = registre.Serveur{
			Nom: "banque-synthetique", Transport: registre.HTTPSpiffe, URL: "https://" + adresse + "/mcp",
			Attestation: registre.Attestation{Mode: registre.ModeSpiffe, Attendu: idServeur},
			Outils:      backendtest.Outils(backendtest.Banque),
		}
		b.config = func(c *Config) {
			c.Connecter = nil
			c.Backend = backend.Options{Spiffe: ac.Source(t, "spiffe://lab.test/gate")}
		}
	})
	trouve := false
	for _, e := range b.entrees(t) {
		if e.Type == journal.AttestationT && e.Serveur == "banque-synthetique" {
			a := e.Attestation
			trouve = true
			if !a.Valide || a.Mode != "spiffe" || a.Observe != idServeur || !strings.Contains(a.Detail, "trust domain lab.test") ||
				!strings.Contains(a.Detail, "expire le ") {
				t.Fatalf("attestation consignée %+v", a)
			}
		}
	}
	if !trouve {
		t.Fatal("pas d'entrée attestation")
	}
}

const (
	idAgentA = "spiffe://lab.test/agent/a"
	idAgentB = "spiffe://lab.test/agent/b"
)

// roulant ajoute des en-têtes à chaque requête sortante.
type roulant struct {
	base  http.RoundTripper
	ajout map[string]string
}

func (r roulant) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	for k, v := range r.ajout {
		req.Header.Set(k, v)
	}
	return r.base.RoundTrip(req)
}

// monterSpiffeAmont sert un gate en HTTP + mTLS SPIFFE ; rend le banc, l'url
// et l'autorité.
func monterSpiffeAmont(t *testing.T) (*banc, string, *identitytest.Autorite) {
	t.Helper()
	ac := identitytest.Nouvelle(t, "lab.test")
	b := monter(t, func(b *banc) {
		b.config = func(c *Config) { c.Appelant, c.AppelantSpiffe = "", true }
	})
	adresse := identitytest.Servir(t, ac.Source(t, "spiffe://lab.test/gate"), b.g.HandlerSpiffe())
	return b, "https://" + adresse + "/mcp", ac
}

func clientSpiffe(t *testing.T, url string, ac *identitytest.Autorite, id string, ajout map[string]string) *mcp.ClientSession {
	t.Helper()
	hc := ac.Source(t, id).ClientHTTP(spiffeid.RequireFromString("spiffe://lab.test/gate"))
	if ajout != nil {
		hc.Transport = roulant{base: hc.Transport, ajout: ajout}
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "0"}, nil).
		Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: hc}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// L'appelant consigné est l'identité SPIFFE du client, prouvée par mTLS ; un
// en-tête d'appelant fourni par le client n'y change rien.
func TestSpiffeAmont_AppelantProuve(t *testing.T) {
	b, url, ac := monterSpiffeAmont(t)
	cs := clientSpiffe(t, url, ac, idAgentA, map[string]string{headerAppelant: idAgentB})
	appelerAgent(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"})
	ouvertes, appels := 0, 0
	for _, e := range b.entrees(t) {
		switch e.Type {
		case journal.SessionOuverte:
			ouvertes++
			if e.Appelant != idAgentA {
				t.Fatalf("session ouverte par %q", e.Appelant)
			}
		case journal.AppelT:
			appels++
			if e.Appelant != idAgentA {
				t.Fatalf("appel de %q", e.Appelant)
			}
		}
	}
	if ouvertes != 1 || appels != 1 {
		t.Fatalf("%d session(s), %d appel(s)", ouvertes, appels)
	}
}

// Sans certificat client SPIFFE, pas de poignée de main.
func TestSpiffeAmont_SansCertificat(t *testing.T) {
	_, url, _ := monterSpiffeAmont(t)
	hc := identitytest.Nouvelle(t, "lab.test").Source(t, "spiffe://lab.test/gate").ClientHTTP(spiffeid.RequireFromString("spiffe://lab.test/gate"))
	hc.Transport.(*http.Transport).TLSClientConfig.GetClientCertificate = nil
	hc.Transport.(*http.Transport).TLSClientConfig.Certificates = nil
	_, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "0"}, nil).
		Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: hc}, nil)
	if err == nil {
		t.Fatal("connexion acceptée sans identité")
	}
}

// Un autre client qui rejoue l'identifiant de session de A, avec sa propre
// SVID : refusé, et consigné.
func TestSpiffeAmont_SessionVolee(t *testing.T) {
	b, url, ac := monterSpiffeAmont(t)
	cs := clientSpiffe(t, url, ac, idAgentA, nil)
	appelerAgent(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"})
	hc := ac.Source(t, idAgentB).ClientHTTP(spiffeid.RequireFromString("spiffe://lab.test/gate"))
	req, _ := http.NewRequest("POST", url, strings.NewReader(
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"taux-officiel__get_taux","arguments":{"devise":"EUR"}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", cs.ID())
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	corps, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(corps), "taux") && !strings.Contains(string(corps), "refus") && !strings.Contains(string(corps), "isError") {
		t.Fatalf("réponse %d : %s", resp.StatusCode, corps)
	}
	changee, appels := false, 0
	for _, e := range b.entrees(t) {
		changee = changee || (e.Type == journal.IdentiteChangee && e.Appelant == idAgentB)
		if e.Type == journal.AppelT {
			appels++
		}
	}
	if !changee || appels != 1 {
		t.Fatalf("identite_changee=%v, %d appel(s) ; réponse %d : %s", changee, appels, resp.StatusCode, corps)
	}
}
