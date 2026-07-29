# Error taxonomy v1

Every service returns a stable code and a retry classification. Callers must
not infer retry safety from transport status alone.

| Code | Class | Retry | Meaning |
| --- | --- | --- | --- |
| `VALIDATION_FAILED` | input | never-same-input | Schema, enum, decimal, or bound-field validation failed. |
| `UNAUTHENTICATED` | identity | after-reauthentication | No valid authenticated session. |
| `SCOPE_MISMATCH` | authorization | never | User/account/session ownership did not match server scope. |
| `TOOL_NOT_AUTHORIZED` | authorization | never | MCP tool is not in the frozen inventory. |
| `CONFIRMATION_REQUIRED` | authorization | after-new-confirmation | Risk-increasing action lacks a valid ticket. |
| `CONFIRMATION_EXPIRED` | authorization | after-new-confirmation | Ticket expiry has passed. |
| `CONFIRMATION_MISMATCH` | authorization | never-same-ticket | Digest or bound fields do not match. |
| `CONFIRMATION_ALREADY_CONSUMED` | idempotency | return-existing-result | Nonce was already atomically consumed. |
| `RISK_REJECTED` | risk | after-new-intent | Deterministic policy rejected the intent. |
| `DATA_STALE` | data | after-fresh-snapshot | Required market or account state is stale. |
| `STATE_TRANSITION_REJECTED` | state | never-same-transition | Requested state edge is not allowed. |
| `DUPLICATE_REQUEST` | idempotency | return-existing-result | Idempotency key already has a recorded result. |
| `UPSTREAM_UNAVAILABLE` | dependency | bounded-backoff-before-dispatch | A read-only dependency is unavailable before any write dispatch. |
| `UNKNOWN_REQUIRES_RECONCILIATION` | execution | reconcile-only | Dispatch outcome is unknown; blind retry is forbidden. |
| `MANUAL_RECONCILIATION_REQUIRED` | execution | operator-only | Automated reconciliation cannot establish external truth. |
| `INTERNAL_ERROR` | internal | policy-dependent | Unexpected failure with a correlation ID and redacted detail. |

`UNKNOWN_REQUIRES_RECONCILIATION` is a domain state, not a generic transient
error. The only permitted next action is reconciliation using recorded
operation, attempt, client-order, exchange-order, open-order, historical-order,
and fill evidence.
