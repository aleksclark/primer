# Primer LMS admin SPA

Vite + React admin UI for the Primer LMS API.

## Dev

```bash
# from repo root
make openapi client
cd web && npm install && npm run dev
```

Proxy `/api/v1` to a running `primer-server` (see `vite.config.ts`).

## Parent session (Student client section)

Routes under **Student client** (Devices, Assignments, Sessions, Overview) call
parent-guarded APIs and need a Bearer session token:

1. Sign in on the gate form (email/password → `POST /auth/login`), or
2. Paste a token, or
3. Set `VITE_PARENT_TOKEN` before `npm run dev`.

Token is stored in `localStorage` key `primer-parent-token`. The parent API also
accepts a configured Primer Identity JWT and resolves it to a local educator
with `parent`/`admin` role. This legacy browser Bearer storage is not a completed
BFF/Clerk migration; see the [current authentication boundaries](../agent_docs/authentication.md).
Do not place production tokens in frontend build-time variables or commit them.

## Codegen

After server API changes:

```bash
make openapi && make client
```
