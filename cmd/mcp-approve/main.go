// Command mcp-approve mints one approval token: the operator-side half of the
// _meta.approvalToken mechanism. In production this role belongs to an
// SSO/OIDC-backed signer, so that "approved_by" is an authenticated identity;
// this CLI is the lab and break-glass shape.
//
//	mcp-approve -key-file hmac.key -by alice@example.com -ticket INC-4242 \
//	    -tool drop_db -args '{"database":"staging"}'
//
// -principal and -actor add the mandate: on whose behalf the act is performed
// and which delegate performs it, so the trail answers "who had it done" and
// not only "who approved".
//
// Subcommands annuaire-demo, identite and valider work with the directory of
// authorisations (internal/annuaire): see annuaire.go.
//
// It prints the action digest and a JWT valid for -ttl (default and maximum
// policy: 15m), bound to exactly that tool+arguments. The agent then passes
// the token in the tools/call params under _meta.approvalToken.
package main

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"mcprogue/internal/approval"
)

func main() {
	if len(os.Args) > 1 {
		if f, ok := sousCommandes[os.Args[1]]; ok {
			if err := f(os.Args[2:], os.Stdout); err != nil {
				log.Fatalf("mcp-approve %s: %v", os.Args[1], err)
			}
			return
		}
	}
	keyFile := flag.String("key-file", "", "signing key: an HMAC secret, or a PEM PKCS#8 private key")
	by := flag.String("by", "", "who approves (goes into sub and the audit trail)")
	ticket := flag.String("ticket", "", "change/incident reference (optional)")
	issuer := flag.String("issuer", "", "iss claim (optional, must match APPROVAL_JWT_ISSUER when set)")
	tool := flag.String("tool", "", "tool name being approved")
	args := flag.String("args", "{}", "exact JSON arguments of the call being approved")
	principal := flag.String("principal", "", "on whose behalf the act is performed (the mandate)")
	actor := flag.String("actor", "", "the delegate performing it, e.g. the agent's SPIFFE ID")
	ttl := flag.Duration("ttl", approval.DefaultMaxTTL, "token lifetime")
	flag.Parse()

	if *keyFile == "" || *by == "" || *tool == "" {
		flag.Usage()
		log.Fatal("mcp-approve: -key-file, -by and -tool are required")
	}

	var arguments map[string]any
	if err := json.Unmarshal([]byte(*args), &arguments); err != nil {
		log.Fatalf("mcp-approve: -args is not a JSON object: %v", err)
	}
	digest := approval.Digest(*tool, arguments)

	if *actor != "" && *principal == "" {
		log.Fatal("mcp-approve: -actor names a delegate but -principal names nobody to delegate from; a mandate with no mandator is just a permission")
	}
	token, err := approval.MintGrant(loadKey(*keyFile), approval.Grant{
		ApprovedBy:   *by,
		ActionDigest: digest,
		TicketID:     *ticket,
		Issuer:       *issuer,
		Principal:    *principal,
		Actor:        *actor,
		TTL:          *ttl,
	})
	if err != nil {
		log.Fatalf("mcp-approve: %v", err)
	}
	fmt.Printf("action_digest: %s\napprovalToken: %s\n", digest, token)
}

// loadKey reads the signing key: a PEM PKCS#8 private key if the file parses
// as one, otherwise the raw bytes as an HMAC secret.
func loadKey(path string) any {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("mcp-approve: read key: %v", err)
	}
	if block, _ := pem.Decode(b); block != nil {
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			log.Fatalf("mcp-approve: parse PEM private key: %v", err)
		}
		return key
	}
	secret := []byte(strings.TrimSpace(string(b)))
	if len(secret) < 32 {
		log.Fatalf("mcp-approve: HMAC secret is %d bytes; below 32 it is guessable", len(secret))
	}
	return secret
}
