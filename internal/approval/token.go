package approval

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"fmt"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// DefaultMaxTTL bounds how long an approval token may live. Fifteen minutes
// is a human pressing "approve" and the agent acting on it, not a standing
// permission that outlives the incident it was minted for.
const DefaultMaxTTL = 15 * time.Minute

// TokenClaims is the payload an operator's identity provider (or
// cmd/mcp-approve) signs. The registered claims carry who and until when; the
// private claims bind the token to one exact call and to its paper trail.
type TokenClaims struct {
	jwt.Claims
	// ActionDigest must equal Digest(tool, arguments) of the call being
	// approved. A token minted for "drop_db staging" is inert against
	// "drop_db prod": the digest differs.
	ActionDigest string `json:"action_digest"`
	// TicketID references the change or incident record.
	TicketID string `json:"ticket_id,omitempty"`
	// Principal is on whose behalf the act is performed — the mandate. It
	// answers a question sub does not: sub says who approved this call,
	// Principal says in whose name the agent is acting at all.
	//
	// RFC 8693 puts the party acted for in sub and the delegate in act.sub.
	// This token cannot: sub is already spent naming the approver, and
	// silently changing what sub means would rewrite the meaning of every
	// audit entry ever written. Hence an explicit principal claim, with act
	// keeping RFC 8693's shape for the delegate.
	Principal string `json:"principal,omitempty"`
	// Act names the party authorised to act for Principal (RFC 8693 §4.1).
	Act *Actor `json:"act,omitempty"`
}

// Actor is the RFC 8693 "act" claim: the delegate acting for the principal.
type Actor struct {
	Subject string `json:"sub,omitempty"`
}

// Token verifies caller-supplied approval JWTs (_meta.approvalToken). The
// verification key belongs to the operators' signing authority — an OIDC/SSO
// backed service, or the shared secret cmd/mcp-approve uses in the lab.
type Token struct {
	// Key verifies signatures: []byte (HMAC), *rsa.PublicKey,
	// *ecdsa.PublicKey or ed25519.PublicKey.
	Key any
	// MaxTTL caps exp-iat. Zero uses DefaultMaxTTL. A token signed with a
	// year of validity is refused even though its signature is good: the
	// signer's intent does not override the proxy's policy.
	MaxTTL time.Duration
	// Issuer, when set, must match the iss claim.
	Issuer string
	// RequireMandate refuses a token that names no principal. Off by default,
	// because a token without a mandate is still a valid approval of an
	// action; on, it makes "no act without a named mandator" a property of
	// the deployment rather than a convention.
	RequireMandate bool
	// now is stubbed in tests.
	now func() time.Time
}

func (t *Token) Name() string { return "token" }

func (t *Token) Approve(_ context.Context, req Request) (Decision, error) {
	if req.Token == "" {
		return Decision{}, fmt.Errorf("no approval token: the call must carry an operator-signed JWT in _meta.approvalToken")
	}
	algs, err := algorithmsFor(t.Key)
	if err != nil {
		return Decision{}, err
	}
	tok, err := jwt.ParseSigned(req.Token, algs)
	if err != nil {
		return Decision{}, fmt.Errorf("unparseable approval token: %w", err)
	}
	var claims TokenClaims
	if err := tok.Claims(t.Key, &claims); err != nil {
		return Decision{}, fmt.Errorf("approval token signature rejected: %w", err)
	}

	now := time.Now
	if t.now != nil {
		now = t.now
	}
	if err := claims.Validate(jwt.Expected{Issuer: t.Issuer, Time: now()}); err != nil {
		return Decision{}, fmt.Errorf("approval token rejected: %w", err)
	}
	if claims.Expiry == nil || claims.IssuedAt == nil {
		return Decision{}, fmt.Errorf("approval token carries no exp/iat: an approval without an expiry is a standing permission")
	}
	maxTTL := t.MaxTTL
	if maxTTL <= 0 {
		maxTTL = DefaultMaxTTL
	}
	if ttl := claims.Expiry.Time().Sub(claims.IssuedAt.Time()); ttl > maxTTL {
		return Decision{}, fmt.Errorf("approval token lives %s, longer than the %s policy allows", ttl, maxTTL)
	}
	if claims.Subject == "" {
		return Decision{}, fmt.Errorf("approval token names no approver (empty sub)")
	}
	if claims.ActionDigest != req.Digest {
		return Decision{}, fmt.Errorf("approval token is for another action (token digest %s, this call is %s)",
			claims.ActionDigest, req.Digest)
	}
	if t.RequireMandate && claims.Principal == "" {
		return Decision{}, fmt.Errorf("approval token names no principal: this deployment requires every act to carry the mandate it is performed under")
	}
	actor := ""
	if claims.Act != nil {
		actor = claims.Act.Subject
	}
	// The delegate named in the token must be the one making the call — but
	// only where the caller's identity was actually proven. Over stdio there
	// is no peer to verify (lock 0 has no handshake to run), so req.Actor is
	// empty and act.sub is recorded as a declaration rather than enforced as
	// a check. The audit entry is the same either way; what differs is what
	// it is worth, and that difference belongs in the deployment's writeup,
	// not hidden in a silent pass here.
	if actor != "" && req.Actor != "" && actor != req.Actor {
		return Decision{}, fmt.Errorf("approval token delegates to %q, but the caller proved it is %q",
			actor, req.Actor)
	}
	return Decision{
		Approver:   "token",
		ApprovedBy: claims.Subject,
		TicketID:   claims.TicketID,
		Principal:  claims.Principal,
		Actor:      actor,
	}, nil
}

// algorithmsFor derives the acceptable signature algorithms from the key
// type, so an attacker cannot pick the algorithm (the alg-confusion class of
// JWT bugs: an RSA public key used as an HMAC secret).
func algorithmsFor(key any) ([]jose.SignatureAlgorithm, error) {
	switch k := key.(type) {
	case []byte:
		return []jose.SignatureAlgorithm{jose.HS256}, nil
	case *rsa.PublicKey, *rsa.PrivateKey:
		return []jose.SignatureAlgorithm{jose.RS256, jose.PS256}, nil
	case *ecdsa.PublicKey:
		return ecdsaAlg(k.Curve)
	case *ecdsa.PrivateKey:
		return ecdsaAlg(k.Curve)
	case ed25519.PublicKey, ed25519.PrivateKey:
		return []jose.SignatureAlgorithm{jose.EdDSA}, nil
	case nil:
		return nil, fmt.Errorf("approval token verification has no key")
	default:
		return nil, fmt.Errorf("unsupported approval token key type %T", key)
	}
}

func ecdsaAlg(c elliptic.Curve) ([]jose.SignatureAlgorithm, error) {
	switch c {
	case elliptic.P256():
		return []jose.SignatureAlgorithm{jose.ES256}, nil
	case elliptic.P384():
		return []jose.SignatureAlgorithm{jose.ES384}, nil
	case elliptic.P521():
		return []jose.SignatureAlgorithm{jose.ES512}, nil
	default:
		return nil, fmt.Errorf("unsupported ECDSA curve %v for approval tokens", c)
	}
}

// Grant is what an operator signs: one approval, for one call, optionally
// under a named mandate.
type Grant struct {
	// ApprovedBy authenticates the decision (the sub claim).
	ApprovedBy string
	// ActionDigest is Digest(tool, arguments) of the call being approved.
	ActionDigest string
	// TicketID references the change or incident record.
	TicketID string
	// Issuer fills iss.
	Issuer string
	// Principal is on whose behalf the act is performed; Actor is the
	// delegate performing it. Both optional, and meaningless apart: a
	// mandate with no mandator is a permission.
	Principal string
	Actor     string
	// TTL defaults to DefaultMaxTTL.
	TTL time.Duration
}

// Mint signs an approval token with no mandate. It is the original signature,
// kept because a token that only records who approved is still a valid
// approval; MintGrant is the one to reach for when the act is performed in
// someone's name.
func Mint(key any, approvedBy, actionDigest, ticketID, issuer string, ttl time.Duration) (string, error) {
	return MintGrant(key, Grant{
		ApprovedBy:   approvedBy,
		ActionDigest: actionDigest,
		TicketID:     ticketID,
		Issuer:       issuer,
		TTL:          ttl,
	})
}

// MintGrant signs an approval token. It exists for cmd/mcp-approve and for
// tests; in production the operators' SSO-backed signer plays this role. key
// is the private half ([]byte HMAC secret, *rsa.PrivateKey, *ecdsa.PrivateKey
// or ed25519.PrivateKey).
func MintGrant(key any, g Grant) (string, error) {
	ttl := g.TTL
	if ttl <= 0 {
		ttl = DefaultMaxTTL
	}
	var alg jose.SignatureAlgorithm
	switch k := key.(type) {
	case []byte:
		alg = jose.HS256
	case *rsa.PrivateKey:
		alg = jose.RS256
	case *ecdsa.PrivateKey:
		algs, err := ecdsaAlg(k.Curve)
		if err != nil {
			return "", err
		}
		alg = algs[0]
	case ed25519.PrivateKey:
		alg = jose.EdDSA
	default:
		return "", fmt.Errorf("unsupported approval token signing key type %T", key)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: alg, Key: key},
		(&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return "", fmt.Errorf("build approval token signer: %w", err)
	}
	now := time.Now()
	claims := TokenClaims{
		Claims: jwt.Claims{
			Subject:  g.ApprovedBy,
			Issuer:   g.Issuer,
			IssuedAt: jwt.NewNumericDate(now),
			Expiry:   jwt.NewNumericDate(now.Add(ttl)),
		},
		ActionDigest: g.ActionDigest,
		TicketID:     g.TicketID,
		Principal:    g.Principal,
	}
	if g.Actor != "" {
		claims.Act = &Actor{Subject: g.Actor}
	}
	return jwt.Signed(signer).Claims(claims).Serialize()
}
