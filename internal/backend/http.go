package backend

import "context"

// connecterHTTP joindra un serveur HTTP streamable, TLS épinglé.
func (c *client) connecterHTTP(ctx context.Context) error {
	return c.echec("", "transport http pas encore câblé")
}
