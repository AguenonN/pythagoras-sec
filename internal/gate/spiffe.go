package gate

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// headerAppelant porte, de HandlerSpiffe au gestionnaire d'outils, le SPIFFE
// ID prouvé par la poignée de main mTLS. HandlerSpiffe le retire de toute
// requête entrante avant de le poser : un client ne peut jamais en fournir
// la valeur.
const headerAppelant = "X-Mcp-Gate-Appelant-Spiffe"

// HandlerSpiffe sert le gate en HTTP streamable. Il doit l'être derrière une
// configuration TLS qui exige et vérifie un certificat client SPIFFE
// (identity.Source.ServerTLS avec un autorisant les seuls appelants admis) :
// l'appelant consigné est le SPIFFE ID de ce certificat, et une requête sans
// certificat SPIFFE reçoit 401. La session MCP est liée à l'appelant à sa
// création : un autre client qui rejouerait son identifiant de session est
// refusé et consigné (identite_changee).
//
// Le gate doit avoir été configuré avec AppelantSpiffe.
func (g *Gate) HandlerSpiffe() http.Handler {
	mcpH := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return g.serveur }, nil)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del(headerAppelant)
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 || len(r.TLS.PeerCertificates[0].URIs) != 1 ||
			r.TLS.PeerCertificates[0].URIs[0].Scheme != "spiffe" {
			http.Error(w, "identité SPIFFE du client requise", http.StatusUnauthorized)
			return
		}
		r.Header.Set(headerAppelant, r.TLS.PeerCertificates[0].URIs[0].String())
		mcpH.ServeHTTP(w, r)
	})
}

func appelantSpiffe(req *mcp.CallToolRequest) string {
	if req == nil || req.GetExtra() == nil {
		return ""
	}
	return req.GetExtra().Header.Get(headerAppelant)
}
