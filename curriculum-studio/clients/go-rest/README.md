# clients/go-rest

Generated Go authoring REST client package. The emitted Huma OpenAPI document
is normalized only for ogen's current 3.0 parser (`type: [T, null]` becomes
`type: T, nullable: true`) in an ignored build-only file; the source contract
remains the emitted 3.1 document.

Generator: `ogen v1.15.0`. Generated files under `generated/` are never
committed. The committed `gorest.Client` façade embeds the generated client and
adds only base URL, HTTP client, and Primer Bearer token setup.

```bash
cd curriculum-studio
make contracts-openapi-emit
make clients-go-rest-build
```
