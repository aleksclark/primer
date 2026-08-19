// Package agentruntime is the production Primer MAF adapter.
//
// # Status
//
// This package uses a pinned public-preview SDK:
//
//	github.com/microsoft/agent-framework-go v0.0.0-20260813082112-00ffc8c3648c
//	upstream commit: 00ffc8c3648c547997eae3a3f2a3b00c28daea09
//
// The spike evidence for feasibility (F1–F8) is documented in
// spikes/maf-go/RUN_REPORT.md. Treat all MAF APIs as preview-unstable;
// pin the commit and expect churn on upgrades.
//
// # Scope
//
// The package is non-durable and in-process. There is no distributed control
// plane, restart recovery, or database persistence in this layer. Process-local
// session continuity only.
//
// # Module ownership
//
// This package (github.com/aleksclark/primer/agents/runtime) is the single
// canonical source for the Primer MAF engine. The LMS compatibility seam at
// server/internal/agent is a temporary re-export shim; once go.work includes
// ./primer-agents the shim imports from here and no second independently-edited
// copy exists. See agent_docs/plans/primer-agents-service/phase-01-*.md.
//
// # Importing rules
//
// Never import github.com/microsoft/agent-framework-go/internal/…
// Only public MAF packages are permitted. A compile-time test enforces this.
package agentruntime
