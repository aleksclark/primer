// Package conformance holds the C12 MCP protocol and tool-schema conformance
// harness for Curriculum Studio. It is independent of the C11 REST/gRPC
// conformance package at internal/conformance and maintains its own
// REQ-MCP-* registry and mcp-coverage.json evidence artifact.
//
// Pin evidence:
//
//	Protocol revision: 2026-07-28
//	SDK: github.com/modelcontextprotocol/go-sdk v1.7.0
package conformance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

// ProtocolVersion is the pinned MCP spec revision under test.
const ProtocolVersion = "2026-07-28"

// SDKPin records the exact Go SDK module requirement for this package.
const SDKPin = "github.com/modelcontextprotocol/go-sdk v1.7.0"

// reqMCPRegistry is the frozen C12 requirement list in sorted order.
var reqMCPRegistry = []string{
	"REQ-MCP-1", // SoT non-overlap documented and gated (P12-S1)
	"REQ-MCP-2", // Tool inventory freeze (P12-S2)
	"REQ-MCP-3", // Official SDK client conformance tour (P12-S3)
	"REQ-MCP-4", // External Streamable HTTP client (P12-S4)
	"REQ-MCP-5", // Protocol and header negatives (P12-S5)
	"REQ-MCP-6", // Authz and tenancy negatives (P12-S6)
	"REQ-MCP-7", // Idempotency/concurrency and publish confirmation (P12-S7)
	"REQ-MCP-8", // Disconnect / cancel (P12-S8)
	"REQ-MCP-9", // Coverage matrix complete (P12-S9)
}

// c12Mappings maps each REQ-MCP-* to the E12-* test IDs that cover it.
var c12Mappings = map[string][]string{
	"REQ-MCP-1": {"E12-01"},
	"REQ-MCP-2": {"E12-02"},
	"REQ-MCP-3": {"E12-03"},
	"REQ-MCP-4": {"E12-04"},
	"REQ-MCP-5": {"E12-05"},
	"REQ-MCP-6": {"E12-06"},
	"REQ-MCP-7": {"E12-07", "E12-08"},
	"REQ-MCP-8": {"E12-09"},
	"REQ-MCP-9": {"E12-10"},
}

// blockedTests lists E2E IDs that are BLOCKED with a named external or
// upstream platform dependency. These are not treated as failures in the
// overall gate, but remain visible in the evidence artifact.
var blockedTests = map[string]string{
	"E12-04": "external MCP client (mcporter/Hermes; https://github.com/mcporter/mcporter) not found in PATH — install or set STUDIO_MCP_EXTERNAL_CLIENT",
	"E12-07": "Studio draft mutation services (idempotency/concurrency) are not available on the S19 transport-only master tip",
	"E12-08": "Studio publish/MRTR persistence services are not available on the S19 transport-only master tip",
}

// C12RequirementProof records evidence for one REQ-MCP-*.
type C12RequirementProof struct {
	E2EIDs        []string `json:"e2e_ids"`
	Passed        bool     `json:"passed"`
	Blocked       bool     `json:"blocked,omitempty"`
	BlockedReason string   `json:"blocked_reason,omitempty"`
}

// C12Coverage is the stable JSON evidence emitted by this conformance package.
type C12Coverage struct {
	SchemaVersion   int                            `json:"schema_version"`
	ProtocolVersion string                         `json:"protocol_version"`
	SDKPin          string                         `json:"sdk_pin"`
	Requirements    map[string]C12RequirementProof `json:"requirements"`
	E12Tests        []string                       `json:"e12_tests"`
	// Passed is true when every non-blocked REQ-MCP-* has at least one
	// passing E2E ID. Blocked requirements are excluded from this gate.
	Passed bool `json:"passed"`
}

// DefaultEvidencePath returns the canonical output path for C12 coverage JSON,
// navigating from this source file to the shared evidence directory.
func DefaultEvidencePath() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("conformance: runtime.Caller failed in DefaultEvidencePath")
	}
	return filepath.Clean(filepath.Join(
		filepath.Dir(thisFile),
		"..", "..", "..",
		"tools", "contract-gates", "evidence", "mcp-coverage.json",
	))
}

// ValidateAndEmit validates REQ-MCP-* coverage and writes deterministic JSON
// evidence. passedIDs are E12-* tests that ran and passed; skippedIDs are
// tests that were skipped (checked against blockedTests and treated as
// blocked rather than failed). An error is returned only for genuine
// un-blocked failures or mapping gaps.
func ValidateAndEmit(path string, passedIDs, skippedIDs []string) error {
	passed := make(map[string]bool, len(passedIDs))
	for _, id := range passedIDs {
		passed[id] = true
	}
	skipped := make(map[string]bool, len(skippedIDs))
	for _, id := range skippedIDs {
		skipped[id] = true
	}

	reqs := make(map[string]C12RequirementProof, len(reqMCPRegistry))
	allPassed := true
	e12Set := make(map[string]bool)

	for _, req := range reqMCPRegistry {
		ids, ok := c12Mappings[req]
		if !ok || len(ids) == 0 {
			return fmt.Errorf("conformance: %s has no E12 mapping", req)
		}
		sorted := append([]string(nil), ids...)
		sort.Strings(sorted)

		proof := C12RequirementProof{E2EIDs: sorted}
		reqPassed := true

		for _, id := range sorted {
			e12Set[id] = true
			if passed[id] {
				// passed — good.
			} else if reason, isBlocked := blockedTests[id]; isBlocked || skipped[id] {
				// Explicit BLOCKED with named reason.
				reqPassed = false
				proof.Blocked = true
				if reason == "" {
					reason = "skipped"
				}
				if proof.BlockedReason == "" {
					proof.BlockedReason = reason
				} else if proof.BlockedReason != reason {
					proof.BlockedReason += "; " + reason
				}
			} else {
				// Genuine un-blocked failure.
				reqPassed = false
				allPassed = false
			}
		}
		proof.Passed = reqPassed
		reqs[req] = proof
	}

	for id := range passed {
		if !e12Set[id] {
			return fmt.Errorf("conformance: orphan passed E12 id %s", id)
		}
	}
	for id := range skipped {
		if !e12Set[id] {
			return fmt.Errorf("conformance: orphan skipped E12 id %s", id)
		}
	}
	for id := range passed {
		if skipped[id] {
			return fmt.Errorf("conformance: E12 id %s is both passed and skipped", id)
		}
	}

	e12Tests := make([]string, 0, len(e12Set))
	for id := range e12Set {
		e12Tests = append(e12Tests, id)
	}
	sort.Strings(e12Tests)

	cov := C12Coverage{
		SchemaVersion:   1,
		ProtocolVersion: ProtocolVersion,
		SDKPin:          SDKPin,
		Requirements:    reqs,
		E12Tests:        e12Tests,
		Passed:          allPassed,
	}

	data, err := json.MarshalIndent(cov, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
