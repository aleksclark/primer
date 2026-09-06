//go:build tasksclientcontract

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Dedicated generated-client gate, wired by the product clients-typescript
// target after real emission/generation. This is not the ordinary server-only
// test lane and never substitutes a mock API/model for public conformance.
func TestGeneratedStudentClientUsesPublicBoundary(t *testing.T) {
	h := newPublicDialogueHarness(t)
	u, _ := url.Parse(h.base)
	cookies := func(clientCookies bool) string {
		client := h.parent
		if clientCookies {
			client = h.student
		}
		parts := []string{}
		for _, cookie := range client.Jar.Cookies(u) {
			parts = append(parts, cookie.Name+"="+cookie.Value)
		}
		return strings.Join(parts, "; ")
	}
	input, err := json.Marshal(map[string]string{"baseUrl": h.base, "occurrenceId": h.occurrence, "studentCookies": cookies(true), "parentCookies": cookies(false), "csrf": h.csrf()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "../../clients/typescript/scripts/public-dialogue-probe.mjs")
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated client public conformance failed: %v; safe diagnostic %s", err, output)
	}
	var result struct {
		Completed      bool `json:"completed"`
		Accepted       int  `json:"accepted"`
		Evaluations    int  `json:"evaluations"`
		TypedError     bool `json:"typedError"`
		RESTSchemaLink bool `json:"restSchemaLink"`
	}
	if json.Unmarshal(output, &result) != nil || !result.Completed || result.Accepted != 3 || result.Evaluations != 4 || !result.TypedError || !result.RESTSchemaLink {
		t.Fatal("generated client did not prove the actual public outcome")
	}
	var decisions, completions int
	if err = h.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM verification_decisions d JOIN verification_attempts a ON a.tenant_id=d.tenant_id AND a.id=d.attempt_id WHERE a.occurrence_id=$1 AND d.accepted),(SELECT count(*) FROM verification_events e JOIN verification_attempts a ON a.tenant_id=e.tenant_id AND a.id=e.attempt_id WHERE a.occurrence_id=$1 AND e.kind='complete')`, h.occurrence).Scan(&decisions, &completions); err != nil || decisions != 1 || completions != 1 {
		t.Fatal("generated client outcome is not one durable decision/completion")
	}
}
