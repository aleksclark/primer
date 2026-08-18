// Package agent is the production Primer MAF adapter.
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
// # Importing rules
//
// Never import github.com/microsoft/agent-framework-go/internal/…
// Only public MAF packages are permitted.
package agent
