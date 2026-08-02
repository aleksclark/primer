# Primer TV admin SPA

Admin interface for the TV server (`server/cmd/tv-server`): the curated media
library, the on-demand rotation, the programmed schedule, and paired devices.

Same stack and conventions as the LMS SPA in `../web` — Vite + React +
Tailwind, shadcn-style primitives in `src/components/ui`, and a typed client
generated from the server's OpenAPI spec. Shared patterns (`api/client.ts`,
`hooks/use-list.ts`, `components/resource-page.tsx`, `lib/columns.tsx`) are
deliberately mirrored; changes worth making usually belong in both.

## Commands

```bash
npm install
npm run dev              # dev server on :5174, proxying /api to :8081
npm run generate:client  # openapi.yaml -> src/api/schema.d.ts
npm run build            # typecheck + production build
npm run lint
```

From the repository root:

```bash
make tv-client   # regenerate openapi.yaml and the TS client
make tv-web      # build the SPA
make tv-bundle   # embed the built SPA into the tv-server binary
```

`openapi.yaml` is generated from the Go handler signatures
(`make openapi-tv`) and checked in for reproducible builds.

## Admin authentication

The TV admin API is guarded by a shared key sent as `X-Admin-Key`
(`TV_ADMIN_API_KEY` on the server; the server also accepts
`Authorization: Bearer`). The SPA prompts for the key, keeps it in
localStorage, attaches it to every request, and returns to the prompt with an
explicit unauthorized message if the server rejects it. A server started
without a key configured accepts requests without one, so local development can
skip the prompt.

## Pages

| Page | Purpose |
|------|---------|
| Library | Browse and import Jellyfin items, classify them (class, subject tags, standard codes, quality notes), see direct-play status, trigger a metadata sync |
| Availability | Rotation calendar over a fortnight plus a CRUD table; bulk expire and "expire & rotate" |
| Schedule | Plain CRUD over the programmed grid |
| Devices | Register devices, show and re-issue pairing codes, rename, revoke, delete |

A metrics dashboard is not included: the TV server exposes no `/metrics`
endpoints yet. It belongs with the reporting work in a later phase.
