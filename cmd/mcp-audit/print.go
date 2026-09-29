package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"mcprogue/internal/approval"
)

// print renders the trail as the chain of responsibility: who mandated the
// act, which delegate performed it, who approved, and under which ticket.
// Fields the mechanism did not carry are shown as "—" rather than omitted, so
// a missing mandate is visible instead of invisible.
func print(r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e approval.Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return fmt.Errorf("decode entry: %w", err)
		}
		fmt.Printf("%s  %-8s  %s\n", e.Time.Format("15:04:05"), strings.ToUpper(e.Decision), e.Tool)
		fmt.Printf("    pour     %s\n", or(e.Principal))
		fmt.Printf("    par      %s\n", or(e.Actor))
		fmt.Printf("    approuve %s   ticket %s\n", or(e.ApprovedBy), or(e.TicketID))
		fmt.Printf("    acte     %s   maillon %s <- %s\n",
			trunc(e.ActionDigest), trunc(e.MAC), trunc(e.Prev))
		if e.Reason != "" {
			fmt.Printf("    motif    %s\n", e.Reason)
		}
	}
	return sc.Err()
}

func or(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func trunc(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
