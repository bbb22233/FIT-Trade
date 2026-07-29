# P1 cross-layer interface capsule

## 0. Status and verdict

| Field | Frozen value |
| --- | --- |
| Task | `P1-IFC-001` |
| Parent task | FIT-Trade local Codex architecture coordination |
| Authorization | Design-only cross-layer interface freeze |
| Accepted contract base | `9b4b113a4032f7f0eee88297781e3b2d069b1f18` |
| Branch | `codex/p1-interface-capsule` |
| Worktree | `/Users/guanlan/Documents/FIT-Trade-worktrees/p1-interface-capsule` |
| Allowed writes | `coordination/design/P1-INTERFACE-CAPSULE/**` only |
| Merge / deploy / production / live trading | `false / false / false / false` |

This capsule freezes the typed handoff between P1-003, P1-004, and P1-005. It
does not implement a shared type, migration, API, broker, exchange, wallet,
signer, model, order-submission, deployment, or production capability.

The accepted contract tree remains normative for its existing wire schemas and
manifests. This capsule is normative for the cross-layer adapter shape and the
previously open interface choices listed below. If an implementation cannot
represent both the accepted contract and this capsule without loss, it must
stop with a bounded predecessor-remediation proposal. It must not add a
compatibility map, nullable authority shortcut, untyped enum, or second wire
schema.

Design result: **FROZEN FOR INDEPENDENT FIXED-COMMIT REVIEW**. This is not
implementation acceptance and does not authorize integration.

## 1. Fixed evidence and precedence

The design was derived from these immutable/read-only inputs:

1. accepted platform contracts at
   `9b4b113a4032f7f0eee88297781e3b2d069b1f18`;
2. P1-004 design at
   `ce256978361df494ad95b2fd3cba39ccbc724da5`;
3. P1-005 design at
   `4589bed3f7f9287751a5c57db311a1373597a67f`.

The two P1-003 design files observed under
`/Users/guanlan/Documents/FIT-Trade-worktrees/p1-003-design-checklist` are
**untracked working-tree files**. The branch happened to point at
`95e77c967e2f9832b6804101d7ff8567c0530fed`, but that commit does not contain
either file. They are mutable, non-authoritative advisory snapshots only.
Their observed SHA-256 values are retained in the machine manifest solely to
detect later drift; they are not fixed-commit input, acceptance evidence, or a
predecessor API.

The coordinator reported an independent fixed-commit review of the P1-005
design with `P0=0`, `P1=0`, `P2=0`, `VERDICT=PASS`. That PASS closes only the
quality review of that design candidate. It explicitly leaves quarantine and
replay authority to P1-004 and grants no merge, deployment, production, or live
authorization.

Precedence is:

```text
accepted contracts
  -> this cross-layer capsule
    -> accepted P1-003 typed application ports
      -> accepted P1-004 PostgreSQL adapters
        -> accepted P1-005 delivery adapters
```

The advisory P1-003 snapshot and fixed P1-004/P1-005 prose are design evidence,
not permission to contradict the accepted contract. This capsule closes an
interface only where the accepted contract leaves implementation-facing shape
open.

## 2. Dependency direction and authority boundary

The only permitted dependency direction is:

```text
raw HTTP / WebSocket / internal-tool input
  -> P1-002 strict accepted-contract decoder
  -> P1-003 server-authored request context and use case
  -> P1-003 application port DTOs defined by this capsule
  -> P1-004 PostgreSQL transaction adapter
  -> PostgreSQL authoritative facts + immutable history + Outbox
  -> P1-005 Outbox publisher
  -> accepted EventEnvelope on exact JetStream subject
  -> P1-005 strict consumer
  -> P1-004 Inbox/quarantine/replay transaction adapter
  -> PostgreSQL authoritative effect
```

The following reverse dependencies are forbidden:

- contract/application packages importing PostgreSQL, NATS, JetStream, HTTP
  framework, or generated database types;
- P1-004 accepting a broker credential, subject, body, header, path, query, or
  client field as user/account authority;
- P1-005 constructing an Outbox event, changing an event identity/digest, or
  creating a business effect outside a P1-004 transaction;
- API code holding a NATS credential;
- a consumer ACK or TERM preceding the required PostgreSQL commit;
- an ownerless control-plane record joining to or mutating owner business rows;
- Operation/Order/ExecutionAttempt publication on a subject not present in the
  accepted 20-subject manifest.

PostgreSQL is the system of record. JetStream is at-least-once derived delivery
only. A service principal identifies a service role, never a user, trading
account, session, device, aggregate owner, or replay authority.

## 3. Primitive types and strict representation

Every boundary uses immutable typed values. A mutable map, `any`, caller-owned
JSON object, raw string enum, or nullable union discriminator is forbidden.

| Type | Representation and validation |
| --- | --- |
| `UUID` | RFC 4122 textual form at accepted wire boundary; 16-byte/`uuid` internally |
| `RequestID` | tagged `UUID`; always server-validated, never an authority grant |
| `OperationID` | tagged `UUID`; server-generated |
| `EventID` | tagged `UUID`; server-generated and globally unique |
| `AggregateID` | tagged `UUID`; meaning fixed by `AggregateType` |
| `Digest32` | exactly 32 bytes; lower-case 64-hex only at wire boundary |
| `RequestDigest` | tagged `Digest32`; accepted `fit.platform.request-digest.v1` |
| `PayloadDigest` | tagged `Digest32`; SHA-256 of RFC 8785 JCS payload bytes |
| `BindingDigest` | tagged `Digest32`; SHA-256 of canonical AAD bytes |
| `SourceKeyDigest` | tagged `Digest32`; HMAC-derived accepted source identity |
| `StableKeyDigest` | tagged `Digest32`; digest of the typed stable-key value |
| `Nonce96` | exactly 12 random bytes; never reused under the same AEAD key |
| `Ciphertext` | non-empty AES-256-GCM ciphertext including the 16-byte tag |
| `CanonicalJSON` | immutable UTF-8 RFC 8785 JCS bytes; strict schema validated |
| `UTCInstantMs` | exact UTC millisecond instant; no local timezone |
| `MonotonicTick` | opaque unsigned ordering value from the injected Clock |
| `RouteTemplate` | accepted, normalized route template; not raw request target |
| `Subject` | one exact value from the accepted 20-subject enum |
| `ConsumerRole` | exact accepted `platform-consumer` or `internal-notification-consumer` |
| `DurableConsumerName` | exact JetStream `PLATFORM_CONSUMER` or `INTERNAL_NOTIFICATION_CONSUMER` |

No tagged identifier can be assigned to another tagged identifier without an
explicit checked constructor.

### 3.1 Frozen enums

```text
AuthorityScopeKind =
  OWNER
  | AUTH_SECURITY_SOURCE_ONLY
  | AUTH_SECURITY_RESOLVED_USER
  | SYSTEM

StableKeyKind =
  IDEMPOTENCY_KEY
  | REQUEST_ID

ResultClass =
  CREDENTIAL_SUCCESS
  | CHALLENGE_SUCCESS
  | GENERIC_REJECTION
  | NON_CREDENTIAL_SUCCESS
  | NON_CREDENTIAL_REJECTION

ReplayDecision =
  CLAIM_NEW
  | REPLAY_EXACT
  | IDEMPOTENCY_CONFLICT
  | RECONCILIATION_REQUIRED_NO_REEXECUTION

ClaimState =
  CLAIMED_IN_TRANSACTION
  | COMMITTED_REPLAYABLE
  | COMMITTED_RECONCILIATION_REQUIRED

CacheState =
  ACTIVE
  | ERASED_AT_TTL
  | UNDECRYPTABLE_RECONCILIATION_REQUIRED

OutboxPublicationState =
  PENDING
  | CLAIMED
  | PUBLISHED

InboxRecordState =
  APPLIED

InboxMutationStage =
  RECEIVED_IN_TRANSACTION
  | APPLIED

QuarantineState =
  ISOLATED
  | REPLAY_AUTHORIZED
  | REPLAY_APPLIED
  | PERMANENTLY_DENIED

ReplayAuthorizationState =
  ACTIVE
  | CONSUMED
  | REVOKED
  | EXPIRED
  | DENIED_FINAL
  | MANUAL_REVIEW_FINAL

DeliveryMode =
  ORDINARY
  | CONTROLLED_REPLAY

AggregateType =
  AUTH_THROTTLE
  | ENROLLMENT_CHALLENGE
  | DEVICE
  | SESSION
  | REFRESH_FAMILY
  | OWNER_OPERATION
  | WAL_INCIDENT
  | OPERATION
  | EXECUTION_ATTEMPT
  | ORDER

EventKind =
  DEVICE_ENROLLED
  | DEVICE_REPLACED
  | DEVICE_REVOKED
  | ENROLLMENT_CHALLENGE_ISSUED
  | ENROLLMENT_PROOF_REJECTED
  | LOGIN_ACCOUNT_LOCKED
  | LOGIN_SOURCE_LOCKED
  | LOGIN_SUCCEEDED
  | REFRESH_REUSE_DETECTED
  | SESSION_REVOKED
  | OWNER_MUTATION_COMMITTED
  | WAL_ARCHIVE_INTERRUPTED

ConfirmationPurpose =
  OPEN_POSITION
  | INCREASE_POSITION

ConfirmationState =
  ISSUED
  | CONSUMED
  | EXPIRED

ConfirmationConsumeDecision =
  CONSUME_AND_CONFIRM
  | EXPIRE_AND_REJECT
  | REPLAY_EXACT
  | DURABLE_OPERATION_RECONCILIATION
  | IDEMPOTENCY_CONFLICT
  | FAIL_CLOSED_GENERIC_REJECTION
```

Unknown enum values fail before mutation. No component coerces an unknown value
to a default. The accepted Outbox wire state is `PENDING`, not the
`UNPUBLISHED` prose alias used in the P1-004 design. `RECEIVED_IN_TRANSACTION`
is an internal uncommitted Inbox stage; the accepted durable `InboxRecord`
status is only `APPLIED`.

## 4. Authority and non-empty idempotency scope

### 4.1 Tagged authority DTO

`AuthorityScopeDTO` is an exhaustive tagged union:

```text
OwnerAuthorityDTO {
  kind: OWNER
  user_id: UUID
  trading_account_id: UUID
  ownership_id: UUID
  authority_origin: SERVER_RESOLVED_ACTIVE_OWNERSHIP
}

SourceOnlyAuthSecurityAuthorityDTO {
  kind: AUTH_SECURITY_SOURCE_ONLY
  source_key_id: non-empty string
  source_key_digest: SourceKeyDigest
  control_plane_aggregate_id: AggregateID
  authority_origin: SERVER_DERIVED_SOURCE_SECURITY_SCOPE
}

ResolvedUserAuthSecurityAuthorityDTO {
  kind: AUTH_SECURITY_RESOLVED_USER
  source_key_id: non-empty string
  source_key_digest: SourceKeyDigest
  user_id: UUID
  control_plane_aggregate_id: AggregateID
  authority_origin: SERVER_RESOLVED_USER_AND_SOURCE_SECURITY_SCOPE
}

SystemAuthorityDTO {
  kind: SYSTEM
  system_operation_id: UUID
  control_plane_aggregate_id: AggregateID
  authority_origin: SERVER_CREATED_SYSTEM_OPERATION
}
```

Presence rules are exact:

| Variant | `user_id` | `trading_account_id` | `ownership_id` | source key | operation ID |
| --- | --- | --- | --- | --- | --- |
| `OWNER` | required | required | required | forbidden | forbidden |
| `AUTH_SECURITY_SOURCE_ONLY` | forbidden | forbidden | forbidden | required | forbidden |
| `AUTH_SECURITY_RESOLVED_USER` | required | forbidden | forbidden | required | forbidden |
| `SYSTEM` | forbidden | forbidden | forbidden | forbidden | required |

An absent required value, present forbidden value, empty string, nil UUID, or
unrecognized variant fails before an idempotency lookup or aggregate lock.

### 4.2 Applicable server-authority checks

Authority is checked according to the tagged variant:

- `OWNER`: P1-004 locks the exact ownership pair and target object and verifies
  active user/account/object ownership in the same transaction.
- `AUTH_SECURITY_SOURCE_ONLY`: P1-004 verifies the server-authored source-key
  record and its exact control-plane aggregate binding. It performs **no user,
  trading-account, ownership, or owner-object lookup**.
- `AUTH_SECURITY_RESOLVED_USER`: P1-004 verifies the server-resolved active user
  and source-key/control-plane binding. It never adds a trading-account ID to
  the security scope.
- `SYSTEM`: P1-004 verifies the server-created system operation and
  control-plane aggregate binding. It performs no owner lookup.

The source-only rule is not a weaker owner check. It is a distinct applicable
authority path for a non-business security effect. It cannot read or mutate an
owner business table.

### 4.3 Idempotency scope DTO

```text
IdempotencyScopeDTO {
  authority: AuthorityScopeDTO
  route_template: RouteTemplate
  stable_key_kind: StableKeyKind
  stable_key_digest: StableKeyDigest
}
```

The effective uniqueness keys are:

```text
OWNER:
  (user_id, trading_account_id, ownership_id, route_template,
   stable_key_kind, stable_key_digest)

AUTH_SECURITY_SOURCE_ONLY / AUTH_SECURITY_RESOLVED_USER / SYSTEM:
  (authority_kind, control_plane_aggregate_id, route_template,
   stable_key_kind, stable_key_digest)
```

Every component is non-null and non-empty for its variant. An owner claim
cannot collapse two ownership records even when user and account are equal.
There is no global `NULL = NULL` idempotency scope. Ownerless/control-plane
idempotency cannot collapse records from different source or system
aggregates.

`IDEMPOTENCY_KEY` is used by ten frozen workflows. `REQUEST_ID` is used by
`password_failure`. Raw idempotency keys are not event payloads or log fields.
The database stores their typed digest. `request_id` is always recorded as
linkage, but it is the stable claim key only when `stable_key_kind=REQUEST_ID`.

## 5. Exact response and rejection cache

### 5.1 Typed claim and seal DTOs

```text
ResultClaimDTO {
  claim_id: UUID
  idempotency_scope: IdempotencyScopeDTO
  original_request_id: RequestID
  first_request_digest: RequestDigest
  result_class: ResultClass
  operation_id: OperationID?       // present only when the workflow has one
  claimed_at: UTCInstantMs
}

ExactResponsePayloadDTO {
  schema_version: "fit.interface.exact-response.v1"
  http_status: integer 100..599
  content_type: "application/json"
  body: CanonicalJSON
  header_values: {
    cache_control: "no-store"
    retry_after_seconds: unsigned 32-bit integer?
    set_cookie: bounded opaque string?
  }
}

ResponseCacheAADDTO {
  schema_version: "fit.interface.response-cache-aad.v1"
  claim_id: UUID
  authority_scope: AuthorityScopeDTO
  route_template: RouteTemplate
  stable_key_kind: StableKeyKind
  stable_key_digest: StableKeyDigest
  original_request_id: RequestID
  first_request_digest: RequestDigest
  authenticated_context_digest: Digest32
  result_class: ResultClass
  key_id: non-empty string
  created_at_ms: UTCInstantMs
  expires_at_ms: UTCInstantMs
}

SealedResponseCacheDTO {
  claim_id: UUID
  cache_version: "fit.interface.response-cache.v1"
  key_id: non-empty string
  nonce: Nonce96
  ciphertext: Ciphertext
  aad_digest: BindingDigest
  exact_response_digest: Digest32
  exact_response_length: unsigned 32-bit integer
  created_at_ms: UTCInstantMs
  expires_at_ms: UTCInstantMs
}
```

Optional response headers are a strict tagged/allowlisted structure. Arbitrary
headers, bearer values, password material, raw refresh/access tokens, raw
request bytes, and client-provided identity cannot appear in AAD or unencrypted
metadata. `set_cookie` is allowed only for a credential result and is inside
the ciphertext. `retry_after_seconds` is allowed only for a typed throttled
rejection. Every other header or result/header mismatch is rejected.

`authenticated_context_digest` is computed from the complete server-authored
workflow context after authentication/proof resolution. It is never computed
from headers, cookies, body authority aliases, model output, broker fields, or
NATS credentials.

Every cache row binds the immutable pair
`(original_request_id,first_request_digest)`. The phrase "same key" below means
the manifest/capsule-selected typed stable key: `IDEMPOTENCY_KEY` for ten
workflows, and `REQUEST_ID` for password failure.
The immutable request pair is never substituted for or allowed to weaken that
scoped stable-key uniqueness.

### 5.2 Keyring and nonce interface

```text
AEADKeyring {
  ActiveEncryptKey(now) -> { key_id, AES256KeyCapability }
  DecryptKey(key_id, now) -> AES256KeyCapability | KEY_UNAVAILABLE
}

ReserveAEADNonceDTO {
  key_id: non-empty string
  nonce: Nonce96
  claim_id: UUID
  reserved_at: UTCInstantMs
}
```

The runtime keyring is deployment-shared across replicas/restarts and is never
stored in PostgreSQL, source, fixtures, logs, metrics, traces, dumps, or
evidence. A retired decrypt key is retained until no unexpired cache row can
reference it.

P1-004 must persist `ReserveAEADNonceDTO` in an append-only
`aead_nonce_ledger`. Its primary key is `(key_id, nonce)` and `claim_id` is
unique. The nonce ledger is **not** the response cache and is **never erased by
the 120-second cache TTL**. Cache cleanup deletes only ciphertext/cache nonce
copies and retains both the durable result claim and the per-key nonce
reservation. A `key_id` is never reassigned to different key material.

Nonce reservation uses
`INSERT ... ON CONFLICT DO NOTHING RETURNING` before encryption. A conflict
generates another nonce and attempts reservation again in the same transaction,
up to exactly three attempts. No encryption occurs before a reservation
succeeds. Exhaustion rolls back the whole transaction as
`NONCE_RESERVATION_FAILED`; it never encrypts under or exposes a reused pair.

### 5.3 One transaction and replay decision

For every persisted response/rejection workflow:

1. take one `ClockSnapshotDTO`;
2. begin the P1-004 transaction;
3. validate the typed authority and claim the non-empty idempotency scope;
4. store `original_request_id` and immutable `first_request_digest`;
5. select the active key, obtain a fresh `Nonce96`, and successfully reserve
   `(key_id,nonce)` in the durable nonce ledger before encryption;
6. write every authoritative effect, audit, notification, aggregate history,
   and Outbox event;
7. canonicalize AAD and seal the exact response in memory using the reserved
   pair;
8. insert the one sealed cache row as the final durable write;
9. commit;
10. transmit the response only after commit.

The lookup decision is exact:

| Existing stable key | Incoming digest | Cache/key state | Decision |
| --- | --- | --- | --- |
| none | any valid first digest | n/a | `CLAIM_NEW` |
| same scoped key | equals `first_request_digest` | unexpired and decryptable | `REPLAY_EXACT` |
| same scoped key | differs | any | `IDEMPOTENCY_CONFLICT` before decrypt/effect |
| same scoped key | equals | missing/expired/erased/malformed/AAD mismatch/key unavailable | `RECONCILIATION_REQUIRED_NO_REEXECUTION` |

`REPLAY_EXACT` revalidates the applicable server authority first, locks the
claim, validates scope/key/digest/AAD, then decrypts. It creates no new session,
family, token, device, challenge, throttle failure, audit, notification, event,
attempt, order, or nonce reservation.

At `snapshot >= expires_at_ms`, the cache is expired. P1-004 atomically erases
the ciphertext row and moves the durable claim to
`COMMITTED_RECONCILIATION_REQUIRED`. The mutation never executes again. A
background sweeper uses the same transition. TTL is exactly 120,000 ms from
the committed transaction snapshot.

Both success responses and generic rejections use AES-256-GCM. A generic
rejection is not exempt from exact replay or nonce discipline.

### 5.4 Rejection-specific source-only rule

For an unknown identifier or source-only fifth password failure:

- `stable_key_kind=REQUEST_ID`;
- `first_request_digest` is the accepted canonical request digest;
- the authority variant is `AUTH_SECURITY_SOURCE_ONLY`;
- the generic response is AEAD-sealed for 120 seconds;
- the transaction may update only source throttle/control-plane security facts;
- the transaction performs no owner lookup and creates no phantom user/account;
- same request ID + same digest replays without another throttle increment;
- same request ID + different digest conflicts;
- missing/expired/unreadable cache reconciles without repeating the failure.

The server-only keyed rejection locator may be used as a lookup accelerator,
but it is not the uniqueness or authority key and cannot replace the durable
request-ID/first-digest claim.

## 6. Outbox materialization and multi-event versions

### 6.1 Typed plan

```text
AggregateRefDTO {
  aggregate_type: AggregateType
  aggregate_id: AggregateID
}

LockedAggregateHeadDTO {
  aggregate: AggregateRefDTO
  current_version: unsigned 64-bit integer
  last_event_id: EventID?
  fencing_token: non-empty opaque value
}

EventDraftDTO {
  event_id: EventID
  event_kind: EventKind
  subject: Subject
  scope: accepted Scope
  aggregate: AggregateRefDTO
  planned_aggregate_version: unsigned 64-bit integer >= 1
  previous_event_id: EventID?
  causation_id: UUID
  correlation_id: UUID
  occurred_at: UTCInstantMs
  payload_schema_version: non-empty accepted version
  payload: CanonicalJSON
  payload_digest: PayloadDigest
  audit_intent_ids: immutable non-empty/empty typed list
  notification_intent_ids: immutable non-empty/empty typed list
}

OutboxMaterializationPlanDTO {
  plan_id: UUID
  result_claim_id: UUID?
  authority: AuthorityScopeDTO
  aggregate_head: LockedAggregateHeadDTO
  ordered_events: immutable non-empty list<EventDraftDTO>
}

MaterializedEventDTO {
  event: accepted fit.platform.event-envelope.v1
  history_id: UUID
  outbox_id: UUID
}
```

`BeginAggregateAppend` locks the one aggregate head inside the workflow
transaction and returns `LockedAggregateHeadDTO`. P1-003 creates the fully typed
ordered event plan inside that same unit of work. P1-004 performs no business
inference: it verifies the plan, persists it, and CASes the head.

### 6.2 Exact version algorithm

For a locked head `(v,last)` and `n` drafts:

- draft `i` (one-based) has version `v+i`;
- draft 1 has predecessor `last`, except aggregate version 1 forbids it;
- every later draft has the immediately prior new event ID;
- every draft creates exactly one immutable history row and one Outbox row;
- all event IDs, subjects, kinds, scopes, payloads, digests, and link IDs are
  fixed before insert;
- after all `n` pairs are inserted, one CAS changes the head from
  `(v,last)` to `(v+n,event[n].event_id)`;
- zero-row CAS, any mismatch, or any insert failure rolls back the complete
  business effect, audit, notification, history, Outbox, cache, idempotency
  claim, and nonce reservation.

Aggregate ordering is per aggregate only. There is no global order. A retry
retains stable event IDs and event ordering through the result claim; it never
allocates an alternate chain.

### 6.3 Fixed event cardinalities

| Effect | Ordered event count |
| --- | ---: |
| successful login | 1 audit event |
| real enrollment challenge | 1 audit event |
| failed enrollment proof | 1 audit event |
| ordinary enrollment | 2: audit, notification |
| replacement enrollment | 2: audit, notification |
| device revocation | 2: audit, notification |
| session revocation | 2: audit, notification |
| refresh active rotation without reuse | 0 security events |
| refresh reuse | 2: audit, notification |
| owner mutation | 1 audit event |
| confirmation consumed or first expiry / Operation confirmed or expired | 1 owner audit event on the accepted owner-mutation subject, aggregate type `OPERATION` |
| password threshold lock | `2*L`, `L ∈ {0,1,2}`, source then user; audit then notification per dimension |

An implementation discovering a different contract-required cardinality stops
for remediation. It does not merge events or advance the aggregate head for a
zero-event result.

## 7. Authoritative Operation, Order, and Attempt facts

These are PostgreSQL owner facts. The accepted public/domain DTOs omit owner
fields because owner comes from the authenticated server context. The durable
rows must add an explicit owner tuple and preserve it through composite foreign
keys; no child infers ownership from an unscoped object ID.

### 7.1 Durable row DTOs

```text
OperationFactDTO {
  schema_version
  operation_id: OperationID
  user_id: UUID
  trading_account_id: UUID
  ownership_id: UUID
  result_claim_id: UUID
  intent_id: UUID
  confirmation_id: UUID?
  state: OperationState
  state_version: unsigned 64-bit integer
  rejection_code: bounded string?
  created_at: UTCInstantMs
  updated_at: UTCInstantMs
}

ExecutionAttemptFactDTO {
  schema_version
  attempt_id: UUID
  operation_id: OperationID
  user_id: UUID
  trading_account_id: UUID
  ownership_id: UUID
  client_order_id: exactly 32 lower-case hex
  attempt_number: integer 1..32
  state: ExecutionAttemptState
  created_at: UTCInstantMs
}

OrderFactDTO {
  schema_version
  order_id: UUID
  operation_id: OperationID
  attempt_id: UUID
  user_id: UUID
  trading_account_id: UUID
  ownership_id: UUID
  client_order_id: exactly 32 lower-case hex
  exchange_order_id: bounded string?
  symbol: BTC-PERP | ETH-PERP | SOL-PERP
  side: accepted Side
  quantity: accepted canonical positive decimal
  filled_quantity: accepted canonical non-negative decimal
  limit_price: accepted canonical positive decimal?
  reduce_only: boolean
  state: OrderState
  updated_at: UTCInstantMs
}
```

Frozen state enums are exactly the accepted contract values:

```text
OperationState =
  DRAFT | AWAITING_CONFIRMATION | EXPIRED | CONFIRMED | RISK_REVALIDATING
  | REJECTED | ADMITTED | DISPATCH_PENDING | DISPATCHED | ACKNOWLEDGED
  | UNKNOWN_REQUIRES_RECONCILIATION | MANUAL_RECONCILIATION | FINAL

ExecutionAttemptState =
  CREATED | SIGNED | SUBMITTED | ACKNOWLEDGED
  | UNKNOWN_REQUIRES_RECONCILIATION | REJECTED

OrderState =
  PENDING | OPEN | PARTIALLY_FILLED | FILLED | CANCELLED | REJECTED
  | UNKNOWN_REQUIRES_RECONCILIATION
```

This design does not authorize reaching signing, submission, or an exchange.
Those enum values are durable contract facts for later separately authorized
phases.

### 7.2 Foreign keys and deduplication

P1-004 must enforce:

| Identity | Required constraint |
| --- | --- |
| request | unique typed idempotency scope; binds immutable first request digest |
| trading-account ownership | unique `(user_id,trading_account_id,ownership_id)` on `TradingAccountOwnership`; referenced by Operation, Attempt, and Order |
| `operation_id` | primary key; unique `result_claim_id`; non-null owner tuple; unique `(operation_id,user_id,trading_account_id,ownership_id)`; composite FK to `TradingAccountOwnership` |
| Confirmation owner/intent | unique `(confirmation_id,user_id,trading_account_id,ownership_id,intent_id)` |
| Confirmation full authority | unique `(confirmation_id,user_id,trading_account_id,ownership_id,session_id,device_id,purpose,intent_id,intent_digest)` |
| confirmation consumption | unique confirmation and Operation; complete authority/intent composite FKs to Confirmation, Operation, and result claim |
| `attempt_id` | primary key; non-null owner tuple; unique `(attempt_id,operation_id,user_id,trading_account_id,ownership_id)` |
| attempt/operation | composite `(operation_id,user_id,trading_account_id,ownership_id)` FK |
| attempt/ownership | composite `(user_id,trading_account_id,ownership_id)` FK to `TradingAccountOwnership` |
| attempt sequence | unique `(operation_id,ownership_id,attempt_number)` |
| `client_order_id` | globally unique across attempts and orders; exact attempt/order equality |
| `order_id` | primary key; non-null owner tuple; unique `(order_id,attempt_id,operation_id,user_id,trading_account_id,ownership_id)` |
| order/attempt | unique `orders.attempt_id`; composite `(attempt_id,operation_id,user_id,trading_account_id,ownership_id)` FK |
| order/operation | composite `(operation_id,user_id,trading_account_id,ownership_id)` FK |
| order/ownership | composite `(user_id,trading_account_id,ownership_id)` FK to `TradingAccountOwnership` |
| `exchange_order_id` | unique `(trading_account_id,ownership_id,exchange_order_id)` when present |
| producer event | globally unique `event_id`; unique aggregate version |
| consumer event | unique `(consumer,event_id)` and immutable first payload digest |

`operations.state_version` changes only by expected-version CAS. An unknown
execution result cannot create a blind replacement order or automatic retry.
Any later separately authorized attempt uses the same Operation, a new explicit
Attempt, a new globally unique client-order ID, and a reconciliation decision.
All `user_id`, `trading_account_id`, and `ownership_id` columns on Operation,
ExecutionAttempt, and Order are `NOT NULL`. A child with the same user/account
but a different ownership cannot satisfy the Operation, Attempt, Order, or
`TradingAccountOwnership` composite FK and fails before any state change.

### 7.3 Confirmation consumption and durable Operation relation

The accepted `POST /v1/confirmations/{confirmation_id}/consume` request carries
only a path `confirmation_id`, an `Idempotency-Key`, and body
`confirmation_hash`. They are assertions, not authority. P1-003 must construct
the command from the accepted server-authored request envelope and load the
durable ticket:

```text
ConfirmationConsumeCommandDTO {
  request: accepted RequestEnvelope
  input: accepted ConfirmationConsumeMutationInput
  authority: OwnerAuthorityDTO
}

ConfirmationFactDTO {
  schema_version
  confirmation_id: UUID
  user_id: UUID
  trading_account_id: UUID
  ownership_id: UUID
  device_id: UUID
  session_id: UUID
  purpose: ConfirmationPurpose
  intent_id: UUID
  canonical_intent: CanonicalJSON
  intent_digest: Digest32
  maximum_loss: accepted canonical non-negative decimal
  maximum_loss_fraction: accepted canonical fraction
  post_trade_total_risk: accepted canonical non-negative decimal
  post_trade_total_risk_fraction: accepted canonical fraction
  liquidation_price: accepted canonical positive decimal
  estimated_fees: accepted canonical non-negative decimal
  slippage_budget: accepted canonical non-negative decimal
  confirmation_hash: Digest32
  confirmation_nonce: accepted 32..128 character base64url string
  issued_at: UTCInstantMs
  expires_at: UTCInstantMs
  state: ConfirmationState
  consumed_at: UTCInstantMs?
  consumed_request_claim_id: UUID?
  expired_at: UTCInstantMs?
}
```

`purpose` is derived only from the accepted ticket intent:
`OPEN -> OPEN_POSITION` and `INCREASE -> INCREASE_POSITION`; every other
position effect is rejected before ticket issuance. `intent_digest` is
`SHA-256(JCS(ticket.intent))`. `confirmation_hash` is recomputed from the full
accepted `ConfirmationTicket` field set in `confirmation-fields.json` and
`confirmation-hash.md`; every hash input, including the one-time nonce and
risk snapshot fields, remains available in the durable ticket. The nonce never
enters logs, audit payloads, events, cache metadata, or observability.
`issued_at` equals the accepted ticket `created_at`.

Database constraints are mandatory:

- every Confirmation, Operation pre-link, consumption, and owner result-claim
  authority column named below is `NOT NULL`; no nullable column or partial
  tuple may satisfy an authority FK;
- `confirmations` has primary key `confirmation_id`, owner-intent candidate key
  `(confirmation_id,user_id,trading_account_id,ownership_id,intent_id)`, and
  full authority/intent candidate key
  `(confirmation_id,user_id,trading_account_id,ownership_id,session_id,device_id,purpose,intent_id,intent_digest)`;
- Confirmation state checks require `ISSUED` to have no consumed/expired
  fields, `CONSUMED` to have `consumed_at` and
  `consumed_request_claim_id` only, and `EXPIRED` to have `expired_at` only;
  `confirmation_nonce` is globally unique and immutable. Confirmation exposes
  candidate unique `(confirmation_id,consumed_request_claim_id)`; the second
  component is non-null exactly in `CONSUMED`;
- `operations.confirmation_id` is unique and non-null for a
  confirmation-required Operation;
  `(confirmation_id,user_id,trading_account_id,ownership_id,intent_id)` is a
  composite FK to the Confirmation owner-intent candidate key, and Operations
  expose the covering candidate key
  `(operation_id,confirmation_id,user_id,trading_account_id,ownership_id,intent_id)`
  for the consumption FK;
- `operations.result_claim_id` remains the unique claim that created the
  existing `AWAITING_CONFIRMATION` Operation; consuming a confirmation never
  creates a second Operation;
- a unique `confirmation_consumptions` relation created only by the successful
  consume transaction stores
  `(confirmation_id,operation_id,user_id,trading_account_id,ownership_id,
  session_id,device_id,purpose,intent_id,intent_digest,request_claim_id,
  consumed_at)`, with uniqueness on both `confirmation_id` and `operation_id`;
- that consumption has a composite FK over
  `(confirmation_id,user_id,trading_account_id,ownership_id,session_id,device_id,purpose,intent_id,intent_digest)`
  to the exact full Confirmation candidate key, and a composite FK over
  `(operation_id,confirmation_id,user_id,trading_account_id,ownership_id,intent_id)`
  to the exact Operation covering candidate key;
- both `confirmation_consumptions.request_claim_id` and
  `confirmations.consumed_request_claim_id` use
  `(request_claim_id,user_id,trading_account_id,ownership_id)` composite FKs to
  the owner-scoped result claim candidate key. In addition, consumption uses
  declarative composite FK `(confirmation_id,request_claim_id)` to
  `confirmations(confirmation_id,consumed_request_claim_id)`. This—not
  application ordering or two independent FKs—forces the two claim IDs to be
  equal for the successful consume. Both claim IDs are unique; a result claim
  linked to an Operation uses the corresponding non-null owner tuple and
  composite FK;
- the same owner tuple with a different consumption request claim fails that
  `(confirmation_id,request_claim_id)` FK before link insertion, Confirmation
  or Operation CAS, cache seal, audit, or Outbox.

At one transaction Clock snapshot, P1-004 locks the owner/ownership, typed
idempotency claim, Confirmation, and its pre-linked Operation in deterministic
ID order. It requires active user/account/ownership/session/device, exact owner
tuple,
`purpose`, `intent_id`, canonical intent digest, recomputed
`confirmation_hash`, every covering-key relation, `AWAITING_CONFIRMATION`
state, and expected Operation state version. The request authority
`ownership_id`, `session_id`, and `device_id` must equal the durable
Confirmation values even if its user/account pair also matches. It then
performs exactly one Confirmation CAS
`ISSUED -> CONSUMED` and one Operation CAS
`AWAITING_CONFIRMATION -> CONFIRMED` with `state_version + 1`.

The success transaction atomically writes the two CAS results, immutable
claim-equality-protected `confirmation_consumptions` link, one owner audit, one
history row, one Outbox row on the already accepted
`fit.platform.v1.owner.owner-mutation-committed` subject with aggregate type
`OPERATION` and aggregate ID `operation_id`, AEAD nonce reservation, and the
sealed accepted Operation response. This owner event is audit/projection only:
it is not an order, executor, signer, dispatch, or trading-authorization
subject. Confirmation consumption never creates an Attempt or Order and never
publishes on an invented trading subject.

Expiry is fail-closed at equality. If the same fully authorized and matching
request locks an `ISSUED` Confirmation at `snapshot >= expires_at`, the
transaction instead CASes it to `EXPIRED`, CASes the linked Operation from
`AWAITING_CONFIRMATION` to `EXPIRED`, writes one immutable owner audit/history/
accepted owner-subject Outbox event for that Operation transition, reserves a
nonce, and seals the typed expired rejection. An already expired ticket returns
the same typed rejection without another state change or event.

Cross-owner/account/ownership/session/device, inactive context, hash, purpose,
intent, relation, or Operation mismatch fails closed before either CAS and
does not reveal which binding failed. In particular, the same user/account
with a different `ownership_id`, the same ownership with a different
`session_id`, or the same ownership/session with a different `device_id`
cannot satisfy a candidate key or composite FK. The generic rejection is
sealed under the same complete owner-scoped claim. It creates no owner audit,
history, Outbox, Attempt, Order, or other business effect.

Replay and transport loss use these exact decisions:

- same scoped key plus same first request digest returns the unexpired exact
  sealed bytes only after revalidating the complete owner/ownership/session/
  device tuple and never repeats either CAS or event;
- same scoped key plus a different digest conflicts before confirmation or
  Operation mutation;
- an already consumed Confirmation is resolved only through its immutable
  full-authority consumption and Operation links. The original claim must
  match both `consumed_request_claim_id` and the consumption's
  `request_claim_id`, as enforced by the declarative composite FK. A new
  owner-scoped reconciliation claim is not substituted into that immutable
  consumption link: it must carry the same non-null user/account/ownership
  tuple and link by composite FK to the same Operation. Only then may it return
  the current accepted Operation and seal that reconciliation result. Neither
  path may repeat the CAS, consumption link, audit, history, or Outbox writes;
- a missing/expired/unreadable exact response cache therefore falls back only
  to `DURABLE_OPERATION_RECONCILIATION`, never to re-execution. An invalid or
  missing relation returns reconciliation-required/fail-closed;
- consume is HTTP-only. A lost HTTP response is recovered by retry and then
  `GET /v1/operations/{operation_id}` after the durable relation is known.
  Missing, delayed, duplicated, or reordered WebSocket delivery has no
  authority and is reconciled against that server-authored Operation.

## 8. Quarantine, DLQ, and controlled replay

P1-004 must provide this typed transactional interface before P1-005 enables
`TERM`, a DLQ, or replay. Absence blocks those capabilities.

### 8.1 Failure and quarantine DTOs

```text
DeliveryFailureCode =
  INVALID_ENVELOPE
  | PAYLOAD_DIGEST_MISMATCH
  | SUBJECT_KIND_SCOPE_MISMATCH
  | WRONG_OWNER
  | STALE_AGGREGATE_VERSION
  | FUTURE_AGGREGATE_VERSION
  | PREDECESSOR_MISMATCH
  | SCHEMA_DRIFT
  | MESSAGE_TOO_LARGE
  | MAX_DELIVER_EXHAUSTED
  | UNKNOWN_PRECOMMIT_STATE

ValidatedEnvelopeEvidenceDTO {
  kind: VALIDATED_ENVELOPE
  event_id: EventID
  payload_digest: PayloadDigest
  subject: Subject
  event_kind: EventKind
  aggregate_type: AggregateType
  aggregate_id: AggregateID
  aggregate_version: unsigned 64-bit integer
  previous_event_id: EventID?
  canonical_envelope: CanonicalJSON
}

RejectedJSONEnvelopeEvidenceDTO {
  kind: REJECTED_JSON_ENVELOPE
  received_bytes_digest: Digest32
  received_length: unsigned 64-bit integer
  canonical_received_json: CanonicalJSON
  candidate_event_id: UUID?
  candidate_payload_digest: Digest32?
  candidate_subject: bounded UTF-8 string?
  candidate_event_kind: bounded UTF-8 string?
  candidate_aggregate_id: UUID?
  candidate_aggregate_version: unsigned 64-bit integer?
}

UndecodableEnvelopeEvidenceDTO {
  kind: UNDECODABLE_OR_OVERSIZED_ENVELOPE
  received_bytes_digest: Digest32
  received_length: unsigned 64-bit integer
  bounded_forensic_prefix: bytes with length 0..4096
}

QuarantineInputDTO {
  consumer: ConsumerRole
  envelope_evidence:
    ValidatedEnvelopeEvidenceDTO
    | RejectedJSONEnvelopeEvidenceDTO
    | UndecodableEnvelopeEvidenceDTO
  received_at: UTCInstantMs
  delivery_attempt: unsigned 32-bit integer >= 1
  failure_code: DeliveryFailureCode
  failure_evidence_digest: Digest32
}

QuarantineRecordDTO {
  quarantine_id: UUID
  input: QuarantineInputDTO
  state: QuarantineState
  isolation_audit_event_id: EventID
  isolated_at: UTCInstantMs
}
```

For a validated envelope, canonical bytes are bounded by the frozen transport/
message limit and stored only in PostgreSQL. A strict-JSON value rejected by
schema or binding retains its bounded canonical JSON and separately tagged
candidate fields; those candidates are forensic data, never authority. For
malformed or oversized input, the full body is not retained: evidence stores
its complete byte digest and length plus at most the first 4,096 bytes.
Forensic bytes are never logged or emitted as a metric.

The validated quarantine identity is unique `(consumer,event_id)` and binds the
first payload digest. A rejected JSON value with a valid candidate event ID
uses `(consumer,candidate_event_id)` and binds the candidate first digest when
present; otherwise its identity is `(consumer,received_bytes_digest)`.
Undecodable evidence also uses `(consumer,received_bytes_digest)` and grants no
event or owner identity. Same event ID/different digest is an integrity
conflict and cannot overwrite the first record. A bounded immutable observation
may record a conflicting digest/evidence digest without creating an Inbox
success or effect.

`IsolateDelivery(QuarantineInputDTO, AuditIntent)` is one P1-004 transaction:

1. validate the typed failure and server-authored target scope where applicable;
2. lock/insert the quarantine identity and first digest;
3. write one immutable non-business isolation audit;
4. create no owner business effect, no successful Inbox, and no business
   Outbox event;
5. commit and return `QuarantineRecordDTO`.

Only after that commit may P1-005 send `TERM`. If validation, persistence, or
audit fails, P1-005 sends delayed `NAK`; it never sends ACK/TERM.

### 8.2 Replay authorization DTO

```text
ReplayDecisionKind =
  AUTHORIZE_ONCE
  | DENY_PERMANENTLY
  | REQUIRE_MANUAL_RECONCILIATION

ReplayReviewReasonCode =
  TRANSIENT_DEPENDENCY_RESTORED
  | SCHEMA_COMPATIBILITY_RESTORED
  | OWNER_BINDING_RECONCILED
  | ORDERING_GAP_REPAIRED
  | FORENSIC_DIGEST_CONFIRMED
  | PERMANENT_DENIAL

ReplayBindingDTO {
  consumer: ConsumerRole
  event_id: EventID
  payload_digest: PayloadDigest
  subject: Subject
  aggregate_type: AggregateType
  aggregate_id: AggregateID
  aggregate_version: unsigned 64-bit integer
  previous_event_id: EventID?
  canonical_envelope_digest: Digest32
}

ReplayAuthorizationDTO {
  authorization_id: UUID
  quarantine_id: UUID
  decision: ReplayDecisionKind
  state: ReplayAuthorizationState
  replay_binding: ReplayBindingDTO?
  reviewed_by_principal_id: UUID
  review_reason_code: ReplayReviewReasonCode
  reconciliation_proof_id: UUID?
  issued_at: UTCInstantMs
  expires_at: UTCInstantMs?
  consumed_at: UTCInstantMs?
  consumed_by_worker_id: UUID?
  terminal_at: UTCInstantMs?
}
```

Only a separately accepted internal P1-004 review capability may create this
fact. An HTTP client, API service, NATS principal, publisher, ordinary consumer,
message body, header, or subject cannot create or modify it.

`replay_binding` is required exactly when `decision=AUTHORIZE_ONCE` and
forbidden for the other decisions. It is constructed only after the complete
stored bytes pass the current accepted strict EventEnvelope/schema/subject
validator. Candidate fields from rejected JSON are never copied without that
revalidation.

The decision/state/presence matrix is a database `CHECK`, not application
convention:

| Decision | Permitted state | Binding/expiry | Consumption/terminal fields | Consumable |
| --- | --- | --- | --- | --- |
| `AUTHORIZE_ONCE` | `ACTIVE` | complete binding required; `expires_at=issued_at+900000ms` | consumed/terminal fields absent | yes, only while snapshot `< expires_at` |
| `AUTHORIZE_ONCE` | `CONSUMED` | same immutable binding/expiry | `consumed_at`, `consumed_by_worker_id`, and equal `terminal_at` required | no |
| `AUTHORIZE_ONCE` | `EXPIRED` or `REVOKED` | same immutable binding/expiry | consumed fields absent; `terminal_at` required | no |
| `DENY_PERMANENTLY` | `DENIED_FINAL` only | binding and expiry forbidden | consumed fields absent; `terminal_at=issued_at` | no |
| `REQUIRE_MANUAL_RECONCILIATION` | `MANUAL_REVIEW_FINAL` only | binding and expiry forbidden | consumed fields absent; `terminal_at=issued_at` | no |

No other decision/state pair is valid. In particular,
`DENY_PERMANENTLY+ACTIVE` and
`REQUIRE_MANUAL_RECONCILIATION+ACTIVE` fail the row `CHECK`; neither terminal
decision can ever be consumed or updated into `AUTHORIZE_ONCE`.

`RecordReplayDecision` is one P1-004 transaction. It locks the quarantine row,
revalidates its current state and first digest, writes the typed decision plus
one immutable control-plane review audit, and commits without an owner business
effect or business Outbox event. `AUTHORIZE_ONCE` is created only in `ACTIVE`;
`DENY_PERMANENTLY` is created only in `DENIED_FINAL` and moves the quarantine
to `PERMANENTLY_DENIED`; `REQUIRE_MANUAL_RECONCILIATION` is created only in
`MANUAL_REVIEW_FINAL`, leaves the quarantine non-consumable, and creates no
active capability.

The unique consumable slot is the partial unique index
`UNIQUE(quarantine_id) WHERE decision='AUTHORIZE_ONCE' AND state='ACTIVE'`.
The worker query and its `SELECT ... FOR UPDATE` predicate must contain exactly
`decision='AUTHORIZE_ONCE' AND state='ACTIVE'`, complete non-null binding,
`consumed_at IS NULL`, `consumed_by_worker_id IS NULL`, and
`snapshot < expires_at`. A row outside that predicate is not a replay
capability. `AUTHORIZE_ONCE` expires exactly 900,000 ms after its transaction
snapshot; equality atomically transitions `ACTIVE -> EXPIRED` with no Inbox or
effect.

`WRONG_OWNER` and `PAYLOAD_DIGEST_MISMATCH` are never automatically
authorizable. Any explicit reviewed decision for them requires a non-null
`reconciliation_proof_id`, remains bound to the original stable event/digest,
and still re-runs all normal checks. It cannot reassign an owner or bless a
changed digest.

`ValidatedEnvelopeEvidenceDTO` may receive `AUTHORIZE_ONCE`.
`RejectedJSONEnvelopeEvidenceDTO` may receive it only after a separately
accepted schema-compatibility change makes the complete stored canonical bytes
strictly valid and produces an exact `ReplayBindingDTO`. Undecodable, truncated,
or oversized evidence can only be denied or held for manual reconciliation; it
is never guessed into an EventEnvelope.

### 8.3 Controlled replay transaction

A controlled replay worker does not publish. It passes the exact quarantined
envelope to the normal consumer application path with
`DeliveryMode=CONTROLLED_REPLAY` and `authorization_id`.

In one P1-004 transaction the normal path:

1. selects and locks only
   `decision=AUTHORIZE_ONCE AND state=ACTIVE`, requires the complete binding,
   no consumption fields, and `snapshot < expires_at`;
2. verifies exact quarantine/consumer/event/digest/subject/aggregate binding;
3. recomputes strict schema, subject-kind-scope, digest, and owner checks;
4. locks/inserts the normal consumer Inbox record;
5. applies normal aggregate version/predecessor CAS;
6. writes the permitted effect and immutable audit/output Outbox if required;
7. marks Inbox `APPLIED`;
8. CASes authorization `ACTIVE -> CONSUMED`, sets `consumed_at`,
   `consumed_by_worker_id`, and equal `terminal_at`, then marks quarantine
   `REPLAY_APPLIED`;
9. commits before any delivery success is reported.

Crash/rollback consumes no authorization and applies no effect. A redelivered
already-applied event verifies the exact Inbox digest and returns the saved
result without a second effect. A replay cannot skip Inbox, scope, digest,
aggregate CAS, or audit checks.

The following fail before Inbox/effect: `DENY_PERMANENTLY+ACTIVE`,
`REQUIRE_MANUAL_RECONCILIATION+ACTIVE`, `AUTHORIZE_ONCE` with missing or partial
binding, `AUTHORIZE_ONCE+ACTIVE` at/after expiry, and an already `CONSUMED`
authorization. None may be repaired by a worker-side default, looser `SELECT`,
or a partial index that omits `decision=AUTHORIZE_ONCE`.

There is no generic DLQ business subject in the accepted manifest. If a later
review chooses broker transport for quarantine, it needs a new versioned
non-business subject/stream and least-privilege principal in a separate
contract task.

## 9. Clock and transport limits

### 9.1 Clock

```text
ClockSnapshotDTO {
  utc_now_ms: UTCInstantMs
  monotonic_tick: MonotonicTick
  watermark_version: unsigned 64-bit integer
}

Clock {
  TakeTransactionSnapshot() -> ClockSnapshotDTO | CLOCK_UNAVAILABLE
}
```

Rules:

- exactly one snapshot is taken at transaction entry;
- all creation, occurrence, issue, expiry, cache, lease, audit, notification,
  aggregate, and recovery times in that transaction derive from it;
- PostgreSQL stores and locks a durable clock watermark;
- `utc_now_ms < last_committed_utc_ms` returns `CLOCK_ROLLBACK` and rolls back;
- equal UTC milliseconds are ordered by `monotonic_tick` and transaction
  serialization order;
- `snapshot >= expires_at` is expired;
- a database `now()`, framework clock, environment override, or caller time
  cannot make a security decision;
- a serialization retry takes a new snapshot but retains the same stable
  request/event identities and re-evaluates expiry; it never extends a frozen
  family deadline or resurrects an expired claim.

### 9.2 Unique typed transport source

The unique development source is:

```text
TransportLimitsDTO {
  schema_version: "fit.transport-limits.dev.v1"

  max_request_target_bytes: 8192
  max_header_bytes: 16384
  max_unauthenticated_body_bytes: 16384
  max_authenticated_body_bytes: 65536
  max_credential_workflow_body_bytes: 32768
  max_response_body_bytes: 65536

  max_websocket_data_frame_bytes: 65536
  max_websocket_application_control_bytes: 4096
  max_websocket_protocol_control_bytes: 125

  http_header_read_deadline_ms: 5000
  http_read_deadline_ms: 10000
  http_write_deadline_ms: 15000
  http_idle_deadline_ms: 60000

  websocket_read_deadline_ms: 75000
  websocket_write_deadline_ms: 10000
  websocket_ping_interval_ms: 30000
  websocket_reauth_issue_before_expiry_ms: 60000
  websocket_reauth_max_response_ms: 60000

  max_http_connections_global: 256
  max_websocket_connections_global: 128
  max_inflight_requests_global: 256
  max_handler_goroutines_per_connection: 2
  max_admission_queue_depth: 128

  shutdown_grace_ms: 10000
  forced_close_at_ms_from_shutdown: 10000
  cleanup_deadline_ms_from_shutdown: 15000
}
```

Boundary behavior is exact: a value equal to a byte/count maximum is accepted;
one byte/item above is rejected before handler/control dispatch. A deadline is
expired at equality. WebSocket reauthorization deadline remains
`min(issued_at + 60,000 ms, current_access_expiry)`.

At shutdown, new work is rejected immediately, existing work drains for 10,000
ms, remaining connections are force-closed at that boundary, and cleanup must
finish by 15,000 ms from shutdown start.

No environment variable, command flag, framework default, package constant, or
test-only override can change these values. Tests may construct only the exact
versioned DTO. A missing/unknown version or altered value fails startup.

The accepted per-source/per-session rate and connection limits remain loaded
from `http-rate-limits-v1.json`; they combine with these global limits using
deny-if-any-denies.

## 10. Eleven request workflow atomic write sets

WF-01 through WF-10 are exactly the ten persisted request workflows in
`transaction-boundaries-v1.json`. WF-11 closes the accepted confirmation
consume API/schema/state-machine handoff that the older transaction manifest
does not enumerate. Synthetic enrollment response, NATS consumer effect,
Outbox publish, and WAL incident are not counted because they are not request
workflow atomic result-cache sets.

All eleven use one PostgreSQL transaction. The cache row is the last durable
write and commit precedes response.

### WF-01 `successful_login`

- authority: resolved active owner;
- stable key: `IDEMPOTENCY_KEY`;
- locks/checks: source then user throttle; active ownership; claim; aggregate;
- atomic writes in order: source/user throttle state, pre-auth invalidation,
  new session, new refresh family with fixed deadline, initial refresh-token
  digest, immutable login audit, one event-history row, one Outbox row, nonce
  ledger reservation, sealed credential response;
- event plan: one `LOGIN_SUCCEEDED`;
- replay: exact credentials only from unexpired AEAD cache.

### WF-02 `real_enrollment_challenge_creation`

- authority: resolved active user/account owner;
- stable key: `IDEMPOTENCY_KEY`;
- locks/checks: shared password throttles, active ownership, claim, aggregate;
- atomic writes: throttle state, real single-use challenge with opaque-handle
  digest/candidate fingerprint, immutable challenge audit, one history row, one
  Outbox row, nonce reservation, sealed exact Challenge response;
- event plan: one `ENROLLMENT_CHALLENGE_ISSUED`;
- synthetic Challenge path writes nothing and is not this workflow.

### WF-03 `failed_enrollment_proof`

- authority: server-resolved real Challenge owner scope;
- stable key: `IDEMPOTENCY_KEY`;
- locks/checks: claim; real Challenge; proof binding; aggregate;
- atomic writes: consume the real Challenge's one attempt, immutable rejected
  proof audit, one history row, one Outbox row, nonce reservation, sealed exact
  generic rejection;
- prohibited: device, session, family, credential, or second proof attempt;
- event plan: one `ENROLLMENT_PROOF_REJECTED`.

### WF-04 `password_failure`

- authority: `AUTH_SECURITY_SOURCE_ONLY` for unknown/source-only, or
  `AUTH_SECURITY_RESOLVED_USER` after unambiguous server resolution;
- stable key: `REQUEST_ID`;
- locks/checks: source then optional user throttle, claim, control-plane
  aggregate only if a new threshold lock occurs;
- atomic writes: source failure, optional resolved-user failure, next-allowed or
  lock state, `2*L` ordered audit/notification history+Outbox events for newly
  locked dimensions, nonce reservation, sealed generic rejection;
- source-only path performs no owner lookup and stores no user/account;
- event plan: `2*L`, source dimension before user dimension.

### WF-05 `ordinary_device_enrollment`

- authority: real Challenge owner plus active owner recheck;
- stable key: `IDEMPOTENCY_KEY`;
- locks/checks: claim, Challenge, active ownership, global key uniqueness,
  aggregate;
- atomic writes: consume Challenge, create unique server-owned device, preserve
  all prior devices/sessions/families, create independent session/family/token,
  immutable audit, internal notification, two history rows, two Outbox rows,
  nonce reservation, sealed exact credential response;
- event plan: `DEVICE_ENROLLED` audit then notification.

### WF-06 `replacement_device_enrollment`

- authority: real Challenge owner plus active owner recheck;
- stable key: `IDEMPOTENCY_KEY`;
- locks/checks: claim, Challenge, ownership, prior device/session/family rows in
  deterministic ID order, key uniqueness, aggregate;
- atomic writes: consume Challenge, revoke every prior device/session/family,
  create unique replacement device, new independent session/family/token,
  immutable audit, internal notification, two history rows, two Outbox rows,
  nonce reservation, sealed exact credential response;
- revocations precede new session issuance;
- event plan: `DEVICE_REPLACED` audit then notification.

### WF-07 `refresh_rotation`

- authority: server-resolved refresh family owner scope;
- stable key: `IDEMPOTENCY_KEY`;
- locks/checks: claim then family; evaluate family deadline, rotated reuse,
  individual expiry, active rotation in that exact order;
- atomic writes: rotate active digest or revoke whole family/descendants; on
  reuse write immutable audit, notification, two history rows and two Outbox
  rows; nonce reservation; sealed exact rotation/reuse result;
- active rotation event plan: zero security events;
- reuse event plan: `REFRESH_REUSE_DETECTED` audit then notification;
- same key replay never becomes reuse classification; a different key against
  an already rotated digest follows the accepted family-revocation rule.

### WF-08 `device_revocation`

- authority: active owner and server-loaded target device owner recheck;
- stable key: `IDEMPOTENCY_KEY`;
- locks/checks: target, owner, claim, dependent sessions/families, aggregate;
- atomic writes: device revocation, dependent session/family revocations,
  immutable audit, notification, two history rows, two Outbox rows, nonce
  reservation, sealed exact non-credential result;
- event plan: `DEVICE_REVOKED` audit then notification.

### WF-09 `session_revocation`

- authority: active owner and server-loaded target session owner recheck;
- stable key: `IDEMPOTENCY_KEY`;
- locks/checks: target, owner, claim, refresh family, aggregate;
- atomic writes: session/family revocation, immutable audit, notification, two
  history rows, two Outbox rows, nonce reservation, sealed exact
  non-credential result;
- event plan: `SESSION_REVOKED` audit then notification.

### WF-10 `owner_mutation`

- authority: active owner and typed allowlisted target owner recheck;
- stable key: `IDEMPOTENCY_KEY`;
- locks/checks: owner/target, claim, business aggregate;
- atomic writes: allowlisted business effect, immutable audit, one history row,
  one Outbox row, nonce reservation, sealed exact non-credential result;
- event plan: one `OWNER_MUTATION_COMMITTED`;
- arbitrary operation names, trading action invention, account/object authority
  fields, or a missing typed allowlist entry fail before the effect.

### WF-11 `confirmation_consume`

- authority: active user/account/ownership/session/device from the
  server-authored request context; every component is non-null and the path ID
  and body hash grant no authority;
- stable key: `IDEMPOTENCY_KEY`; immutable first digest covers the accepted
  route, path `confirmation_id`, and body `confirmation_hash`;
- locks/checks: owner/ownership, claim, durable Confirmation, and pre-linked
  `AWAITING_CONFIRMATION` Operation; exact user/account/ownership/session/
  device/purpose/intent/digest/hash and all composite-FK bindings at one Clock
  snapshot;
- consumed atomic writes: Confirmation `ISSUED -> CONSUMED` CAS,
  full-authority `confirmation_consumptions` unique relation whose
  `(confirmation_id,request_claim_id)` FK equals the Confirmation consumed
  claim, Operation
  `AWAITING_CONFIRMATION -> CONFIRMED` expected-version CAS, immutable owner
  audit, one history row, one Outbox row on the accepted owner-mutation subject
  with aggregate type `OPERATION`, nonce reservation, sealed accepted Operation
  response;
- expired atomic writes: at equality or later, Confirmation
  `ISSUED -> EXPIRED` CAS and linked Operation
  `AWAITING_CONFIRMATION -> EXPIRED` CAS, the same single owner audit/history/
  accepted owner-subject Outbox event shape, nonce reservation, and sealed
  typed rejection;
- mismatch/cross-owner/inactive paths, including same user/account with a
  different ownership, session, or device: composite-key/FK failure plus
  generic sealed fail-closed rejection, zero Confirmation/Operation CAS and
  zero audit/history/Outbox;
- consumed replay or HTTP/WS loss: exact cache replay when available, otherwise
  complete-authority-checked `DURABLE_OPERATION_RECONCILIATION` through the
  immutable consumption, Operation, and result-claim relations; never a second
  CAS/event and never an Attempt, Order, signer, dispatch, or exchange write.

`logout` is described by P1-003 but is absent from the accepted ten-workflow
transaction manifest. It remains an implementation-blocking contract gap and
must not be silently treated as session revocation.

## 11. Exact database, API, and JetStream mapping

### 11.1 Request, claim, and authority fields

| P1-003/API application field | PostgreSQL field | JetStream |
| --- | --- | --- |
| `RequestEnvelope.request_id` | `idempotency_records.original_request_id` | never published |
| `canonical_request_digest` | `idempotency_records.first_request_digest` | never published |
| owner `user_id` | `idempotency_records.user_id`; owner-row composite FKs | EventEnvelope scope only for accepted `OWNER` event |
| owner `trading_account_id` | `idempotency_records.trading_account_id`; owner-row composite FKs | EventEnvelope scope only for accepted `OWNER` event |
| owner `ownership_id` | non-null `idempotency_records.ownership_id`; owner claim/target composite FKs | never omitted or published as client authority |
| `source_key.key_id` | FK to `source_key_records.key_id` | accepted `AUTH_SECURITY` scope |
| `source_key.source_key_digest` | `source_key_records.source_key_digest` | accepted `AUTH_SECURITY` scope; never raw IP/key |
| `control_plane_aggregate_id` | `idempotency_records.control_plane_aggregate_id` | EventEnvelope `aggregate_id` |
| `route_template` | `idempotency_records.route_template` | never published |
| stable-key kind/digest | `idempotency_records.stable_key_kind/stable_key_digest` | never published |
| `result_class` | `idempotency_records.result_class` | never published |
| exact plaintext response | never stored outside seal scope | never published |
| sealed response fields | `commit_response_cache` exact columns | never published |
| `(key_id,nonce)` reservation | `aead_nonce_ledger` | never published |
| path `confirmation_id` | lookup only; exact `confirmations.confirmation_id` | never published as authority |
| body `confirmation_hash` | compared with recomputed durable ticket hash | never published |
| server owner/ownership/session/device | non-null full-authority Confirmation, consumption, Operation, and claim composite FKs | accepted owner audit scope only |
| ticket purpose/intent/hash/expiry | immutable `confirmations` columns | never published as trading authority |
| consumed confirmation/Operation link | unique `confirmation_consumptions` relation | owner audit aggregate identity only |

The PostgreSQL `idempotency_records` row has a check constraint implementing
the exact tagged authority presence table in section 4. It has separate unique
indexes for owner and control-plane variants; no uniqueness relies on nullable
owner columns.

### 11.2 Event materialization field parity

| P1-003 `EventDraftDTO` | PostgreSQL history / Outbox | JetStream |
| --- | --- | --- |
| schema version | `aggregate_event_history.schema_version`, `outbox.event_schema_version` | body `schema_version` |
| `subject` | immutable `subject` in both rows | exact NATS publish subject and body `subject`; must be equal |
| stream | immutable `FIT_PLATFORM_V1` | body `stream=FIT_PLATFORM_V1` |
| `event_id` | history PK; Outbox unique FK | body `event_id`; stable on redelivery |
| `event_kind` | immutable history/Outbox kind | body and payload `event_kind`; exact binding |
| `scope` | typed columns plus canonical JSON | body and payload `scope`; broker grants no authority |
| `aggregate_type` | immutable column | body/payload identity |
| `aggregate_id` | immutable column | body/payload identity |
| `aggregate_version` | unique per aggregate | body/payload identity |
| `previous_event_id` | immutable nullable FK/check | body/payload identity; absent only at v1 |
| `causation_id` | immutable column | body |
| `correlation_id` | immutable column | body |
| `occurred_at` | transaction snapshot | body |
| payload schema version | immutable column | body `payload_schema_version` and inner version |
| canonical payload | validated immutable `jsonb` plus exact JCS bytes/digest | body `payload` |
| `payload_digest` | exact 32 bytes | body; consumer recomputes before mutation |
| audit/notification IDs | FKs from history/Outbox linkage | present only in accepted payload data shape |

Publisher input is the P1-004 Outbox row, not an API command. Publisher sends
the exact accepted envelope without remapping. Consumer requires equality of
outer and inner event ID, kind, subject, scope, aggregate identity/version, and
predecessor before an Inbox insert.

### 11.3 Exact accepted subject binding

| JetStream subject | Event kind | Scope | Consumer role | Aggregate type |
| --- | --- | --- | --- | --- |
| `fit.platform.v1.auth-security.device-enrolled` | `DEVICE_ENROLLED` | `AUTH_SECURITY` | `platform-consumer` | `DEVICE` |
| `fit.platform.v1.auth-security.device-replaced` | `DEVICE_REPLACED` | `AUTH_SECURITY` | `platform-consumer` | `DEVICE` |
| `fit.platform.v1.auth-security.device-revoked` | `DEVICE_REVOKED` | `AUTH_SECURITY` | `platform-consumer` | `DEVICE` |
| `fit.platform.v1.auth-security.enrollment-challenge-issued` | `ENROLLMENT_CHALLENGE_ISSUED` | `AUTH_SECURITY` | `platform-consumer` | `ENROLLMENT_CHALLENGE` |
| `fit.platform.v1.auth-security.enrollment-proof-rejected` | `ENROLLMENT_PROOF_REJECTED` | `AUTH_SECURITY` | `platform-consumer` | `ENROLLMENT_CHALLENGE` |
| `fit.platform.v1.auth-security.login-account-locked` | `LOGIN_ACCOUNT_LOCKED` | `AUTH_SECURITY` | `platform-consumer` | `AUTH_THROTTLE` |
| `fit.platform.v1.auth-security.login-source-locked` | `LOGIN_SOURCE_LOCKED` | `AUTH_SECURITY` | `platform-consumer` | `AUTH_THROTTLE` |
| `fit.platform.v1.auth-security.login-succeeded` | `LOGIN_SUCCEEDED` | `AUTH_SECURITY` | `platform-consumer` | `SESSION` |
| `fit.platform.v1.auth-security.refresh-reuse-detected` | `REFRESH_REUSE_DETECTED` | `AUTH_SECURITY` | `platform-consumer` | `REFRESH_FAMILY` |
| `fit.platform.v1.auth-security.session-revoked` | `SESSION_REVOKED` | `AUTH_SECURITY` | `platform-consumer` | `SESSION` |
| `fit.platform.v1.notification.device-enrolled` | `DEVICE_ENROLLED` | `AUTH_SECURITY` | `internal-notification-consumer` | `DEVICE` |
| `fit.platform.v1.notification.device-replaced` | `DEVICE_REPLACED` | `AUTH_SECURITY` | `internal-notification-consumer` | `DEVICE` |
| `fit.platform.v1.notification.device-revoked` | `DEVICE_REVOKED` | `AUTH_SECURITY` | `internal-notification-consumer` | `DEVICE` |
| `fit.platform.v1.notification.login-account-locked` | `LOGIN_ACCOUNT_LOCKED` | `AUTH_SECURITY` | `internal-notification-consumer` | `AUTH_THROTTLE` |
| `fit.platform.v1.notification.login-source-locked` | `LOGIN_SOURCE_LOCKED` | `AUTH_SECURITY` | `internal-notification-consumer` | `AUTH_THROTTLE` |
| `fit.platform.v1.notification.refresh-reuse-detected` | `REFRESH_REUSE_DETECTED` | `AUTH_SECURITY` | `internal-notification-consumer` | `REFRESH_FAMILY` |
| `fit.platform.v1.notification.session-revoked` | `SESSION_REVOKED` | `AUTH_SECURITY` | `internal-notification-consumer` | `SESSION` |
| `fit.platform.v1.notification.wal-archive-interrupted` | `WAL_ARCHIVE_INTERRUPTED` | `SYSTEM` | `internal-notification-consumer` | `WAL_INCIDENT` |
| `fit.platform.v1.owner.owner-mutation-committed` | `OWNER_MUTATION_COMMITTED` | `OWNER` | `platform-consumer` | `OWNER_OPERATION` for WF-10; `OPERATION` for WF-11 |
| `fit.platform.v1.system.wal-archive-interrupted` | `WAL_ARCHIVE_INTERRUPTED` | `SYSTEM` | `platform-consumer` | `WAL_INCIDENT` |

No other application subject is accepted. Application subject wildcards are
forbidden. P1-004 rejects a subject/kind/scope/aggregate-type combination that
does not match this row before materialization or Inbox mutation. The owner row
has exactly two typed workflow bindings: WF-10 uses `OWNER_OPERATION`, and
WF-11 uses `OPERATION`. It is not a generic aggregate-type wildcard.

### 11.4 Operation/Attempt/Order mapping

| Accepted field | PostgreSQL | Public API / typed consumer | JetStream |
| --- | --- | --- | --- |
| Operation domain fields | `operations` exact domain columns | exact accepted `Operation` JSON/proto | no trading-domain subject; WF-11 emits only the accepted generic owner audit/projection event |
| Operation owner tuple | explicit columns/composite FK | injected server context; omitted from response DTO | **not mapped** |
| `result_claim_id` | unique FK to idempotency result | not exposed | **not mapped** |
| Confirmation ticket and user/account/ownership/session/device binding | immutable `confirmations` row with owner-intent and full-authority candidate keys | accepted ticket is server-loaded; consume request supplies only ID/hash | **not mapped** |
| Confirmation/Operation owner-bound relation | Operation pre-link FK includes ownership; consumption FKs cover ownership/session/device/purpose/intent/digest | accepted Operation returned/reconciled only after complete authority check | accepted owner audit aggregate identity only |
| consume/reconciliation result claim | non-null user/account/ownership composite FK to the linked Operation; successful consumption has `(confirmation_id,request_claim_id)` FK to the Confirmation consumed claim | exact replay or current Operation reconciliation only | **not mapped** |
| Confirmation consume/expiry transition | Confirmation and Operation CAS in one transaction | consume POST; Operation GET for reconciliation | one generic `OWNER_MUTATION_COMMITTED` audit/projection event; never an execution command |
| Attempt domain fields | `execution_attempts` exact domain columns | accepted `ExecutionAttempt` typed contract only; no accepted HTTP route | **not mapped; no accepted subject** |
| Attempt owner tuple | non-null user/account/ownership; covering candidate key plus composite FKs to Operation and `TradingAccountOwnership` | server context only | **not mapped** |
| Order domain fields | `orders` exact domain columns | accepted `Order` typed contract only; no accepted HTTP route | **not mapped; no accepted subject** |
| `attempt_id` on Order | required internal FK | omitted from accepted Order DTO; resolved by repository | **not mapped** |
| Order owner tuple | non-null user/account/ownership; covering candidate key plus composite FKs to Attempt, Operation, and `TradingAccountOwnership` | server context only | **not mapped** |

The absence of an Attempt, Order, or trading-domain Operation mapping is an
exact fail-closed mapping, not a TODO that P1-004/P1-005 may fill locally. The
WF-11 owner event carries only the accepted audit payload and aggregate
identity; it is not an Operation wire DTO or execution command. Publishing
trading execution facts requires a separate accepted contract/subject/ACL task
and does not belong to Phase 1 design implementation.

### 11.5 Quarantine, replay, Clock, and limits mapping

| Application/port DTO | PostgreSQL | API | JetStream |
| --- | --- | --- | --- |
| `QuarantineInputDTO` | `delivery_quarantines` plus immutable `delivery_quarantine_observations` for later conflicts | no public route; internal P1-004 port only | no message; returns typed post-commit disposition eligibility |
| isolation `AuditIntent` | immutable `audit_events` row in same transaction | not externally writable | no business event/subject |
| `ReplayAuthorizationDTO` | decision/state/presence `CHECK`; only `AUTHORIZE_ONCE+ACTIVE` enters the exact partial unique index and worker predicate; deny/manual are terminal | no public route; separately accepted internal review capability only | never published |
| controlled replay envelope | read from quarantine in replay transaction | normal consumer application port only | not republished |
| `ClockSnapshotDTO` | durable `clock_watermark` plus exact transaction timestamps | injected application capability; caller cannot set it | never published as authority |
| `TransportLimitsDTO` | no mutable configuration table | unique typed startup/input guard source | broker configuration must be separately verified; message body cannot override it |

`delivery_quarantines` stores the tagged evidence discriminator, consumer role,
identity/digest columns applicable to that discriminator, failure code,
delivery attempt, evidence digest, state, isolation audit ID, and timestamps.
Database checks enforce the same variant-presence rules as the DTO.
`replay_authorizations` stores the decision discriminator, authorize-only
binding, reviewer/reason/proof linkage, state, issue/expiry/consumption/terminal
times, and worker identity. Its database matrix forbids a decision/state or
presence mismatch and it cannot be updated into a different decision or
binding.

## 12. Consumer transaction and broker disposition

| Condition | P1-004 transaction result | P1-005 disposition |
| --- | --- | --- |
| valid new event | Inbox + owner check + CAS + effect + audit/output Outbox + `APPLIED` commit | ACK after commit |
| exact redelivery after committed Inbox | verify same digest and return recorded application | ACK after verification |
| transient dependency / unknown precommit | complete rollback; no quarantine success | delayed NAK |
| invalid/digest/scope/owner/order/schema/max-deliver | quarantine + immutable audit commit; no effect/Inbox success | TERM after commit |
| quarantine transaction failure | rollback | delayed NAK; never ACK/TERM |
| controlled replay valid | authorization + normal Inbox/effect/CAS/audit + consumed state commit | report success after commit |
| replay deny/manual/missing-binding/expired/consumed/mismatch | no Inbox/effect; only valid authorize expiry may CAS `ACTIVE -> EXPIRED` | no success; reconciliation |

An ACK never represents a failed or unknown mutation. TERM never erases the
PostgreSQL source Outbox. Neither publisher nor consumer can republish a
quarantined event.

## 13. Implementation gates and unresolved design dependencies

The capsule closes the cross-layer shapes but does not claim they exist in code.
Implementation remains gated on:

1. accepted P1-002 typed immutable API inventory exposing these DTOs/enums,
   strict constructors, canonical digest bytes, `ClockSnapshotDTO`, and the
   unique `TransportLimitsDTO`;
2. accepted P1-003 implementation using the exact authority/idempotency/cache/
   event-plan ports without a generic map or raw-string decision;
3. accepted P1-004 implementation of tagged-scope constraints, durable nonce
   ledger, eleven atomic write sets, non-null Confirmation full-authority keys,
   ownership-complete Confirmation/Operation/consumption/result-claim FKs and
   declarative consume-claim equality, ownership-complete Operation/Attempt/
   Order candidate keys and FKs, CAS and dedup, quarantine, immutable isolation
   audit, and the closed replay decision/state consumption matrix;
4. accepted P1-005 implementation consuming only P1-004 Outbox and
   quarantine/replay interfaces and the accepted subject manifest;
5. separate contract remediation for logout, because it is not one of the ten
   accepted request transaction workflows;
6. separate contract work before any trading-domain Operation/Attempt/Order
   event subject or execution delivery is introduced; the accepted owner audit
   subject does not satisfy that gate.

Every implementation needs contract/unit/integration/failure-path tests and an
independent fixed-commit review. Green evidence does not authorize merge,
deployment, production, wallet/exchange access, automatic trading, or a live
order.
