# Identity schema notes

Goose version table: `identity_goose_db_version` (never shared with LMS/TV/Studio).

## 00001_foundation

- `schema_meta` service marker only.

## 00002_accounts

### `accounts`

| Column | Notes |
| --- | --- |
| `id` | UUID PK, JWT `sub` |
| `status` | `active` \| `locked` |
| `display_name` | ≤200 chars; optional empty |
| `primary_email` | ≤320 chars; **nullable; NOT unique**; recovery/search only |
| `primary_email_verified_at` | optional |
| `created_at` / `updated_at` | timestamptz |

Index `accounts_primary_email_idx` on `lower(primary_email)` is non-unique.

### `external_identities`

| Column | Notes |
| --- | --- |
| `id` | UUID PK |
| `account_id` | FK → accounts ON DELETE CASCADE |
| `provider` | `google` \| `password` \| `breakglass` |
| `provider_subject` | ≤255; **UNIQUE with provider** |
| `created_at` / `updated_at` | timestamptz |

**Stable external key = `(provider, provider_subject)` only.** Never auto-link or
merge by email. Same email + different provider subject ⇒ two accounts until an
explicit authenticated link API exists (later phase).

### `credentials_password`

| Column | Notes |
| --- | --- |
| `account_id` | PK/FK → accounts |
| `password_hash` | Argon2id PHC string only — never plaintext |
| `algorithm` | `argon2id` |
| `rotated_at` | last set |
| `disabled_at` | null = enabled; set ⇒ CheckPassword fails closed |

### Non-scope (v1)

- **No `students` table** and no `student` provider. Students are not Identity
  principals; LMS owns student records.
- No OAuth clients, sessions, JWKS, or refresh tokens (later migrations).
- No `FindOrCreateByEmail` repository API.

### Password KDF

Argon2id PHC (`$argon2id$v=19$m=65536,t=3,p=4$…`): 64 MiB memory, time=3,
parallelism=4, 16-byte random salt, 32-byte key. See `internal/password`.

## 00003_stytch_mappings

Exact `(project_id, organization_id, member_id)` tuple mapping. Email and
profile fields are intentionally absent.

Required indexes only:

- `stytch_mappings_pkey` on `id`
- `stytch_mappings_tuple_uidx` unique on `(project_id, organization_id, member_id)`
- `stytch_mappings_account_id_idx` on `account_id`

There is no redundant `stytch_mappings_project_org_idx`; the unique tuple
constraint already covers that lookup.

## 00004_ib1_oauth_clients

IB1 OAuth client registry. See `00004_ib1_oauth_clients.sql`.

## 00005_ib1_broker_exchange

IB1 broker transactions, grants, and authorization codes. See
`00005_ib1_broker_exchange.sql`.

## 00006_ib2_signing_keys

Persistent ES256 signing-key custody for IB2 issuance. Donor
`53b693cc93c8cccb15109d90bae90811029af893` `00004_signing_keys` is
concepts-only (different column names: `sealed_private` / `retire_after` /
`updated_at`). This migration is authored from the IB0 data contract, not
copied as donor 00004.

### `signing_keys`

| Column | Notes |
| --- | --- |
| `id` | UUID PK |
| `kid` | unique, nonempty, ≤128, no control characters |
| `alg` | fixed `ES256` |
| `public_jwk` | JSONB public JWK only |
| `sealed_private_key` | BYTEA AES-GCM envelope; NULL only when `destroyed` |
| `key_version` | smallint > 0 |
| `status` | `next` \| `active` \| `retired` \| `destroyed` |
| `not_before` / `not_after` | `not_after` optional and must be > `not_before` |
| `created_at` | timestamptz; active requires `activated_at >= created_at` |
| `activated_at` / `retired_at` / `destroyed_at` | lifecycle stamps; IB2 writes only `next`/`active` |

Partial unique indexes `signing_keys_one_active_uq` and
`signing_keys_one_next_uq` allow at most one `active` and one `next` row.
Checks: `signing_keys_profile_ck`, `signing_keys_lifetime_ck`,
`signing_keys_status_material_ck`. Down drops only this table.

IB2 proves the table and at-most-one active/next constraints needed for
issuance. Rotate/retire/destroy admin operations wait for IB7. App HTTP
signer readiness is deferred to the integration HTTP lane and is not a
blocker for this donor custody lane. Production `IDENTITY_KEY_*` missing or
malformed must fail `config.Load`/`Validate` before app migrate/listen;
non-production may remain uncomposed when custody is disabled.
