# Primer Tasks Phase 4 evidence

## Scope

Phase 4 is the web/server student-dialogue verification phase.

## Reviewed result

- Reviewed branch: `impl/tasks-p4-dialogue`
- Reviewed SHA: `851cc537e79963160ec7787f005245b8ee269791`
- Base plan revision: `24cd3abe`
- Working tree: clean
- Evidence directory: `.paseo-e2e/phase4-student-dialogue/`

Fresh Terra browser exploration and promotion verified:

- Parent authoring, publish, schedule, browser pairing, and student dialogue completion with three distinct accepted answers.
- Insufficient answer and prompt-injection/answer-key attempts produce bounded retry behavior without increasing accepted count.
- Reconnect resumes the durable attempt at 2/3 with only the remaining question.
- Cross-student/cross-occurrence WebSocket subscription is denied with a generic `not_found` response.
- Two browser tabs submitting the same turn produce a typed conflict; the durable turn evaluates once and the recovered tab continues.
- Malformed and provider-timeout runs retain the durable answer, expose a generic retryable failure, and never invent an evaluation or completion. Explicit retry reuses the existing durable answer/job.
- Parent Inspect displays student-authored text separately from questions/evaluations, safe rationale, server-selected provenance/policy, usage, and append-only override evidence. Immediate override reconciliation was observed without rewriting prior evidence.
- Fresh desktop/mobile light/dark responsive checks, Lighthouse accessibility checks, and console/network inspection passed for the final browser paths.
- Wire/DB/log/privacy and anti-cheat review found no raw reasoning transport or persistence, client-owned policy, broad student tools, model-direct completion, fake completion path, or cross-occurrence lookup.

## Promoted browser evidence

- Playwright spec: `primer-tasks/web/e2e/phase4-student-dialogue.spec.ts`
- Promotion command:
  `PRIMER_TASKS_BASE_URL=http://127.0.0.1:38638 npm --prefix web run browser:promoted -- e2e/phase4-student-dialogue.spec.ts`
- Result: **PASS, 2/2**
- Independent fresh Chrome redrive: **PASS**
- Sanitized records:
  - `.paseo-e2e/phase4-student-dialogue/exploration.md`
  - `.paseo-e2e/phase4-student-dialogue/last-run.log`
  - `.paseo-e2e/phase4-student-dialogue/privacy-scan-call18.txt`
  - `.paseo-e2e/phase4-student-dialogue/state.json`

## Web/server gates

- `make tasks-test` — PASS
- `make tasks-cover` — PASS at **85.0%** (mandatory minimum met)
- `make tasks-agent-compat` — PASS
- `make tasks-clients` — PASS
- `make tasks-web` — PASS
- TypeScript client tests — PASS, 6/6
- Web typecheck and lint — PASS; two pre-existing unrelated warnings remain
- `go test -race ./internal/verification/... ./internal/agent/... ./internal/api/... -count=10` — PASS
- `git diff --check` — PASS

## Release decision

The Phase 4 web/server completion gate is **PASS**. Phase 5 may dispatch.

No live-quality or educational-quality claim is made.
