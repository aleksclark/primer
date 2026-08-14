# internal/authn

JWT / JWKS validation adapter **interface** for Curriculum Studio.

**Consumes (Identity-owned):** JWKS document, access tokens with
`aud=curriculum-studio`, scopes, migration `X-Service-Token` alias rules.

**Does not own:** token minting, sessions, OIDC login, Google RP, account DB.

Production-auth wiring lands with Platform S2 + Identity I3/I12. Contracts may
ship credential-free test verifiers for harnesses only.
