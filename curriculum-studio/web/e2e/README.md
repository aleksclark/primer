# S17 credential-free browser acceptance

From `curriculum-studio/web`:

```sh
(cd .. && GOWORK=off make clients-generate)
npm ci --ignore-scripts --no-audit --no-fund
npx playwright install chromium
npm run test:e2e:s17
```

The npm `pretest:e2e:s17` hook runs the existing `npm run build` before each
normal test invocation, so the fixture serves a fresh SPA. Generated clients and
the Playwright Chromium installation remain prerequisites as shown above.

Docker must be available. The runner starts the existing opt-in
`../scripts/browser-fixture.sh`, unsets external `STUDIO_TEST_DATABASE_URL`, and
uses its printed ephemeral loopback URL. Every run gets a new PostgreSQL
container and separate Ada/Ruth/owner/W2/W3 browser contexts. The application is
the built real SPA + Huma/chi + JWT validator + local memberships. Only the
fixture issuer/session edge is test wiring; no production identity/provider or
CSC-00 acceptance is claimed.

The full Chromium browser starts **after** Docker fixture readiness (avoids
route-change interruptions during startup). Full headless Chromium is deliberate:
headless-shell does not reproduce Chrome's implicit favicon request. No retries,
network mocking, auth shortcuts, blanket negative-status allowlists, or traces
containing cookies are enabled. Every intended negative request is named and
matched by exact method/path/status; all other HTTP/console failures fail.

One scenario has six named steps: durable comments/diff; template/library;
mapped stale/current review and policy publication; approved-invalid refusal and
valid shared-draft mutation denial; share/private-action isolation and revocation;
mobile native tap, focus, and themes. Reads of the authenticated API supplement
visible assertions, including published state and unchanged graph fingerprint.

Failures close all owned contexts and signal only the verified fixture host;
the Go host cleans its container and the launcher removes its directory. Startup
failure signals only the detached owned process group. Readiness and shutdown
are bounded; cleanup failures surface. Runner logs print safe PID/URL/directory
only, never the 0600 database URL. Automated screenshots, attachments, and runner
state live in `curriculum-studio/web/test-results/s17-collaboration/`, covered by
the existing web-local `/test-results/` ignore rule. Check a generated file with
`git check-ignore -v <path>` from the repository root. No automated runner output
is written to root `.paseo-e2e/`; historical manual agent evidence there is
separate and is preserved.

## Promotion handoff

Verified against product `5fb2d97e99067b5e6b6dc5c9dd715cad8a408bb9`:
full-Chromium suite **1 passed**, followed by a fresh independent Chrome MCP
multi-principal re-drive of all six groups. Native CDP touch supplemented MCP's
mouse-only click tool; it did not replace the MCP journey or reuse the Playwright
test. Desktop/mobile dark/light evidence and safe response records remain local
under `.paseo-e2e/s17-collaboration/call7-chrome/`.

The earlier full-Chrome favicon404 regression remains protected by the
unexpected-console audit. The source-owned HTML5 chooser/icon fix now returns200
without exemptions. Fixture cleanup verification allows for asynchronous Ryuk
container removal after the Go host exits. No live identity, provider generation,
coverage threshold, whole-phase approval, or production readiness is claimed.

L1 should review/integrate this test-only commit, then ask this same leaf to run
`npm run test:e2e:s17` on the final integrated candidate. Historical failed logs,
including the favicon and startup font-network failures, remain in local evidence.
