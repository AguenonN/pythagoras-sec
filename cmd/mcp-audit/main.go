// Command mcp-audit verifies an approval trail and prints the chain of
// responsibility it records. It is the auditor's half of the mechanism:
// mcp-approve mints the authority, mcp-proxy enforces and records it, and
// this walks the record back.
//
//	mcp-audit -file audit.jsonl -key-file audit-hmac.key
//
// Verification checks two things per entry: that its content still seals to
// the MAC it carries, and that its prev equals the previous entry's MAC. A
// trail that passes shows nothing was edited, dropped or reordered since it
// was written — by anyone without the key.
//
// Without -key-file the chain is checked as plain SHA-256: enough to catch
// truncation and accidental edits, not enough to stand against someone who
// rewrote the file. The exit status says which guarantee you got.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"mcprogue/internal/approval"
)

func main() {
	file := flag.String("file", "", "approval trail to verify (AUDIT_LOG_FILE)")
	keyFile := flag.String("key-file", "", "the HMAC key the trail was sealed with (AUDIT_HMAC_KEY_FILE)")
	quiet := flag.Bool("quiet", false, "verify only; do not print the entries")
	flag.Parse()

	if *file == "" {
		flag.Usage()
		log.Fatal("mcp-audit: -file is required")
	}

	var key []byte
	if *keyFile != "" {
		b, err := os.ReadFile(*keyFile)
		if err != nil {
			log.Fatalf("mcp-audit: read key: %v", err)
		}
		key = []byte(strings.TrimSpace(string(b)))
	}

	f, err := os.Open(*file)
	if err != nil {
		log.Fatalf("mcp-audit: %v", err)
	}
	defer f.Close()

	n, err := approval.Verify(f, key)
	if err != nil {
		fmt.Printf("CHAIN BROKEN after %d valid entries\n  %v\n", n, err)
		os.Exit(1)
	}

	if !*quiet {
		if _, err := f.Seek(0, 0); err != nil {
			log.Fatalf("mcp-audit: rewind: %v", err)
		}
		if err := print(f); err != nil {
			log.Fatalf("mcp-audit: %v", err)
		}
	}
	guarantee := "integrity only (no key given: tamper-evident, not tamper-proof)"
	if len(key) > 0 {
		guarantee = "authenticated with the trail's key"
	}
	fmt.Printf("\nCHAIN INTACT: %d entries, %s\n", n, guarantee)
}
