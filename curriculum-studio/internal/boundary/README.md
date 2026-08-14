# internal/boundary

Shared **wire helpers** at the service edge: opaque id parsing, enum encode/decode
maps derived from contract sources, pagination envelope helpers.

## Hard rule (REQ-OWN-3)

This package is **not** a third hand-maintained DTO or enum catalog.

- Do not duplicate proto messages field-for-field as Go structs for public export
- Do not check in a primary `enums.yaml` value list that humans edit as SoT
- Closed enum parity is enforced by `tools/contract-gates` extracting from
  proto + OpenAPI + DB CHECK (see C2)
