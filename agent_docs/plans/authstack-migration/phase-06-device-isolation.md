# Phase 6: Device boundary certification

## Goal

Certify that authstack migration did not absorb, weaken, or bypass any device pairing/token system. LMS workstation, TV/Android, and Primer Tasks student browser/Android credentials remain narrow product-local capabilities with their original one-device/one-student or one-TV bindings, replay rules, revocation, rotation, and re-pair behavior.

Device tests begin in Phase 2 and run throughout cutovers; this dedicated phase performs final cross-product, bidirectional certification before legacy human/service auth can be removed.

## BDD Success Criteria

#### Scenario: LMS workstation pairing lifecycle is unchanged

- **Given** an authstack-authenticated local parent/admin and an unpaired workstation
- **When** the parent issues a one-use code and the workstation pairs
- **Then** LMS returns a product-local opaque device token once and binds it to the intended device/student
- **And** replay/expired/wrong code is rejected
- **And** rotate/revoke immediately rejects the old token and requires re-pair
- **And** the unprivileged TUI never receives the durable token.

#### Scenario: TV device pairing lifecycle is unchanged

- **Given** an authstack-authorized TV admin and an unpaired Android TV device
- **When** the admin issues a code and the device claims it
- **Then** TV stores the local token hash/binding and device routes work
- **And** code replay/concurrent second claim fails
- **And** pairing-code rotation or revocation invalidates the old token and client returns to pairing
- **And** no selected-provider identity is created for the device.

#### Scenario: Tasks student pairing lifecycle is unchanged

- **Given** an authstack-authenticated Tasks parent and a student browser/Android client
- **When** the parent issues a QR/code and the client claims it
- **Then** Tasks creates only its existing product-local student/device/session binding
- **And** the QR/code remains one-use, expiring, and replay-safe
- **And** archive/revoke clears access and re-pair is required
- **And** Android encrypted token storage and student WebSocket/API behavior remain intact.

#### Scenario: Device credentials cannot cross into human or service routes

- **Given** valid LMS, TV, and Tasks device tokens
- **When** each is presented to every product's human/admin BFF/API, MCP, gRPC, and service-only representative route
- **Then** every presentation is rejected with sanitized unauthenticated semantics
- **And** token shape/header fallback cannot reinterpret it as authstack API-key/session/M2M identity.

#### Scenario: Authstack identities cannot bypass pairing

- **Given** valid selected-provider human sessions/OAuth tokens and M2M tokens for all Primer audiences
- **When** they are presented directly to LMS `/student/*`, TV catalog/playback/device, and Tasks student/device/WebSocket routes
- **Then** every route requiring a local device credential rejects them
- **And** human authorization to issue/revoke a pairing code does not itself grant device access
- **And** no authstack principal is silently mapped to a student/device.

#### Scenario: Cross-product device tokens remain isolated

- **Given** one valid token from each device system
- **When** tokens are exchanged among LMS, TV, and Tasks device routes
- **Then** all foreign product tokens are rejected
- **And** revoking one product's token does not invalidate unrelated device or human/service sessions.

## Implementation Instructions

1. Freeze device route groups before authstack middleware composition. LMS `StudentDeviceGuard`, TV `requireDevice`, and Tasks student/device/WebSocket identity functions must remain separate routers/guards backed by their existing local tables/repositories.
2. Do not import authstack into device credential generation, hash, claim, storage, or validation packages. Authstack may protect the human action that issues/revokes codes; it must not issue or verify resulting device credentials.
3. Preserve LMS privileged broker/token-file/cache boundaries and 0600/encrypted storage behavior. Verify migration cleanup does not delete `authutil` functions still required by devices merely because parent opaque sessions once shared the package.
4. Preserve TV pairing code/token hash/revoked/paired fields and Android unauthorized→clear pairing→re-pair behavior. Remove only admin shared-key behavior, not TV device bearer parsing.
5. Preserve Tasks pairing/auth migrations, browser/Android student sessions, encrypted token store, WebSocket bearer handling, archive/revoke semantics, and tenant/student composite constraints.
6. Add a reusable compatibility matrix test per service that mints/creates credentials through real public pairing and authstack flows, then presents them to representative route classes. Never use string literals alone as proof of local binding.
7. Ensure OpenAPI/security docs distinguish selected-provider browser/M2M schemes from opaque local device schemes. Generated clients must not reuse a generic global bearer provider across admin and device clients.
8. Add logging/redaction tests around pairing failures, unauthorized requests, Android logs, broker IPC, and WebSocket errors. Credentials/QR material must not appear in logs or URLs.
9. Benchmark or race-test concurrent code claim/revoke paths where existing guarantees depend on transactional one-winner semantics.

## End-to-End Test Plan

- **LMS real flow:** start LMS with real Postgres; authstack parent logs in; create student and pairing code via parent API; pair through workstation/broker public IPC/API; call student profile/work/session; replay code; rotate/revoke; assert old token denied and fresh re-pair works. Inspect broker output/token file permissions without printing token.
- **TV real flow:** start TV with real Postgres; authstack admin creates device/code; Android or HTTP device client pairs; call catalog/guide/playback/heartbeat/release; concurrently replay code; rotate code/revoke; assert client clears pairing/re-pairs.
- **Tasks real flow:** run parent browser plus student browser and connected/headless Android where available; issue QR through public UI, pair, complete representative checklist/dialogue/artifact action, archive/revoke, assert generic unavailable state, then re-pair.
- Generate valid human and M2M credentials for each audience and valid local device tokens through each pairing endpoint. Execute the full credential×route-class matrix, including MCP and gRPC metadata, and assert every non-diagonal use rejects.
- Verify an authstack parent/admin can issue and revoke pairing codes but cannot use the same session/token on device routes.
- Restart each service/device client and prove local token persistence/revocation behavior remains as before. Provider outage must not invalidate already paired device operation unless the product action inherently needs a human/service call.
- Run concurrency/race tests for one-use claims and revoke/claim races; inspect persisted rows for exactly one winner and correct revoked/bound state.
- Run existing suites such as LMS `student_flow`, `device_assign`, broker/cache tests; TV `device`, `schedule`, `release`, identity boundary tests; Tasks pairing/security/browser/Android tests; then full product/root/coverage gates.

No mocked repository or preinstalled context principal may replace the public pairing/mint step in certification evidence.

## Anti-Cheating Audit

- Diff device packages/schemas against the integrated pre-authstack baseline and require justification for every behavioral change.
- Search device code for authstack/provider imports, canonical human link lookups, or acceptance of OAuth/M2M/session credential kinds.
- Verify device tokens used in matrix tests were minted through real pair endpoints and stored/hashed in real Postgres.
- Check human auth can only administer pairing, not set arbitrary device/student identity or receive existing plaintext tokens.
- Inspect generic bearer middleware ordering so authstack cannot consume a device request first or fall through after failure.
- Check TV/LMS/Tasks generated clients keep separate credential providers and do not install human auth globally on device clients.
- Verify revoke/rotate assertions include persisted state and a subsequent public request, not only response status.
- Inspect logs, Android Logcat fixtures, broker IPC, URLs, screenshots, and test reports for credential/QR leakage.
- Reject claims based only on existing unit tests if cross-product and authstack-to-device directions are absent.
- Confirm provider logout/revocation and service credential rotation do not mutate device-token records.

## Completion Gate

- [ ] LMS workstation, TV/Android, and Tasks browser/Android pairing/re-pair lifecycles pass through real public boundaries and real stores.
- [ ] One-use, expiry, binding, concurrent claim, revoke, rotate, restart, and persistence semantics match the integrated baseline.
- [ ] Full bidirectional human/M2M/device and cross-product credential matrix rejects every invalid crossing.
- [ ] Device code contains no authstack/provider integration and device routers remain separate.
- [ ] Browser/Android/TUI credential custody and redaction checks pass.
- [ ] Existing focused, full, race, coverage, browser, Android, build, and generated-contract gates pass.
- [ ] Baseline diff audit finds no unexplained device behavior migration.
- [ ] Device certification evidence is recorded before Phase 7 may delete legacy human/service auth.
