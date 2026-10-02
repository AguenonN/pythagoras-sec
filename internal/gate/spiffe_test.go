package gate

import (
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
