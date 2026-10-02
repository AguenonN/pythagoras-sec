package backend

import (
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
)

// connecterSpiffe joint un serveur HTTP streamable en mTLS SPIFFE. Le gate
// présente sa propre SVID, obtenue de la Workload API ; le serveur doit
// présenter une SVID X.509 qui remonte au bundle de confiance et dont
// l'identité SPIFFE est exactement celle que le registre épingle
// (attestation.attendu). Le nom d'hôte de l'url n'est pas consulté : en
// SPIFFE, l'identité est dans le SAN URI, pas dans le DNS.
//
// Comme pour le pin TLS, la vérification a lieu à chaque poignée de main.
// L'identité observée est celle du premier serveur qui a présenté une SVID
// valide (même refusée par le registre : l'attestation la consigne) ; le
// trust domain et l'expiration de la SVID vont dans le détail de
// l'attestation. Ni proxy ni redirection.
func (c *client) connecterSpiffe(ctx context.Context) error {
	if c.opts.Spiffe == nil {
		return c.echec("", "aucune identité SPIFFE pour le gate (Workload API non configurée)", nil)
	}
	attendu, err := spiffeid.FromString(c.s.Attestation.Attendu)
	if err != nil {
		return c.echec("", "identité SPIFFE attendue illisible", err)
	}
	var mu sync.Mutex
	observe, detail := "", ""
	autorise := func(id spiffeid.ID, chaines [][]*x509.Certificate) error {
		mu.Lock()
		if observe == "" {
			observe = id.String()
			detail = "trust domain " + id.TrustDomain().String()
			if len(chaines) > 0 && len(chaines[0]) > 0 {
				detail += " ; SVID du serveur expire le " + chaines[0][0].NotAfter.UTC().Format(time.RFC3339)
			}
		}
		mu.Unlock()
		return tlsconfig.AuthorizeID(attendu)(id, chaines)
	}
	transport := &http.Transport{
		Proxy:             nil,
		TLSClientConfig:   c.opts.Spiffe.ClientTLS(autorise),
		ForceAttemptHTTP2: true,
	}
	hc := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("redirection refusée : l'engine ne parle qu'à l'url du registre")
		},
	}
	c.mu.Lock()
	c.liberer = transport.CloseIdleConnections
	c.mu.Unlock()
	err = c.etablir(ctx, &mcp.StreamableClientTransport{Endpoint: c.s.URL, HTTPClient: hc},
		func() string { mu.Lock(); defer mu.Unlock(); return observe })
	mu.Lock()
	d := detail
	mu.Unlock()
	c.mu.Lock()
	if c.att.Detail == "" && d != "" && c.att.Observe != "" {
		// Conforme : le détail ne dit que ce qui a été observé. En échec, il
		// garde son motif.
		c.att.Detail = d
	}
	c.mu.Unlock()
	return err
}
