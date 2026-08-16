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
