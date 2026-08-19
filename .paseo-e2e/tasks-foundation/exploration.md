# Tasks foundation — exploration record

**Status:** BLOCKED before UI exploration

- **Start URL:** `http://127.0.0.1:37456/`
- **API base (provided):** `http://127.0.0.1:37455`
- **Attempted:** 2026-08-19T01:40:52Z
- **Mode:** Chrome DevTools MCP only; no fixture, private-repository, mock-server, or API data seeding was performed.

## Browser actions and result

1. Discovered the Chrome DevTools MCP tool catalog and inspected `chrome_devtools_take_snapshot`.
2. Tried `chrome_devtools_new_page` at the start URL in a named isolated context (`tasks-foundation-parent-a`).
3. The MCP server rejected the request before navigation: `The browser is already running for /home/aleks/.cache/chrome-devtools-mcp/paseo-e2e-profile. Use --isolated to run multiple browser instances.` The exposed `new_page` interface has `isolatedContext`, but that did not resolve the underlying shared profile lock.
4. A follow-up `chrome_devtools_list_pages` failed with the identical browser-profile error. Therefore no existing page could safely be selected and no browser UI was driven.

## Environment observation (not acceptance evidence)

A non-mutating connectivity check found the web origin reachable (`HTTP 200` at `/`). The API origin was reachable but its root endpoint returned `404 page not found`, which is normal for an API root and does not demonstrate any requested behavior.

## Acceptance evidence

No requested acceptance criterion is PASS or FAIL: all are **BLOCKED** because Chrome DevTools MCP cannot launch or access its required isolated browser profile.

| Criterion | Result | Evidence |
|---|---|---|
| Parent A login; create student; refresh persistence | BLOCKED | No browser page available |
| Issue QR/code; student pairs; empty checklist | BLOCKED | No browser page available |
| Tenant B denial | BLOCKED | No browser page available |
| Pairing replay denial | BLOCKED | No browser page available |
| Cookie security | BLOCKED | No browser page/network/cookie inspector available |
| Dark desktop/mobile/light screenshots | BLOCKED | No browser page available |
| Console/network notes | BLOCKED | Console/network tools reject due to same profile error |

## Retry — call #2

**Attempted:** 2026-08-19T01:45:00Z

Per the caller's instruction, I retried the existing Chrome DevTools session first with `chrome_devtools_list_pages`; exact error:

```text
The browser is already running for /home/aleks/.cache/chrome-devtools-mcp/paseo-e2e-profile. Use --isolated to run multiple browser instances.
Cause: The browser is already running for /home/aleks/.cache/chrome-devtools-mcp/paseo-e2e-profile. Use a different `userDataDir` or stop the running browser first.
```

I then retried by opening `http://127.0.0.1:37456/` with `chrome_devtools_new_page` and the explicit isolated context `tasks-foundation-explore-2`. It failed with the exact same error before page creation. The MCP tool accepts `isolatedContext` but cannot reach the running browser/profile to create that context.

No UI flow was driven and all acceptance results remain BLOCKED. No private repo, mock, fixture, API seed, or Playwright file was used.

## Intended public-UI procedure when unblocked

1. Launch a fresh isolated Chrome DevTools browser/profile, navigate to the start URL, and snapshot the real UI.
2. Use only snapshot-derived controls to log in as dev parent A, create a uniquely named student, and reload to confirm persistence.
3. Issue the pairing QR/code in the parent UI. Use a separate isolated browser context as the student browser, complete pairing through the public UI, and confirm the checklist contains no tasks.
4. Use a third isolated context for tenant B. Where the product offers public navigation to an A-owned resource, verify a user-visible denial/non-disclosure without API mutation or hidden endpoint guessing.
5. Re-enter the consumed pairing credential in a fresh student context and record the public replay-denial message.
6. Capture browser-visible cookie/security evidence and DevTools network response headers; record only attribute values, never cookie secrets.
7. Capture screenshots at desktop dark, mobile dark, and desktop light after the relevant UI has rendered; inspect console and failed requests after each significant flow.

## Flake risks / blocker

The shared Chrome DevTools profile at `/home/aleks/.cache/chrome-devtools-mcp/paseo-e2e-profile` is already owned by another process/session. This agent did not terminate it because that could destroy another agent's isolated test state. Release that profile or configure the MCP runner with a distinct user-data directory/actual `--isolated` launch option, then rerun exploration.
