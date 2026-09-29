package approval_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mcprogue/internal/approval"
)

var hmacKey = []byte("0123456789abcdef0123456789abcdef")

func req(tool string, args map[string]any, token string) approval.Request {
	return approval.Request{Tool: tool, Arguments: args, Digest: approval.Digest(tool, args), Token: token}
}

// ---------------------------------------------------------------------------
// Digest
// ---------------------------------------------------------------------------

func TestDigest_BindsToolAndArguments(t *testing.T) {
	base := approval.Digest("drop_db", map[string]any{"database": "staging"})
	if approval.Digest("drop_db", map[string]any{"database": "prod"}) == base {
		t.Fatal("digests collide across different targets")
	}
	if approval.Digest("delete_repo", map[string]any{"database": "staging"}) == base {
		t.Fatal("digests collide across different tools")
	}
	if approval.Digest("drop_db", map[string]any{"database": "staging"}) != base {
		t.Fatal("the same call digests differently twice")
	}
}

// ---------------------------------------------------------------------------
// Token approver
// ---------------------------------------------------------------------------

func TestToken_ApprovesExactlyTheMintedCall(t *testing.T) {
	args := map[string]any{"database": "staging"}
	digest := approval.Digest("drop_db", args)
	token, err := approval.Mint(hmacKey, "alice@example.com", digest, "INC-4242", "", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ap := &approval.Token{Key: hmacKey}

	dec, err := ap.Approve(context.Background(), req("drop_db", args, token))
	if err != nil {
		t.Fatalf("a valid token for this exact call was refused: %v", err)
	}
	if dec.ApprovedBy != "alice@example.com" || dec.TicketID != "INC-4242" {
		t.Fatalf("decision lost its provenance: %+v", dec)
	}

	// The same token against another target is inert.
	if _, err := ap.Approve(context.Background(), req("drop_db", map[string]any{"database": "prod"}, token)); err == nil {
		t.Fatal("a token minted for staging approved prod")
	}
	// And against another tool.
	if _, err := ap.Approve(context.Background(), req("delete_repo", args, token)); err == nil {
		t.Fatal("a token minted for drop_db approved delete_repo")
	}
}

func TestToken_Refusals(t *testing.T) {
	args := map[string]any{"database": "staging"}
	digest := approval.Digest("drop_db", args)
	ap := &approval.Token{Key: hmacKey}
	ctx := context.Background()

	// No token at all.
	if _, err := ap.Approve(ctx, req("drop_db", args, "")); err == nil {
		t.Fatal("an action with no token was approved")
	}
	// Signed with the wrong key.
	forged, err := approval.Mint([]byte("another-32-byte-key-entirely!!!!"), "mallory", digest, "", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ap.Approve(ctx, req("drop_db", args, forged)); err == nil {
		t.Fatal("a token under the wrong key was approved")
	}
	// Longer-lived than the policy allows: signature is fine, TTL is not.
	greedy, err := approval.Mint(hmacKey, "alice", digest, "", "", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ap.Approve(ctx, req("drop_db", args, greedy)); err == nil {
		t.Fatal("a 24h approval token was accepted against a 15m policy")
	}
	// Wrong issuer when one is required.
	strict := &approval.Token{Key: hmacKey, Issuer: "https://sso.corp"}
	tok, err := approval.Mint(hmacKey, "alice", digest, "", "https://evil.example", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := strict.Approve(ctx, req("drop_db", args, tok)); err == nil {
		t.Fatal("a token from the wrong issuer was accepted")
	}
}

// ---------------------------------------------------------------------------
// Webhook approver
// ---------------------------------------------------------------------------

func TestWebhook_DecisionRoundTrip(t *testing.T) {
	var seen approval.WebhookRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&seen); err != nil {
			t.Errorf("webhook payload: %v", err)
		}
		_ = json.NewEncoder(w).Encode(approval.WebhookResponse{
			Approved: true, ApprovedBy: "bob@example.com", TicketID: "CHG-7",
		})
	}))
	defer srv.Close()

	ap := &approval.Webhook{URL: srv.URL}
	r := req("drop_db", map[string]any{"database": "staging"}, "")
	dec, err := ap.Approve(context.Background(), r)
	if err != nil {
		t.Fatalf("approved decision refused: %v", err)
	}
	if dec.ApprovedBy != "bob@example.com" || dec.TicketID != "CHG-7" {
		t.Fatalf("decision lost its provenance: %+v", dec)
	}
	if seen.Tool != "drop_db" || seen.ActionDigest != r.Digest {
		t.Fatalf("the endpoint was not told what it was approving: %+v", seen)
	}
}

func TestWebhook_FailsClosed(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"refused": func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(approval.WebhookResponse{Approved: false, Reason: "nope"})
		},
		"error status": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
		"garbage body": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not json"))
		},
		"anonymous approval": func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(approval.WebhookResponse{Approved: true})
		},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			ap := &approval.Webhook{URL: srv.URL, Timeout: 5 * time.Second}
			if _, err := ap.Approve(context.Background(), req("drop_db", nil, "")); err == nil {
				t.Fatal("the webhook approver failed open")
			}
		})
	}
	t.Run("unreachable endpoint", func(t *testing.T) {
		ap := &approval.Webhook{URL: "http://127.0.0.1:1/approve", Timeout: time.Second}
		if _, err := ap.Approve(context.Background(), req("drop_db", nil, "")); err == nil {
			t.Fatal("an unreachable endpoint approved the call")
		}
	})
}

// ---------------------------------------------------------------------------
// AnyOf
// ---------------------------------------------------------------------------

type stub struct {
	name string
	err  error
}

func (s stub) Name() string { return s.name }
func (s stub) Approve(context.Context, approval.Request) (approval.Decision, error) {
	if s.err != nil {
		return approval.Decision{}, s.err
	}
	return approval.Decision{Approver: s.name, ApprovedBy: s.name}, nil
}

func TestAnyOf_OneApprovalSuffices_AllRefusalsJoin(t *testing.T) {
	ok, err := approval.AnyOf(stub{name: "a", err: context.Canceled}, stub{name: "b"})
	if err != nil {
		t.Fatal(err)
	}
	dec, err := ok.Approve(context.Background(), approval.Request{})
	if err != nil {
		t.Fatalf("one approving mechanism did not suffice: %v", err)
	}
	if dec.ApprovedBy != "b" {
		t.Fatalf("decision came from %q, want b", dec.ApprovedBy)
	}

	no, err := approval.AnyOf(stub{name: "a", err: context.Canceled}, stub{name: "b", err: context.DeadlineExceeded})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := no.Approve(context.Background(), approval.Request{}); err == nil {
		t.Fatal("every mechanism refused and the chain approved")
	} else if !strings.Contains(err.Error(), "a:") || !strings.Contains(err.Error(), "b:") {
		t.Fatalf("joined refusal %q does not name both mechanisms", err)
	}

	if _, err := approval.AnyOf(); err == nil {
		t.Fatal("AnyOf() with no approvers built a silent deny-all")
	}
}

// ---------------------------------------------------------------------------
// Audit chain
// ---------------------------------------------------------------------------

func TestAudit_ChainVerifies(t *testing.T) {
	var buf bytes.Buffer
	a := approval.NewAudit(&buf, hmacKey)
	entries := []approval.Entry{
		{Tool: "drop_db", ActionDigest: "d1", Decision: "approved", Approver: "token", ApprovedBy: "alice", TicketID: "INC-1"},
		{Tool: "drop_db", ActionDigest: "d2", Decision: "refused", Approver: "token", Reason: "no token"},
	}
	for _, e := range entries {
		if err := a.Record(e); err != nil {
			t.Fatal(err)
		}
	}

	// Re-verify the chain the way an auditor would.
	prev := hex.EncodeToString(make([]byte, sha256.Size))
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != len(entries) {
		t.Fatalf("wrote %d lines, want %d", len(lines), len(entries))
	}
	for i, line := range lines {
		var e approval.Entry
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
		if e.Prev != prev {
			t.Fatalf("line %d chains to %s, want %s", i, e.Prev, prev)
		}
		mac := e.MAC
		e.MAC = ""
		body, _ := json.Marshal(e)
		h := hmac.New(sha256.New, hmacKey)
		h.Write([]byte(e.Prev))
		h.Write(body)
		if want := hex.EncodeToString(h.Sum(nil)); mac != want {
			t.Fatalf("line %d MAC %s, want %s", i, mac, want)
		}
		if e.ApprovedBy != entries[i].ApprovedBy || e.Decision != entries[i].Decision {
			t.Fatalf("line %d lost fields: %+v", i, e)
		}
		prev = mac
	}

	// A nil audit records nothing and does not crash the call path.
	var nilAudit *approval.Audit
	if err := nilAudit.Record(approval.Entry{}); err != nil {
		t.Fatalf("nil audit errored: %v", err)
	}
}

// TestAudit_ResumesAcrossRestarts is the regression test for the defect this
// package shipped with: a process restart built a fresh Audit, which began at
// genesis and appended a second chain into the same file. The trail then held
// two chains an auditor could not link, so continuity — the whole point of
// chaining — was only ever guaranteed within one process lifetime.
func TestAudit_ResumesAcrossRestarts(t *testing.T) {
	var trail bytes.Buffer

	// First run of the process.
	a := approval.NewAudit(&trail, hmacKey)
	record(t, a, "envoyer_mise_en_demeure", "refused")
	record(t, a, "envoyer_mise_en_demeure", "refused")

	// Restart: a new Audit over the trail already on disk.
	prev, err := approval.LastMAC(bytes.NewReader(trail.Bytes()))
	if err != nil {
		t.Fatalf("LastMAC: %v", err)
	}
	if prev == approval.Genesis() {
		t.Fatal("LastMAC returned genesis for a trail holding two entries")
	}
	b := approval.NewAudit(&trail, hmacKey)
	b.Resume(prev)
	record(t, b, "envoyer_mise_en_demeure", "approved")

	n, err := approval.Verify(bytes.NewReader(trail.Bytes()), hmacKey)
	if err != nil {
		t.Fatalf("a resumed trail must verify end to end, got: %v", err)
	}
	if n != 3 {
		t.Fatalf("verified %d entries, want 3", n)
	}
}

// TestAudit_UnresumedRestartIsDetected pins the failure mode down: without
// Resume the trail must not quietly pass verification.
func TestAudit_UnresumedRestartIsDetected(t *testing.T) {
	var trail bytes.Buffer
	a := approval.NewAudit(&trail, hmacKey)
	record(t, a, "drop_db", "refused")

	b := approval.NewAudit(&trail, hmacKey) // no Resume: restarts at genesis
	record(t, b, "drop_db", "approved")

	n, err := approval.Verify(bytes.NewReader(trail.Bytes()), hmacKey)
	if err == nil {
		t.Fatal("a trail with two concatenated chains must not verify")
	}
	if n != 1 {
		t.Fatalf("verification stopped after %d entries, want 1", n)
	}
	if !strings.Contains(err.Error(), "breaks the chain") {
		t.Fatalf("error should name the broken link, got: %v", err)
	}
}

func TestAudit_VerifyDetectsEditedEntry(t *testing.T) {
	var trail bytes.Buffer
	a := approval.NewAudit(&trail, hmacKey)
	record(t, a, "envoyer_mise_en_demeure", "refused")
	record(t, a, "envoyer_mise_en_demeure", "approved")

	// Rewrite history: the refusal becomes an approval, MAC untouched.
	edited := strings.Replace(trail.String(), `"decision":"refused"`, `"decision":"approved"`, 1)
	if edited == trail.String() {
		t.Fatal("test setup: nothing was edited")
	}
	if _, err := approval.Verify(strings.NewReader(edited), hmacKey); err == nil {
		t.Fatal("an edited entry must not verify")
	} else if !strings.Contains(err.Error(), "altered") {
		t.Fatalf("error should say the entry was altered, got: %v", err)
	}
}

func TestAudit_LastMAC_EmptyAndCorrupt(t *testing.T) {
	got, err := approval.LastMAC(strings.NewReader(""))
	if err != nil {
		t.Fatalf("an empty trail is a valid new trail: %v", err)
	}
	if got != approval.Genesis() {
		t.Fatalf("empty trail gave %q, want genesis", got)
	}

	if _, err := approval.LastMAC(strings.NewReader("{not json\n")); err == nil {
		t.Fatal("an undecodable tail must be an error, not a silent fresh chain")
	}
}

func record(t *testing.T, a *approval.Audit, tool, decision string) {
	t.Helper()
	if err := a.Record(approval.Entry{
		Tool:         tool,
		ActionDigest: "3b5b94b58b23df86",
		Decision:     decision,
		Approver:     "token",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
}

// TestToken_MandateIsCarriedAndRecorded covers the half of attribution the
// action digest cannot reach. The digest binds an approval to one call, so a
// token minted for dossier-42 is already inert against dossier-77 — scope is
// solved. What it cannot say is in whose name the agent acted at all, which
// is what principal adds.
func TestToken_MandateIsCarriedAndRecorded(t *testing.T) {
	args := map[string]any{"dossier": "dossier-42", "destinataire": "SOCIETE ADVERSE SA"}
	digest := approval.Digest("envoyer_mise_en_demeure", args)

	tok, err := approval.MintGrant(hmacKey, approval.Grant{
		ApprovedBy:   "me.dupont@cabinet.fr",
		ActionDigest: digest,
		TicketID:     "DOSSIER-42",
		Principal:    "me.dupont@cabinet.fr",
		Actor:        "spiffe://cabinet.lab/agent-contentieux",
		TTL:          5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("MintGrant: %v", err)
	}

	verifier := &approval.Token{Key: hmacKey}
	dec, err := verifier.Approve(context.Background(), approval.Request{
		Tool: "envoyer_mise_en_demeure", Arguments: args, Digest: digest, Token: tok,
		Actor: "spiffe://cabinet.lab/agent-contentieux",
	})
	if err != nil {
		t.Fatalf("a token matching call, mandate and delegate must approve: %v", err)
	}
	if dec.Principal != "me.dupont@cabinet.fr" {
		t.Errorf("principal = %q, want the mandator", dec.Principal)
	}
	if dec.Actor != "spiffe://cabinet.lab/agent-contentieux" {
		t.Errorf("actor = %q, want the delegate", dec.Actor)
	}
}

func TestToken_MandateRefusals(t *testing.T) {
	args := map[string]any{"dossier": "dossier-42"}
	digest := approval.Digest("envoyer_mise_en_demeure", args)

	mint := func(principal, actor string) string {
		t.Helper()
		tok, err := approval.MintGrant(hmacKey, approval.Grant{
			ApprovedBy: "me.dupont@cabinet.fr", ActionDigest: digest,
			Principal: principal, Actor: actor, TTL: 5 * time.Minute,
		})
		if err != nil {
			t.Fatalf("MintGrant: %v", err)
		}
		return tok
	}

	t.Run("no principal where the deployment requires one", func(t *testing.T) {
		v := &approval.Token{Key: hmacKey, RequireMandate: true}
		_, err := v.Approve(context.Background(), approval.Request{
			Tool: "envoyer_mise_en_demeure", Arguments: args, Digest: digest,
			Token: mint("", ""),
		})
		if err == nil {
			t.Fatal("a token with no principal must be refused when a mandate is required")
		}
		if !strings.Contains(err.Error(), "names no principal") {
			t.Fatalf("error should name the missing mandate, got: %v", err)
		}
	})

	t.Run("delegate is not the proven caller", func(t *testing.T) {
		v := &approval.Token{Key: hmacKey}
		_, err := v.Approve(context.Background(), approval.Request{
			Tool: "envoyer_mise_en_demeure", Arguments: args, Digest: digest,
			Token: mint("me.dupont@cabinet.fr", "spiffe://cabinet.lab/agent-contentieux"),
			Actor: "spiffe://cabinet.lab/agent-recherche",
		})
		if err == nil {
			t.Fatal("a token delegating to another agent must be refused")
		}
		if !strings.Contains(err.Error(), "delegates to") {
			t.Fatalf("error should name both identities, got: %v", err)
		}
	})

	t.Run("no proven caller: recorded, not enforced", func(t *testing.T) {
		// The stdio case. There is no handshake, so Request.Actor is empty and
		// act.sub cannot be checked against anything. It must still approve —
		// refusing here would make the token unusable on desktop — and the
		// delegate must still reach the audit trail.
		v := &approval.Token{Key: hmacKey}
		dec, err := v.Approve(context.Background(), approval.Request{
			Tool: "envoyer_mise_en_demeure", Arguments: args, Digest: digest,
			Token: mint("me.dupont@cabinet.fr", "spiffe://cabinet.lab/agent-contentieux"),
			Actor: "",
		})
		if err != nil {
			t.Fatalf("an unproven caller must not break the token: %v", err)
		}
		if dec.Actor != "spiffe://cabinet.lab/agent-contentieux" {
			t.Errorf("the declared delegate must still be recorded, got %q", dec.Actor)
		}
	})
}
