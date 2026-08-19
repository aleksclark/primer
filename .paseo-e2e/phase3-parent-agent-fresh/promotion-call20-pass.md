# Promotion call #20 — PASS

Focused real-stack command (full output in `last-run.log`) exited 0:

```sh
PRIMER_TASKS_BASE_URL=http://127.0.0.1:38237 \
PRIMER_TASKS_EXPLORATORY_BROWSER_PASS=1 \
PRIMER_TASKS_BROWSER_COMMAND='npx playwright test e2e/phase3-parent-agent.spec.ts' \
npm --prefix primer-tasks/web run browser:test
```

Result: `1 passed (1.8m)`.

The promoted spec uses observed roles/labels and actual Compose controls only. Its readiness gate now polls the real BFF-proxied `/api/auth/login?return_to=%2Fparent%2Fstudents` response for 200/302/303 rather than treating Vite's `/` response as API readiness. It covers remount/replay, API restart recovery, restricted tool failure/no effects, A/B isolation and prompt injection, public Origin/CSRF negatives, and responsive dark/light menu behavior. Slow-subscriber and axe remain retained, sanitized exploratory evidence rather than fake browser mocks.

Independent Chrome DevTools re-drive on fresh real `http://127.0.0.1:38250` authenticated Parent agent completed `List my tasks.` with allowlisted progress, cursor 11, and completed terminal (`165`–`166` snapshots plus MCP terminal observation).
