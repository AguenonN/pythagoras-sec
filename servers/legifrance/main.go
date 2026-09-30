// Commande legifrance : serveur MCP adaptateur de l'API Légifrance (DILA)
// via PISTE, OAuth client_credentials. Identifiants dans PISTE_CLIENT_ID et
// PISTE_CLIENT_SECRET (inutiles en REJEU). Mode fixé par DONNEES_MODE.
package main

import (
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/lanceur"
	"mcprogue/servers/internal/rejeu"
)

func main() {
	lanceur.Lancer(Nom, func(c lanceur.Config) (*mcp.Server, error) {
		var jeton *Jeton
		if c.Mode != rejeu.Rejeu {
			jeton = &Jeton{
				HTTP: &http.Client{
					Timeout: 30 * time.Second,
					// Aucune redirection : le secret ne part que vers
					// oauth.piste.gouv.fr.
					CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
				},
				Maintenant: time.Now,
			}
		}
		return Serveur(c.Client(), jeton, time.Now), nil
	})
}
