# internal/authn

JWT / JWKS validation boundary for Curriculum Studio.

The validator accepts only Identity-owned ES256 `at+jwt` access tokens with the
single audience `curriculum-studio`, exact issuer, bounded lifetime, a known
JWKS `kid`, required scopes, and a signed public `client_id`. It rejects
`azp`, internal OAuth-client UUIDs, malformed claims, unknown keys, and all
unsigned/header-based identity fallbacks. JWKS refresh is fail-closed and
accepts key rotation only from the configured public set. Registration of a
public client ID is an Identity-side concern: Studio trusts only the signed
claim from its configured issuer, validates its public shape, and never maps it
from `azp` or an internal OAuth-client UUID.

Credential-free tests use `internal/testutil/jwttest`, which is outside the
Studio runtime path and mints fixtures only. Studio is a validator, never an
OP or token/session issuer. BFF login, cookie sessions, and CSRF belong to I7.

## Phase 9 boundary shape

The access-token wire shape is a compact three-segment JWT signed with ES256:
`{base64url(header)}.{base64url(payload)}.{base64url(raw-r||s)}`. The header is
`{"alg":"ES256","typ":"at+jwt","kid":"<key id>"}`. The payload contains
`iss`, `sub`, single-string `aud=curriculum-studio`, numeric `iat`/`nbf`/`exp`,
UUID `jti`, public `client_id`, and space-delimited `scope`; `azp` is not a
substitute for `client_id`. Human subjects use `sub=<uuid>` and become
`identity:<uuid>`; services use `sub=identity:svc:<id>`.

A future same-host BFF may bridge browser state with a host-only session cookie,
not an access/refresh token cookie: `__Host-studio-session` over HTTPS (or the
non-`__Host-` `studio-session` name only for HTTP test fixtures), `Path=/`,
`HttpOnly`, `SameSite=Lax`, and `Secure` whenever HTTPS is used. Cookie mutation
requests require a separate non-HttpOnly `X-CSRF-Token` double-submit cookie and
matching header. I7 owns issuing/validating that session and attaching its
already-validated Bearer identity; this S2 package currently accepts only the
Bearer form. Neither cookie may contain provider credentials or be logged.

**Does not own:** token minting, sessions, OIDC login, Google RP, account DB,
or provider credentials.
