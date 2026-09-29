package approval

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"sync"
	"time"
)

// Entry is one approval decision in the audit trail. Every field except the
// chain fields is set by the caller; Time, Prev and MAC are filled by Record.
type Entry struct {
	Time         time.Time `json:"ts"`
	Tool         string    `json:"tool"`
	ActionDigest string    `json:"action_digest"`
	Decision     string    `json:"decision"` // "approved" | "refused"
	Approver     string    `json:"approver"`
	ApprovedBy   string    `json:"approved_by,omitempty"`
	TicketID     string    `json:"ticket_id,omitempty"`
	// Principal is on whose behalf the act was done (the mandate) and Actor
	// the agent that performed it. With ApprovedBy they are the chain an
	// auditor reconstructs: who mandated, who acted, who approved. Empty
	// when the approval mechanism carries no mandate.
	Principal string `json:"principal,omitempty"`
	Actor     string `json:"actor,omitempty"`
	Reason    string `json:"reason,omitempty"`

	// Prev is the previous entry's MAC (a run of zeros for the first entry),
	// and MAC authenticates this entry plus Prev. Together they chain the
	// file: an entry cannot be dropped, reordered or edited without breaking
	// every MAC after it. With a key, MAC is HMAC-SHA256 and forging it
	// requires the key; without one it degrades to plain SHA-256 — tamper
	// EVIDENT against accidents, not against an attacker who can rewrite the
	// whole file.
	Prev string `json:"prev"`
	MAC  string `json:"mac"`
}

// Audit writes the hash-chained trail, one JSON object per line. A nil *Audit
// is valid and records nothing, so wiring is unconditional at call sites.
type Audit struct {
	mu   sync.Mutex
	w    io.Writer
	key  []byte
	prev string
}

// NewAudit chains entries into w. key is the optional HMAC key; nil degrades
// the chain to integrity-only.
//
// A fresh Audit starts at genesis, which is correct for a new trail and wrong
// for an existing one: appending a second genesis into the same file splits it
// into two chains an auditor cannot link. When w is an existing trail, read
// its tail with LastMAC and hand the result to Resume.
func NewAudit(w io.Writer, key []byte) *Audit {
	return &Audit{w: w, key: key, prev: genesis}
}

// Resume points the chain at prev — the MAC of the last entry already in the
// trail — so appended entries continue it instead of restarting. Call it
// before the first Record.
func (a *Audit) Resume(prev string) {
	if a == nil || prev == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prev = prev
}

var genesis = hex.EncodeToString(make([]byte, sha256.Size))

// Record appends one entry. The write happens under the lock so the chain
// order on disk is the chain order in memory.
func (a *Audit) Record(e Entry) error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	e.Time = time.Now().UTC()
	e.Prev = a.prev
	e.MAC = ""
	body, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("audit: encode entry: %w", err)
	}
	e.MAC = a.seal(body)
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("audit: encode entry: %w", err)
	}
	if _, err := a.w.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("audit: write entry: %w", err)
	}
	a.prev = e.MAC
	return nil
}

// seal computes the MAC over prev || entry-without-mac.
func (a *Audit) seal(body []byte) string {
	var h hash.Hash
	if len(a.key) > 0 {
		h = hmac.New(sha256.New, a.key)
	} else {
		h = sha256.New()
	}
	h.Write([]byte(a.prev))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// Genesis is the Prev value of a trail's first entry: a run of zeros standing
// for "nothing precedes this".
func Genesis() string { return genesis }

// LastMAC reads an existing trail and returns the MAC its next entry must
// chain onto. An empty or absent trail returns Genesis.
//
// A line that does not decode is an error rather than a fresh start: a
// truncated tail (a crash mid-write) means the chain has a hole, and silently
// beginning a new one at genesis is exactly the defect Resume exists to fix.
// The caller decides whether that is fatal; for an audit trail it should be.
func LastMAC(r io.Reader) (string, error) {
	last := genesis
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxEntryBytes)
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return "", fmt.Errorf("audit: trail line %d does not decode: %w", n, err)
		}
		if e.MAC == "" {
			return "", fmt.Errorf("audit: trail line %d carries no mac", n)
		}
		last = e.MAC
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("audit: read trail: %w", err)
	}
	return last, nil
}

// Verify walks a trail and checks every link: each entry's MAC must
// authenticate its own content, and its Prev must equal the previous entry's
// MAC. It returns the number of entries verified.
//
// key must be the one the trail was sealed with. Without it the check still
// detects accidental corruption, but not an attacker who rewrote the file and
// recomputed the plain hashes — which is the difference between a
// tamper-evident log and an opposable record.
func Verify(r io.Reader, key []byte) (int, error) {
	a := &Audit{key: key, prev: genesis}
	n := 0
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxEntryBytes)
	for line := 1; sc.Scan(); line++ {
		raw := bytes.TrimSpace(sc.Bytes())
		if len(raw) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(raw, &e); err != nil {
			return n, fmt.Errorf("audit: entry %d does not decode: %w", line, err)
		}
		if e.Prev != a.prev {
			return n, fmt.Errorf("audit: entry %d breaks the chain: it follows %s, but the previous entry sealed to %s",
				line, short(e.Prev), short(a.prev))
		}
		want := e.MAC
		e.MAC = ""
		body, err := json.Marshal(e)
		if err != nil {
			return n, fmt.Errorf("audit: entry %d does not re-encode: %w", line, err)
		}
		if got := a.seal(body); got != want {
			return n, fmt.Errorf("audit: entry %d has been altered: it seals to %s, not the %s it carries",
				line, short(got), short(want))
		}
		a.prev = want
		n++
	}
	if err := sc.Err(); err != nil {
		return n, fmt.Errorf("audit: read trail: %w", err)
	}
	return n, nil
}

// maxEntryBytes caps one trail line. A refusal reason quotes the caller, so
// entries are not bounded by anything the audit package controls.
const maxEntryBytes = 1 << 20

func short(mac string) string {
	if len(mac) > 12 {
		return mac[:12]
	}
	return mac
}
