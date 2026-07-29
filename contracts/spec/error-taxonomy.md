# Error taxonomy v1

Every service returns a stable code and a retry classification. Callers must
not infer retry safety from transport status alone.

| Code | Class | Retry | Meaning |
| --- | --- | --- | --- |
| `VALIDATION_FAILED` | input | `NEVER_SAME_INPUT` | Schema, enum, decimal, or bound-field validation failed. |
| `UNAUTHENTICATED` | identity | `AFTER_REAUTHENTICATION` | No valid authenticated session. |
| `SCOPE_MISMATCH` | authorization | `NEVER` | User/account/session ownership did not match server scope. |
| `TOOL_NOT_AUTHORIZED` | authorization | `NEVER` | MCP tool is not in the frozen inventory. |
| `CONFIRMATION_REQUIRED` | authorization | `AFTER_NEW_CONFIRMATION` | Risk-increasing action lacks a valid ticket. |
| `CONFIRMATION_EXPIRED` | authorization | `AFTER_NEW_CONFIRMATION` | Ticket expiry has passed. |
| `CONFIRMATION_MISMATCH` | authorization | `NEVER_SAME_TICKET` | Digest or bound fields do not match. |
| `CONFIRMATION_ALREADY_CONSUMED` | idempotency | `RETURN_EXISTING_RESULT` | Nonce was already atomically consumed. |
| `RISK_REJECTED` | risk | `AFTER_NEW_INTENT` | Deterministic policy rejected the intent. |
| `DATA_STALE` | data | `AFTER_FRESH_SNAPSHOT` | Required market or account state is stale. |
| `STATE_TRANSITION_REJECTED` | state | `NEVER_SAME_TRANSITION` | Requested state edge is not allowed. |
| `DUPLICATE_REQUEST` | idempotency | `RETURN_EXISTING_RESULT` | Idempotency key already has a recorded result. |
| `UPSTREAM_UNAVAILABLE` | dependency | `BOUNDED_BACKOFF_BEFORE_DISPATCH` | A read-only dependency is unavailable before any write dispatch. |
| `UNKNOWN_REQUIRES_RECONCILIATION` | execution | `RECONCILE_ONLY` | Dispatch outcome is unknown; blind retry is forbidden. |
| `MANUAL_RECONCILIATION_REQUIRED` | execution | `OPERATOR_ONLY` | Automated reconciliation cannot establish external truth. |
| `INTERNAL_ERROR` | internal | `POLICY_DEPENDENT` | Unexpected failure with a correlation ID and redacted detail. |

`UNKNOWN_REQUIRES_RECONCILIATION` is a domain state, not a generic transient
error. The only permitted next action is reconciliation using recorded
operation, attempt, client-order, exchange-order, open-order, historical-order,
and fill evidence.
