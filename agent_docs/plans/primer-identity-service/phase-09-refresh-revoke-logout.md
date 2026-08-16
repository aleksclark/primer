# 09: IB5 / Primer-owned service principals

**Status: STOP — candidate dependency under independent exact-tip review; no dispatch.**

## Goal

Implement distinct Primer machine identities and `client_credentials` without Stytch M2M or human impersonation, following [`../stytch-identity-ib0/02-http-oauth-bff-mcp-contract.md`](../stytch-identity-ib0/02-http-oauth-bff-mcp-contract.md).

## BDD Success Criteria

#### Scenario: IB5-S1 — Separate service subject
- **Given** an enabled confidential service client
- **When** it requests an allowed resource/scope
- **Then** `/oauth/token` issues a ≤15m single-audience ES256 JWT with `sub=identity:svc:<id>` and no refresh/provider association.

#### Scenario: IB5-S2 — No human/publish authority
- **Given** a service token
- **When** it requests human scopes or calls Studio human-write/publish-confirm tools
- **Then** it fails closed
- **And** no Stytch call or human account impersonation occurs.

## Implementation Instructions

Add service principal/credential tables and hashed/rotatable credentials or `private_key_jwt`. Enforce exact registered resource/audience/scope and subject class. MCP services may receive explicitly granted read/draft authority, but never initial publish-confirmation authority. Stytch M2M remains deferred.

## End-to-End Test Plan

Run IB5-E01/E02 through public token and Studio/MCP validation boundaries, including disabled/rotated client, scope escalation, wrong resource/audience, human impersonation and publish-confirm denial.

## Anti-Cheating Audit

No shared human/service row type that permits impersonation, static unrotated plaintext secret, refresh token, provider role/session, wildcard scope/resource, or test bypass of publish confirmation.

## Completion Gate

- [ ] IB5-S* and IB5-E01/E02 green.
- [ ] Credential rotation/redaction and OpenAPI/generated-client parity pass.
- [ ] Fresh exact-tip security review approves.
