# Studio outbox webhooks

Durable domain-event delivery for Curriculum Studio. The worker claims
unpublished `outbox_events` with `FOR UPDATE SKIP LOCKED`, fans out to
**active same-workspace** `webhook_endpoints`, and POSTs a signed JSON
`DomainEvent` envelope. Studio is correct with zero subscribers.

## Signature verification

Each delivery is an HTTP `POST` of the exact JSON body with:

| Header | Value |
| --- | --- |
| `Content-Type` | `application/json` |
| `X-Curriculum-Studio-Event-Id` | outbox event UUID |
| `X-Curriculum-Studio-Event-Type` | dotted type (`curriculum.created`, …) |
| `X-Studio-Timestamp` | unix seconds (UTC) |
| `X-Studio-Signature` | `v1=<hex>` |
| `X-Curriculum-Studio-Signature` | same `v1=<hex>` (C9-compatible alias) |
| `Idempotency-Key` | `wh:<endpoint-uuid>:<event-uuid>` |

### Algorithm (`v1`)

```
mac = HMAC-SHA256(secret, "{unixSeconds}." + raw_body)
header = "v1=" + hex(mac)
```

`secret` is resolved from the endpoint's `secret_ref` pointer. The raw HMAC
secret is never stored on the endpoint row, never returned by the API, and
must not be logged.

### Receiver check

1. Read `X-Studio-Timestamp` and the raw request body (do not re-serialize).
2. Reject if the timestamp is outside your replay window (recommend ±5 minutes).
3. Compute `v1=` as above with your stored secret.
4. Compare using a constant-time equality check against `X-Studio-Signature`.

Do not mark the event processed unless the signature validates. Retry POSTs
reuse the same `Idempotency-Key`; treat that key (or `id` in the JSON body)
as the dedupe identity.

## Isolation and retries

- Fanout never uses a global endpoint. Only `status=active` endpoints in the
  event's workspace whose `event_types` include the type (or are empty = all
  types) receive the event.
- `(endpoint_id, event_id)` is unique. Re-dispatch reuses the same delivery.
- A delivery is `delivered` only after HTTP 2xx. Non-2xx / transport errors
  increment `attempt_count`, apply backoff, and dead-letter as `failed` after
  `MaxAttempts` (default 5).
- A restarted worker continues drain of unpublished events and of
  pending / failed / lease-expired deliveries. Outbox rows are never deleted
  by the worker.
