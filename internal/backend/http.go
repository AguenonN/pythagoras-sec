package backend

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connecterHTTP joint un serveur HTTP streamable. L'identité du serveur est
// la clé publique de son certificat feuille, épinglée au registre
// (sha256 de la SubjectPublicKeyInfo DER), et le nom imposé à la poignée
// de main doit figurer dans le certificat. La chaîne de certification n'est
// pas consultée : le pin la remplace, et une AC compromise ou complaisante
// ne suffit pas à se faire passer pour le serveur.
//
// La vérification a lieu à chaque poignée de main, pas seulement à la
// première : une connexion ouverte plus tard vers une autre clé échoue.
// Ni proxy ni redirection : l'engine parle au serveur du registre, et à lui
// seul.
func (c *client) connecterHTTP(ctx context.Context) error {
	attendu, nom := c.s.Attestation.Attendu, c.s.Attestation.ServerName
	var mu sync.Mutex
	premiere := ""
	observe := func() string {
		mu.Lock()
		defer mu.Unlock()
		return premiere
	}
	tlsConf := &tls.Config{
		ServerName: nom,
		MinVersion: tls.VersionTLS12,
		// La vérification par défaut (chaîne et nom) est remplacée par
		// VerifyConnection, qui épingle la clé et vérifie le nom.
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("aucun certificat présenté")
			}
			feuille := cs.PeerCertificates[0]
			somme := sha256.Sum256(feuille.RawSubjectPublicKeyInfo)
			spki := "sha256:" + hex.EncodeToString(somme[:])
			mu.Lock()
			if premiere == "" {
				premiere = spki
			}
			mu.Unlock()
			if spki != attendu {
				return fmt.Errorf("clé TLS du serveur non épinglée (observée %s)", spki)
			}
			if err := feuille.VerifyHostname(nom); err != nil {
				return fmt.Errorf("le certificat ne couvre pas le nom imposé %q", nom)
			}
			return nil
		},
	}
	transport := &http.Transport{
		Proxy:             nil,
		TLSClientConfig:   tlsConf,
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
	return c.etablir(ctx, &mcp.StreamableClientTransport{Endpoint: c.s.URL, HTTPClient: hc}, observe)
}
