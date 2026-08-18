# clients/ts-rest

Generated TypeScript authoring REST types plus a committed `openapi-fetch`
façade. The emitted Huma OpenAPI document is the only generation input;
`generated/` is build output and remains gitignored.

```bash
cd curriculum-studio/clients/ts-rest
npm install
npm run generate
npm run build                 # strict tsc
```

The façade exports `createClient({ baseUrl, token })`. It attaches a signed
Primer Bearer JWT when provided. Studio UI consumers must import this package;
they must not construct raw `fetch` calls to `/studio/v1`.
