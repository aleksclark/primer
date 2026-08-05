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

Token is stored in `localStorage` key `primer-parent-token`.

## Codegen

After server API changes:

```bash
make openapi && make client
```
