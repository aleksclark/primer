# Phase 2: Accounts and external identities

**File:** `phase-02-accounts-and-external-identities.md`
**Depends on:** Phase 1
**Duration guess:** 3–5 days
**Migration stage:** S1
**Handoff wave:** W1

## Goal

Persist the Identity account model: internal UUID `sub`, external identities keyed solely by `(provider, provider_subject)`, optional password credentials for migration/break-glass, and repository APIs that **refuse email-based merge**. Establish claim-size bounds at the store boundary. Students remain non-entities here.

## Scope

### In scope

- Migration `00002_accounts.sql` (or continue numbering from Phase 1):
  - `accounts(id uuid PK, status, display_name, primary_email, primary_email_verified_at, created_at, updated_at)`
  - `external_identities(id, account_id, provider, provider_subject, UNIQUE(provider, provider_subject))`
  - `credentials_password(account_id PK/FK, password_hash, algorithm, rotated_at, disabled_at)`
- Providers enum/check: `google`, `password`, `breakglass` (extend later carefully)
- Repo: CreateAccount, UpsertExternalIdentity, FindByProviderSubject, SetPassword, CheckPassword, LockAccount
- Bounds: sub/id UUID; provider_subject ≤255; email ≤320; display_name ≤200; valid UTF-8; reject controls
- **No** UNIQUE on primary_email as login key; email index optional for recovery search only
- Unit/integration tests for no-merge semantics
- Document student non-scope in README

### Out of scope

- OAuth HTTP, sessions, Google token validation
- LMS `educators` column (Phase 13)
- Recovery codes table detail may stub until Phase 11 (if needed FK-ready, ok)

## BDD Success Criteria

#### Scenario: P2-S1 — Create account yields UUID sub

- **Given** empty Identity DB
- **When** repo creates an account
- **Then** row has UUID id usable as JWT `sub`
- **And** status is `active`

#### Scenario: P2-S2 — Account status transitions

- **Given** an active account
- **When** admin/repo sets status `locked`
- **Then** status persisted
- **And** later token mint phases will refuse (hook documented)

#### Scenario: P2-S3 — External identity unique on provider+sub

- **Given** identity `(google, sub-A)` linked to account A
- **When** insert duplicate `(google, sub-A)` for account B
- **Then** unique constraint/error
- **And** account B not linked

#### Scenario: P2-S4 — Same provider different sub is separate

- **Given** `(google, sub-A)` exists
- **When** upsert `(google, sub-B)` with same email string as A
- **Then** new account B is created (or explicit create path)
- **And** A and B remain distinct rows

#### Scenario: P2-S5 — Email match does not auto-merge

- **Given** password account with email E
- **When** Google subject G arrives with verified email E via repo API that only keys provider+sub
- **Then** no automatic link to password account
- **And** two accounts may share email display fields without shared id

#### Scenario: P2-S6 — FindByEmail is non-authoritative

- **Given** multiple accounts with same primary_email
- **When** caller lists by email
- **Then** API returns multiple or requires explicit disambiguation
- **And** no single “canonical login by email” without provider

#### Scenario: P2-S7 — Password hash stored with modern KDF

- **Given** plaintext password
- **When** SetPassword
- **Then** only hash stored (argon2id preferred or bcrypt cost≥10)
- **And** plaintext absent from DB bytes

#### Scenario: P2-S8 — CheckPassword constant-time failure

- **Given** account with password
- **When** wrong password presented
- **Then** false without user enumeration differences beyond existing LMS patterns
- **And** disabled_at password fails closed

#### Scenario: P2-S9 — No student principal type required

- **Given** schema and domain types
- **When** inventory roles/providers
- **Then** no `student` provider or required student table
- **And** docs state students out of Identity v1

## Implementation Instructions

1. Author goose up/down; add SCHEMA notes under `primer-identity/internal/db/SCHEMA.md`.
2. Domain types in `internal/domain`; repos in `internal/repo` using pgx (mirror LMS `repo` style, no import of LMS packages).
3. Password: prefer argon2id (`golang.org/x/crypto/argon2`) with params documented; bcrypt acceptable if matching LMS migration tooling temporarily — record choice in decision note in PR.
4. Store defense-in-depth validators for subject/email/name bounds (typed errors).
5. Factories in testutil for accounts/identities.
6. Grep gate later: ban `ON CONFLICT (primary_email)` merge patterns.

## End-to-End Test Plan

| ID | Setup | Action | Assert | Command |
| --- | --- | --- | --- | --- |
| P2-E1 | testcontainer | create account | UUID persisted; reload by id | `go test ./internal/repo -run AccountCreate` |
| P2-E2 | two inserts same provider+sub | second insert | unique violation | `go test ./internal/repo -run ExternalUnique` |
| P2-E3 | password acct email E; google sub different | create google acct same email | two account ids; no FK merge | `go test ./internal/repo -run NoEmailMerge` |
| P2-E4 | set password | check right/wrong/disabled | hash-only; wrong false | `go test ./internal/repo -run Password` |
| P2-E5 | schema inventory | query information_schema | no students table | `go test ./internal/db -run NoStudent` |

## Anti-Cheating Audit

- No application code path that `SELECT id FROM accounts WHERE primary_email=$1` then attaches Google sub without step-up link API.
- Tests must insert two subjects with identical emails and assert two ids.
- Password tests read raw DB column and assert not equal to plaintext.
- Do not implement “find or create by email” convenience in repo public API.

## Completion Gate

- [ ] P2-S* and P2-E* green
- [ ] Migration applied on fresh + upgrade from Phase 1
- [ ] Anti-cheat clean
- [ ] Bounds tests for max/max+1 subject and email

## Dependencies

- Upstream: Phase 1
- Downstream: Phases 3–6, 11, 13

## Rollback

- Revert migration only if never production; forward-fix additive only after any prod data.
