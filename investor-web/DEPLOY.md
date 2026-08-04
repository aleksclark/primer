# Investor pitch site — deploy

Static Vite SPA under `investor-web/`. No server API is required for the public pitch.

## Environments

| Env | Purpose | Indexing |
| --- | --- | --- |
| Local dev | `npm run dev` | n/a (meta defaults noindex) |
| Preview / PR | Static upload of `dist/` | **noindex** (default) |
| Staging | Shared review URL | **noindex** unless overridden |
| Production | Fundraising URL | index only with explicit flags |

## Environment variables

Set at **build time** (Vite inlines `VITE_*`):

| Variable | Required | Description |
| --- | --- | --- |
| `VITE_SITE_ORIGIN` | prod recommended | Canonical origin, e.g. `https://invest.example.com` (no trailing slash) |
| `VITE_SITE_ENV` | prod | Set to `production` for the live fundraising host |
| `VITE_ALLOW_INDEX` | prod | Set to `1` only on the production host that should be crawled |
| `VITE_CONTACT_EMAIL` | recommended | Public founder inbox for mailto CTAs |
| `VITE_NOINDEX` | optional | Force `1`/`true` to noindex even if other flags allow |
| `VITE_ANALYTICS_DEBUG` | local only | Set `1` in dev to log analytics events to the console |

Do **not** bake third-party analytics API keys into the bundle. To connect a privacy-safe collector, assign `window.__PRIMER_ANALYTICS__` from the host page or a tiny loader that you control.

### Staging noindex behavior

- `index.html` ships with `<meta name="robots" content="noindex, nofollow">`.
- The SPA keeps noindex unless `VITE_ALLOW_INDEX=1` **and** `VITE_SITE_ENV=production` (see `src/lib/siteMeta.ts`).
- Build emits `dist/robots.txt` with `Disallow: /` unless those production flags are set.
- Preview deployments must never set both production flags.

### Production indexable build example

```bash
export VITE_SITE_ORIGIN=https://invest.example.com
export VITE_SITE_ENV=production
export VITE_ALLOW_INDEX=1
export VITE_CONTACT_EMAIL=you@example.com
make investor-web-ci
# upload investor-web/dist/ to the production static host
```

## Preview deploy (manual)

```bash
# from repo root
make design-system
cd investor-web
npm ci
npm run validate
npm test
npm run typecheck
npm run lint
npm run build
npm run check:bundle
# upload dist/ to any static host / object storage / Pages project
npx vite preview   # optional local smoke of the production build
```

Or the single CI-oriented target:

```bash
make investor-web-ci
```

## Production deploy checklist

1. Confirm content gate: `cd investor-web && npm run check:launch`
2. Build with production env vars (table above).
3. Upload `investor-web/dist/` immutably (versioned release or content-hashed assets).
4. Configure host headers from `public/_headers` (CSP, referrer-policy, permissions-policy, HSTS at the edge).
5. Verify:
   - `https://<host>/` loads the thesis
   - `https://<host>/robots.txt` allows crawl only in production
   - View-source / DOM shows `index, follow` only in production
   - Canonical URLs use `VITE_SITE_ORIGIN`
   - Mailto CTA uses the real inbox
   - Print on `/market` and `/evidence` hides chrome
6. Smoke routes: `/`, `/demo`, `/market`, `/evidence`, `/schools`, `/company`, `/diligence`
7. Optional: attach `window.__PRIMER_ANALYTICS__` without blocking render

## CI command sequence

Recommended order (also encoded in `make investor-web-ci`):

1. `make design-system` — regenerate tokens / `primer.css`
2. `cd investor-web && npm ci` (CI) or `npm install` (local)
3. Data + launch gates: `npm test` (includes validate-data, system-c, market math, launch, a11y)
4. `npm run typecheck`
5. `npm run lint`
6. `npm run build`
7. `npm run check:bundle` — performance budgets + required SEO files
8. Deploy `dist/` as a preview artifact (PR) or production release (main/tag)

## Rollback

- Keep the previous `dist/` artifact (or prior object-storage version / Pages deployment).
- Point the production hostname back to the last known-good artifact.
- Because assets are content-hashed, rollback is a full directory swap — do not mix asset generations.
- After rollback, re-run the production smoke checklist above.

## Security / privacy notes

- Static hosting only for the public pitch.
- No student or family data in demos (synthetic only).
- No source maps in production builds (`vite.config.ts` sets `sourcemap: false`).
- Contact is mailto (or a future spam-resistant form endpoint) — no inbox credentials in the client.
- Dependency review: run `npm audit` periodically (see launch runbook).

## Monitoring

- Uptime check on the production origin and the primary contact path (mailto cannot be probed; monitor the page that contains it).
- After analytics wiring, watch qualified events (CTA, demo complete, diligence visits) — not raw page views alone.
