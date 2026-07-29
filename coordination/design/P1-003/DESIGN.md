# P1-003 design R2 — API, identity, session, ownership, and response replay

## Status, authority, and scope

| Field | Value |
| --- | --- |
| Design task | `P1-003-DESIGN-R2` |
| Candidate base | `19b778785565d31dc095996ce4d4b0f7d58bebdf` |
| Frozen interface capsule | `f61474ba4f96624f34ff478bd0047c28e72becab` |
| Authority | design and future-test planning only |
| This change set | `coordination/design/P1-003/**` only |
| Implementation status | not started |

The capsule is the controlling interface authority.  Its typed DTOs, enums,
eleven workflow write sets, exact response/cache semantics, and subject table
override old design prose.  This document neither changes a shared contract nor
authorizes implementation, merge, deployment, production, wallet/signing,
exchange access, automatic trading, or live orders.

The future server implementation owns only `services/trading-core/api/**`,
`auth/**`, `session/**`, and `ownership/**` (plus separately admitted module
metadata).  It must not modify shared contracts, `docs/`, `coordination/`,
`infra/`, the Hermes service, or the exchange adapter.  P1-004 is responsible
for PostgreSQL persistence and P1-005 for publication; P1-003 creates typed
plans and never opens NATS or creates an execution subject.

## Required typed boundary

All security and persistence boundaries use capsule checked constructors for
immutable tagged values: `UUID`, `RequestID`, `OperationID`, `EventID`,
`AggregateID`, `Digest32` and its digest tags, `Nonce96`, `Ciphertext`,
`CanonicalJSON`, `UTCInstantMs`, `MonotonicTick`, `RouteTemplate`, and exact
`Subject`. Mutable maps, `any`, raw string enums, nullable discriminators, and
caller-owned decoded objects fail before routing, idempotency, or locking.

Unknown enum values fail closed; they never acquire a default.  The implementation
uses the capsule's exact `AuthorityScopeKind`, `StableKeyKind`, `ResultClass`,
`ReplayDecision`, `ClaimState`, `CacheState`, `OutboxPublicationState`,
`AggregateType`, `EventKind`, `ConfirmationState`, and
`ConfirmationConsumeDecision` values.  In particular the durable Outbox state
is `PENDING`, not an inferred `UNPUBLISHED` alias.

### Server-authored authority and claim scope

`AuthorityScopeDTO` is an exhaustive tagged union:

| Variant | Required server facts | Forbidden facts | Applicable check |
| --- | --- | --- | --- |
| `OWNER` | user, trading account, ownership | source, operation | lock active ownership and target in the transaction |
| `AUTH_SECURITY_SOURCE_ONLY` | source key ID/digest, control-plane aggregate | user/account/ownership | verify source-key record and control-plane aggregate only |
| `AUTH_SECURITY_RESOLVED_USER` | source key ID/digest, resolved user, control-plane aggregate | account/ownership | verify resolved active user and source/control-plane binding |
| `SYSTEM` | system operation, control-plane aggregate | user/account/ownership/source | verify server-created operation and aggregate |

No body, URL, query, header, cookie, WebSocket frame, model output, broker
credential, or client identity field grants authority.  Source-only authority
is not a weak owner check: it performs **no** user, account, ownership, or
owner-object lookup and cannot read or mutate owner business tables.  Resolved
user security authority never adds a trading-account ID.

`IdempotencyScopeDTO` is non-empty and comprises the tagged authority, route
template, stable-key kind, and stable-key digest.  Owner uniqueness is
`(user_id,trading_account_id,ownership_id,route_template,stable_key_kind,stable_key_digest)`;
all control-plane variants use
`(authority_kind,control_plane_aggregate_id,route_template,stable_key_kind,stable_key_digest)`.
There is no nullable/global scope.  Ten workflows use `IDEMPOTENCY_KEY`;
`password_failure` uses `REQUEST_ID`. `request_id` remains linkage on every
claim, while raw keys never enter event payloads or logs.

## Exact response and rejection cache

Every persisted response, including a generic rejection, has one
`ResultClaimDTO`: a scope, `original_request_id`, immutable
`first_request_digest`, result class, claim time, and an operation ID only when
the workflow has one.  "Same key" means the selected scoped stable key; it
does not mean only request ID and never weakens claim uniqueness.

The exact response uses `ExactResponsePayloadDTO` and AES-256-GCM seal
metadata. `ResponseCacheAADDTO` binds schema version, claim ID, full authority
scope, route, stable-key kind/digest, request ID, first request digest,
server-derived authenticated-context digest, result class, key ID, created
time, and expiry.  Only typed `cache_control=no-store`, throttled
`retry_after_seconds`, and credential-only `set_cookie` are permitted; cookie
data is ciphertext. Raw credentials, tokens, passwords, request bytes,
client-supplied identity, and arbitrary headers cannot appear in AAD or clear
metadata.

The runtime `AEADKeyring` is deployment-shared across replicas/restarts,
outside PostgreSQL/source/logs/evidence.  New seals use the active encrypt key;
only explicitly retained decrypt keys can read unexpired rows.  Before
encryption, reserve `(key_id, Nonce96)` in append-only `aead_nonce_ledger`
using the claim ID; retry a collision at most three times.  The ledger survives
cache expiry and key IDs are never reused for different material.

The transaction rule is: take one Clock snapshot; validate applicable authority
and claim the scope; record request ID plus first digest; reserve nonce; write
all state/audit/history/Outbox effects; canonicalize AAD and seal; write the
cache row last; commit; only then transmit.  `expires_at` is snapshot plus
120,000 ms; equality is expired.  Cleanup erases ciphertext and transitions
the claim to `COMMITTED_RECONCILIATION_REQUIRED`, never re-executes it.

| Existing scoped key | Incoming first digest | Cache state | required decision |
| --- | --- | --- | --- |
| none | valid | n/a | `CLAIM_NEW` |
| same | equal | unexpired/decryptable | `REPLAY_EXACT` after applicable authority revalidation |
| same | different | any | `IDEMPOTENCY_CONFLICT` before decrypt or effect |
| same | equal | missing, expired, malformed, AAD mismatch, or key unavailable | `RECONCILIATION_REQUIRED_NO_REEXECUTION` |

`REPLAY_EXACT` creates no state, token, device, throttle failure, event, or
nonce. A server-keyed rejection locator may accelerate lookup only: it is not
an authority key or uniqueness key. For an unknown/source-only fifth password
failure, the authority is `AUTH_SECURITY_SOURCE_ONLY`, stable key is
`REQUEST_ID`, source facts alone may change, and the generic AES-GCM response
replays for 120 seconds. Same request ID/different digest conflicts; missing or
unreadable cache reconciles without a repeat failure.

## Clock and transport admission

`Clock.TakeTransactionSnapshot` returns `ClockSnapshotDTO` containing UTC
milliseconds, monotonic tick, and watermark version. One snapshot controls all
times in a transaction. A durable watermark rejects rollback; equality at an
expiry is expired. Serialization retry takes a new snapshot but keeps stable
request/event identity and may not resurrect a claim or extend a family.
Framework clocks, environment values, DB `now()`, and client time cannot decide
security state.

The sole limits source is `TransportLimitsDTO`
`fit.transport-limits.dev.v1`: request target 8192; headers 16384;
unauthenticated/authenticated/credential bodies 16384/65536/32768; response
65536; WebSocket data/application-control/protocol-control 65536/4096/125;
HTTP read-header/read/write/idle deadlines 5000/10000/15000/60000 ms;
WebSocket read/write/ping/reauth-issue/reauth-response
75000/10000/30000/60000/60000 ms; global HTTP/WS/inflight limits 256/128/256;
per-connection handler goroutines 2; admission queue 128; shutdown drain/force
close/cleanup 10000/10000/15000 ms. Equal byte/count limits are accepted; one
above fails before dispatch. A deadline at equality is expired. Any missing,
unknown, altered, environment- or flag-overridden value blocks startup.

## Eleven atomic workflows

All are one PostgreSQL transaction; a sealed cache is the final durable write
and response follows commit. `L` below is the number of newly locked throttle
dimensions.

| Workflow | Authority/key | Mandatory atomic facts and event plan |
| --- | --- | --- |
| WF-01 successful login | resolved active owner / idempotency key | throttle and pre-auth invalidation; new session/family/initial token digest; audit/history/Outbox; nonce/sealed credential; one `LOGIN_SUCCEEDED` |
| WF-02 real enrollment challenge | resolved active owner / idempotency key | throttle, one real challenge, audit/history/Outbox, nonce/sealed challenge; one `ENROLLMENT_CHALLENGE_ISSUED`; synthetic challenge writes nothing |
| WF-03 failed enrollment proof | real challenge owner / idempotency key | consume its one attempt, rejected-proof audit/history/Outbox, nonce/sealed generic rejection; no device/session/family; one `ENROLLMENT_PROOF_REJECTED` |
| WF-04 password failure | source-only or resolved-user security / request ID | source and optional user throttle, `2*L` ordered source-before-user audit/notification history+Outbox, nonce/sealed generic rejection; source-only never owner-lookups |
| WF-05 ordinary device enrollment | challenge owner plus active owner recheck / idempotency key | consume challenge, globally unique server-owned device, preserve old devices/sessions/families, new session/family/token, audit+notification/history+Outbox, nonce/sealed credential; `DEVICE_ENROLLED` audit then notification |
| WF-06 replacement enrollment | challenge owner plus active owner recheck / idempotency key | deterministic locks; consume challenge; revoke every prior device/session/family before replacement device/new session/family/token; two events `DEVICE_REPLACED` |
| WF-07 refresh rotation | server-resolved family owner / idempotency key | evaluate family deadline, rotated reuse, individual expiry, active rotation in order; rotate or revoke family/descendants; zero events on normal rotation, two `REFRESH_REUSE_DETECTED` on reuse |
| WF-08 device revocation | active owner plus loaded target-owner recheck / idempotency key | revoke device and dependent sessions/families; audit+notification/history+Outbox; two `DEVICE_REVOKED` |
| WF-09 session revocation | active owner plus loaded target-owner recheck / idempotency key | revoke session/family; audit+notification/history+Outbox; two `SESSION_REVOKED` |
| WF-10 owner mutation | active owner plus allowlisted loaded target recheck / idempotency key | allowlisted effect only; audit/history/Outbox, nonce/sealed result; one `OWNER_MUTATION_COMMITTED` on the exact owner subject |
| WF-11 confirmation consume | full active owner/account/ownership/session/device / idempotency key | exact confirmation/operation checks and CAS rules below; one owner audit/history/Outbox event; never an Attempt, Order, signer, dispatch, or exchange write |

The old `logout` route is not one of the accepted ten-workflow transaction
manifest entries. It is a contract gap: P1-003 must fail closed and stop for
remediation rather than treating it as WF-09 or inventing a subject/write set.

## Identity, refresh, device, and WebSocket semantics

Identity is always server-resolved. A successful login creates a random session
and refresh family; access expiry is 15 minutes. Individual refresh expiry is
`min(issued+7 days,family deadline)`, and the immutable family deadline is 30
days. Only digests persist. A rotated ancestor use revokes its whole family and
descendants; family expiry wins. Replacement enrollment revokes prior
device/session/family records before new credentials; ordinary enrollment does
not.

Every HTTP refresh, WebSocket upgrade/frame, and internal ingress rechecks
revocation. WebSocket binding is server-authored and bounded by the limits;
the system issues `REAUTH_REQUIRED` 60 seconds before expiry and the deadline
is `min(issued+60000,current_access_expiry)`. Reauth consumes its nonce exactly
once, atomically replaces the binding on success, and rejects/terminates at
expiry or deadline. Revocation closes/rejects within the frozen five-second
contract. Missing, duplicated, delayed, or reordered WebSocket delivery has no
authority; durable server state is reconciled over HTTP.

## Confirmation consumption and durable relations

`POST /v1/confirmations/{confirmation_id}/consume` accepts only path ID,
idempotency key, and `confirmation_hash` assertion. Authority comes from full
server request context. The durable `ConfirmationFactDTO` includes non-null
user/account/ownership/session/device, purpose, intent/digest, risk snapshot,
hash, immutable globally unique nonce, issue/expiry/state, and consumption
linkage. Purpose is only `OPEN_POSITION` or `INCREASE_POSITION` from accepted
intent; all others fail before ticket issue.

P1-004 must enforce non-null full tuple candidate keys and composite FKs across
Confirmation, pre-linked `AWAITING_CONFIRMATION` Operation, owner-scoped claim,
and unique `confirmation_consumptions`. Its `(confirmation_id,request_claim_id)`
FK must equal the Confirmation's consumed claim; separate application checks
are insufficient. At one snapshot, lock ownership, claim, Confirmation, and
Operation deterministically; verify active tuple, purpose, intent, digest,
hash, candidate/FK bindings, and expected state versions.

Success performs exactly one `ISSUED -> CONSUMED` Confirmation CAS, one
`AWAITING_CONFIRMATION -> CONFIRMED` Operation CAS, the immutable consumption
link, one owner audit/history/Outbox row on
`fit.platform.v1.owner.owner-mutation-committed` with aggregate type
`OPERATION`, nonce reservation, and sealed Operation response. At expiry
equality/later, CAS to `EXPIRED` and the linked Operation to `EXPIRED` with the
same one-event audit shape. Any cross-owner/account/ownership/session/device,
hash, purpose, intent, relation, inactive, or operation mismatch seals a
generic failure with zero CAS/audit/history/Outbox. Cache miss after consumption
can only use full-authority durable Operation reconciliation; it never repeats
CAS, events, or creates Attempt/Order.

## Outbox and mapping boundary

P1-003 creates a typed `OutboxMaterializationPlanDTO` inside the transaction;
P1-004 verifies and persists it without business inference. It locks one
aggregate head. For head `(v,last)` and `n` drafts, versions are `v+1..v+n`,
first predecessor is `last` (forbidden at v1), later predecessors are prior
new event IDs, every draft creates one immutable history and one Outbox row,
then one head CAS advances to `(v+n,last_new)`. Any mismatch rolls back every
effect, claim, cache, and nonce. Retries retain event IDs/order.

The API-to-PostgreSQL-to-JetStream mapping is exact: request ID and first
digest map only to `idempotency_records`; authority tuple/source key/control
plane aggregate map to typed DB columns and permitted event scope; sealed cache
and nonce ledger are never published; an `EventDraftDTO` maps field-for-field
to history/Outbox and then the same EventEnvelope. The publisher consumes only
the P1-004 Outbox row. Only the capsule's 20 subject/kind/scope/consumer/
aggregate rows are valid; no wildcards or invented subjects. WF-10 uses owner
aggregate `OWNER_OPERATION`; WF-11 uses `OPERATION`. There is no accepted
Attempt, Order, or trading-domain subject mapping.

## Implementation gates and acceptance

Implementation is blocked until the exact capsule API inventory and accepted
typed P1-002 descendant are available; dependency licenses and Go version are
admitted; P1-004 accepts these typed authority/cache/nonce/clock/outbox plans;
the logout gap is remediated; and the full checklist plus machine manifest has
actual fixed-commit evidence. A green local test alone does not authorize
merge, deployment, production, or live trading.
