# P1-004 PostgreSQL persistence and transaction-boundary design

## 1. Status, scope, and frozen inputs

- Task: `P1-004`; design only.  No application, contract, infrastructure, or
  existing-document change is authorized by this document.
- Design baseline: `9b4b113a4032f7f0eee88297781e3b2d069b1f18`.
- Authority: PostgreSQL is the system of record.  NATS is derived delivery
  only; it must never repair, select, or reassign an authoritative fact.
- Environment: authenticated, non-public, disposable development PostgreSQL
  only.  There is no wallet, signer, exchange, production deployment, automatic
  trading, or live order path in this task.
- Normative sources are the accepted P1 platform contracts, especially
  `transaction-boundaries-v1`, `migration-policy-v1`, `recovery-policy-v1`,
  `failure-injection-v1`, and `platform-v1.schema.json`.  Where this design is
  less specific, those contracts win.

P1-004 begins only from the exact accepted P1-003 descendant.  P1-003 is a Git
predecessor, **not** a migration predecessor.  Schema revisions have their own
linear, numbered manifest.

## 2. Predecessor acceptance interfaces (no premature DTO freeze)

This task intentionally does not define Go struct names, package paths, JSON
transport shapes, or constructor signatures for unfinished predecessors.
Implementation starts only after both gates below have fixed commits and their
acceptance evidence.

| Gate | Accepted capability consumed by P1-004 | P1-004 adapter obligation | Not frozen here |
| --- | --- | --- | --- |
| P1-002 | Strict platform-contract validation; canonical RFC 8785 JCS request and durable-record digests; UUID/timestamp/enum/scope validation; contract fixture oracle. | Persist only already validated server-authored commands and revalidate boundary-critical invariants before commit.  Use P1-002 digest bytes as opaque 32-byte values; compare, never recompute with a different serializer. | Go DTO layout, generated type names, validator API, error class names. |
| P1-003 | Server-authored request/identity context; storage/transaction, clock, entropy, nonce, `AuditIntent`, and `NotificationIntent` abstractions; idempotency and response-result semantics. | Supply a PostgreSQL implementation/adapter that preserves all accepted fields and returns the predecessor's typed outcomes without granting authority to callers.  Add parity tests from accepted intents to durable records. | P1-003 interface method names, in-memory fake internals, HTTP/websocket DTOs, response envelope fields. |

If either acceptance surface cannot express a required frozen field (including
request-versus-incident linkage, complete tagged scope, result-cache outcome,
or expected aggregate version), P1-004 stops and emits the prescribed
machine-readable remediation proposal.  It must not patch a predecessor or
invent a compatibility DTO.

## 3. PostgreSQL and dependency admission

- Minimum engine: PostgreSQL **16.0**.  CI must test the supported latest 16.x
  image and reject an older server at migration startup.  PostgreSQL 17+ may be
  an additional compatibility target, not a replacement for 16.x evidence.
- Encoding/locale: UTF-8; database/session timezone `UTC`; `standard_conforming_strings=on`.
  All persisted instants use `timestamptz`; no business timestamp may use
  `timestamp without time zone`.
- Initial schema uses core PostgreSQL only: `uuid`, `bytea`, `jsonb`, btree,
  partial indexes, constraints, row-level security, and advisory locks.  No
  extension is admitted by default.  In particular, `pgcrypto`, `uuid-ossp`,
  `citext`, untrusted procedural languages, FDWs, and replication extensions
  are not prerequisites.
- UUIDs, entropy, canonical JSON, SHA-256/JCS digests, Argon2id verification,
  source-key HMAC, and token generation originate in the accepted P1-002/P1-003
  boundary.  PostgreSQL stores and compares them; it does not silently generate
  alternate identities or digests.
- Any driver, migrator, extension, superuser requirement, background worker,
  or networked dependency needs a dependency-intake record with version,
  license, checksum, CVE review, least-privilege rationale, offline test
  fixture, and removal/rollback impact.  The application must not need
  superuser, `CREATEDB`, `CREATEROLE`, `REPLICATION`, `BYPASSRLS`, extension
  creation, or DDL privileges.

## 4. Physical model and integrity rules

Names below are the implementation target, not externally stable API DTOs.
Every durable primary identifier is `uuid`; every frozen digest is `bytea` with
`octet_length(value)=32`; all UUID/digest text conversion is done at the strict
P1-002 boundary.  UUID columns have no database random default.

| Area / table | Essential facts and constraints | Required indexes / isolation rule |
| --- | --- | --- |
| `users` | Server-authored `user_id`, status, creation time; no password/token plaintext. | PK user; status lookup. |
| `trading_accounts` | Server-authored account identity/status only; P1-004 stores no exchange credential or wallet material. | PK account. |
| `trading_account_ownership` | `ownership_id`, `user_id`, `trading_account_id`, role/status/revocation.  The immutable candidate key is `(user_id,trading_account_id,ownership_id)`; all three columns are `NOT NULL`.  A revocation time exists iff status is revoked.  Sessions and every owner fact use this three-column composite FK, while each mutation locks and confirms active ownership. | PK ownership; unique `(user_id,trading_account_id,ownership_id)`; active-owner lookup. |
| `devices` | Server-generated device ID; owning user; global unique Ed25519 public-key fingerprint; active/revoked lifecycle with revocation time iff revoked. | PK device; unique fingerprint; unique `(device_id,user_id)` for composite FK; user/status lookup. |
| `sessions` | Server-generated session; exact owner tuple, device, refresh family, access expiry/status/revocation.  Composite FKs prove device belongs to user and user/account ownership exists; transaction checks active ownership/device before issue/use. | PK session; unique `(session_id,user_id,trading_account_id,device_id)`; device and owner active indexes. |
| `refresh_families` | One family belongs to exactly one owner/session; immutable creation and hard deadline (30 days); lifecycle/revocation metadata.  Session/family linkage is inserted as a deferred FK cycle or by a separate initially-null, transaction-completed binding, never as a partially committed row. | PK family; unique `(family_id,session_id)`; active family/session lookup. |
| `refresh_tokens` | Digest only, never plaintext.  Each row has family/session, issued/expires/family deadlines, state, optional successor digest and revocation time.  Enforce exact 7-day-or-family-deadline formula, same family creation/deadline, acyclic successor within family, one active token per family, and rotated/revoked field rules. | PK/unique token digest; unique `(family_id, token_digest)`; partial unique active token per family; deferrable self-FK for `(family_id, rotated_to_digest)`.  Lock family row before evaluation; evaluation order is deadline, rotated reuse, individual expiry, rotation. |
| `enrollment_challenges` | Only real, password-authorized challenges.  Store one-way opaque-subject-handle digest, candidate fingerprint, canonical public-challenge digest/validated immutable binding, resolved owner, source-key reference, request ID, purpose, expiry, status, and consumed/expired time.  Synthetic challenge responses create no row. | PK challenge state ID; unique subject-handle digest; active-expiry/purpose lookup.  `UPDATE ... WHERE status='ACTIVE' AND expires_at>clock`/row lock gives single consumption. |
| `device_action_challenges` | Bound user/account/device/session/action/payload digest plus nonce digest, issue/expiry and single-use lifecycle.  A revoked device/session/key or expiry rejects before consumption. | PK; unique active nonce digest in its binding; lock-and-conditional-consume. |
| `source_key_records`, `password_throttle_dimensions` | Persist only source key ID + HMAC digest, never raw IP/runtime HMAC key.  Account dimension is User ID; source dimension is source-key digest.  Store count/window/next-allowed/lock timestamps and lock correlation. | Unique `(dimension_type, dimension_subject)`; lock dimension rows in deterministic source-then-user order. |
| `idempotency_records`, `commit_response_cache`, `aead_nonce_ledger` | The idempotency row has an exhaustive non-null authority variant.  `OWNER` is unique on `(user_id,trading_account_id,ownership_id,route_template,stable_key_kind,stable_key_digest)`; `AUTH_SECURITY_SOURCE_ONLY`, `AUTH_SECURITY_RESOLVED_USER`, and `SYSTEM` are separately unique on `(authority_kind,control_plane_aggregate_id,route_template,stable_key_kind,stable_key_digest)`.  The tagged presence `CHECK` forbids a global nullable scope: an owner claim cannot omit `ownership_id`; control-plane rows have their required non-empty normalized aggregate scope and forbidden owner fields.  The one-to-one cache row contains only AES-256-GCM sealed ciphertext, a 96-bit nonce, authenticated key ID, timestamps, exact-response metadata, and AAD digest; no plaintext token, cookie, password, or response field.  Exact ciphertext retention is 120 seconds; expiry erases only the cache ciphertext/nonce copy while the result claim remains.  `aead_nonce_ledger` is append-only and holds the reservation independently of cache retention. | Separate partial/covering unique indexes for the owner and each control-plane authority variant; no ordinary nullable `UNIQUE` index is accepted.  Unique cache row per claim.  `aead_nonce_ledger` PK `(key_id,nonce)`, unique `claim_id`; it is never TTL-cleaned and a `key_id` is never rebound to different key material.  Claim, nonce reservation, and cache insert occur in the business transaction. |
| `operations`, `execution_attempts`, `orders` | These are authoritative owner facts, not Outbox projections. `operations.operation_id`, `execution_attempts.attempt_id`, and `orders.order_id` are separately `PRIMARY KEY` global fact identifiers; none is a scoped surrogate. Every `user_id`, `trading_account_id`, and `ownership_id` is `NOT NULL`; each durable child carries the full tuple. Operation has unique immutable creating `result_claim_id` (the request dedup link), owner-covering unique `(operation_id,user_id,trading_account_id,ownership_id)`, accepted-enum state `CHECK`, expected-version state CAS, and composite FK to ownership. Attempt has owner-covering unique `(attempt_id,operation_id,user_id,trading_account_id,ownership_id)`, accepted-enum state `CHECK`, composite FKs to the exact Operation and ownership, unique `(operation_id,ownership_id,attempt_number)`, and globally unique `client_order_id`. Order has owner-covering unique `(order_id,attempt_id,operation_id,user_id,trading_account_id,ownership_id)`, accepted-enum state `CHECK`, unique `attempt_id`, composite FKs to the exact Attempt, Operation, and ownership, globally unique/equal `client_order_id`, and partial unique `(trading_account_id,ownership_id,exchange_order_id)` when exchange ID is present. | Reusing an existing `operation_id`, `attempt_id`, or `order_id` under another owner fails its primary key before insert. Any same-user/account but different-ownership substitution fails a composite FK before insert or state transition. Event rows have globally unique `event_id` and unique aggregate version; confirmation required Operations use the unique pre-link/consume dedup constraints below. Unknown execution result is `UNKNOWN_REQUIRES_RECONCILIATION`; it never causes a blind replacement order or automatic retry. |
| `confirmations`, `confirmation_consumptions` | A Confirmation preserves the complete durable ticket and full authority tuple `user_id,trading_account_id,ownership_id,session_id,device_id,purpose,intent_id,intent_digest`, all `NOT NULL`; nonce is globally unique/immutable.  It exposes both owner-intent and full-authority candidate keys.  The one successful consumption relation is unique on confirmation and operation and records the same full authority plus `request_claim_id`.  Its composite FK to `(confirmation_id,consumed_request_claim_id)` enforces equality between the consumed Confirmation claim and consumption request claim, not merely two independent claim FKs. | `ISSUED`, `CONSUMED`, and `EXPIRED` field-presence states are database checks.  An Operation requiring confirmation has non-null unique `confirmation_id`, a pre-link composite FK to the Confirmation owner-intent key, and its creating result claim remains unique.  Full-authority, Operation-covering, owner-scoped result-claim, and `(confirmation_id,request_claim_id)` FKs must all hold before either Confirmation/Operation CAS. |
| `aggregate_heads`, `aggregate_event_history` | `aggregate_heads` is one lock row per aggregate.  History is append-only event envelope metadata/JCS payload with exact event ID, scope, aggregate ID/type/version, predecessor, causation/correlation/time/digest.  Version starts at one, is contiguous, and predecessor is absent only at version one.  A multi-event workflow locks the head once, allocates a deterministic ordered run `v+1..v+n`, inserts a separate history and Outbox row for every event, then CASes the head from `v` to `v+n`. | PK event ID; unique `(aggregate_type,aggregate_id,aggregate_version)`; unique predecessor where applicable; head locks with `SELECT ... FOR UPDATE`, then final CAS on the observed expected version/last-event ID. |
| `audit_events` | Immutable `AuditEvent` materialization with actor, kind, scope columns, occurrence, causation/correlation, schema version, request/security-operation/system-operation linkage, optional allowed device/session link, and durable-record digest.  `OWNER` requires valid user/account; `AUTH_SECURITY` requires source key and forbids account; `SYSTEM` forbids user/account.  Kind-to-scope/device/session/request matrix is checked in database and parity-tested against P1-002. | PK event; unique durable digest only if contract permits; indexes on owner/time, source/time, operation/request, causation/correlation.  No update/delete grants; immutable trigger as defense in depth. |
| `notifications` | Immutable internal Notification only: kind, exact severity, tagged scope, occurrence, message-code enum, schema-validated JSON args, causation/correlation and record digest.  No arbitrary text, email, push, destination, webhook, address, or credential columns. | PK notification; scope/time and causation indexes; kind/severity/message-code/scope checks; immutable trigger. |
| `outbox` | One persisted, validated EventEnvelope per authoritative event, pointing to aggregate history and durable audit/notification payload as applicable.  Accepted wire state is `PENDING`, `CLAIMED`, or `PUBLISHED`; only lease fields and monotonic publication state may change.  Business events are never deleted, truncated, or cascaded in Phase 1. | PK outbox; unique event ID and aggregate version; partial pending/expired-lease queue index; `(lease_expires_at, outbox_id)` claim scan.  `FOR UPDATE SKIP LOCKED` claims only expired/unclaimed rows; claim token/owner and lease must match on mark-published. |
| `inbox` | Per-consumer immutable delivery identity plus event digest, subject/kind/scope, aggregate identity/version/predecessor, received time, business transaction ID and applied status.  It records success only in the transaction that applies the effect. | Unique `(consumer,event_id)`; event-ID/digest conflict query.  Same ID/different digest is rejected and alerted without marking success or applying a business effect. |
| `wal_archive_incidents`, `recovery_runs`, `recovery_evidence` | WAL incident has one incident identity, open/cleared state and consecutive-success count; recovery records frozen workload ID/hash/seed/rate/UTC clock, failure point/time, sequences, restore markers, version/hash, gates and measured RPO/RTO evidence.  Evidence cannot assert target RPO/RTO as observations. | Unique open incident discriminator; recovery-run PK; unique terminal markers; indexes by run/gate.  P1-004 provides durable facts; P1-005 owns incident behavior and P1-006 owns archive/restore tooling. |

All scope-bearing children carry the required owner tuple or control-plane scope
columns; no child may infer an owner only from an unscoped object ID.  Composite
FKs are used wherever the parent has an owner tuple, and every mutation also
performs an explicit active-owner check under lock.  This combination prevents
cross-user/account FK substitution while allowing a historical revoked owner
fact to remain auditable.

## 5. Transactions, clock, response cache, CAS, leases, and failure behavior

Each workflow in `transaction-boundaries-v1.json` is exactly one PostgreSQL
transaction unless that manifest explicitly defines the Outbox claim and mark
transactions.  The implementation must expose an ordered statement trace so
the failure matrix can inject before the first statement, after every statement,
at commit, and after commit before response.

1. Begin at `SERIALIZABLE` for throttle, challenge consumption, refresh,
   idempotency, replacement enrollment, and aggregate append operations.
   Retry only serialization/deadlock failures with the same stable operation or
   idempotency identity; never retry an unknown result by generating new IDs.
2. Establish request scope with `SET LOCAL` only after the server-authored
   P1-003 context is validated.  Lock owner/device/session/family/challenge and
   throttle rows in the frozen workflow order; source then resolved user is the
   deterministic two-key lock order.
3. Claim idempotency in the same transaction.  A same-scope/same-digest retry
   returns the recorded outcome; a changed digest aborts with
   `IDEMPOTENCY_CONFLICT`; a cache-expired committed result returns
   `RECONCILIATION_REQUIRED_NO_REEXECUTION` and never re-executes.
4. Apply the complete mutation, immutable audit and required internal
   notification, then append every planned aggregate event/history/Outbox row
   and persist the exact encrypted response cache in that same transaction.
   Commit precedes every response, credential return, broker publication, and
   NATS acknowledgement.
5. A consumer verifies envelope scope and target ownership before Inbox insert.
   It inserts/locks Inbox, checks version/predecessor, writes the effect/audit/
   output Outbox, and marks Inbox applied in the same commit.  NATS ack is
   strictly after commit.

### 5.1 P1-003 Clock acceptance gate and one-transaction time

P1-003's task packet promises a Clock abstraction but this baseline does not
contain its accepted signature.  Before implementation, its accepted capability
must provide one immutable UTC, second-or-exact-millisecond `now` snapshot for
each persistence transaction and a monotonic ordering value suitable for the
frozen throttle semantics.  P1-004 does not name its DTO or method.

The repository takes that snapshot exactly once at transaction entry and uses
the same value for every `created_at`, `occurred_at`, `issued_at`, expiry,
lease, response-cache TTL, audit, notification, aggregate, and recovery fact in
the transaction.  It never makes security decisions from a second database
`now()` call.  Expiry is strict: `snapshot >= expires_at` is expired.  Under a
row lock on a durable clock watermark, a UTC candidate earlier than the last
committed snapshot fails closed as `CLOCK_ROLLBACK`; it is not silently clamped
or used to extend a deadline.  The monotonic component orders same-UTC throttle
attempts according to the frozen serialization rule.  If P1-003 cannot provide
this snapshot/monotonic/rollback capability, P1-004 is blocked pending a
remediation proposal rather than inventing a Clock DTO.

### 5.2 Exact response-cache atomic persistence protocol

The frozen wording must be read precisely.  `contracts/platform/README.md`
states that credential-bearing commit responses are retained for 120 seconds as
"AES-256-GCM ciphertext under a deployment-shared runtime keyring that is not
stored in PostgreSQL" and that same-scope retries on any replica recover the
exact result; `transaction-boundaries-v1.json` additionally requires
`persist_exact_*_response_aead_cache_in_same_transaction` and specifies a key
source shared across replicas/restarts but not PostgreSQL.  The prohibition is
on the **keyring**, not on ciphertext.  Therefore the bounded P1-004 protocol
persists ciphertext in `commit_response_cache` atomically with the business
transaction while keeping every AES key exclusively in the injected runtime
keyring.

This applies to every manifest workflow with `response_cache`: successful
login, real enrollment-challenge creation, failed enrollment proof, device
revocation, session revocation, owner mutation, password failure, ordinary and
replacement enrollment, and refresh rotation.  The cache binding uses the
accepted P1-003/P1-002 canonical authenticated context: user/account when the
scope has them, route template, stable idempotency/request key, canonical
request digest, authenticated key ID, `created_at_ms`, and `expires_at_ms`.
For a control-plane response without an owner, the accepted P1-003 capability
must provide the contract-valid absent-owner representation; P1-004 must not
fabricate an owner or a DTO.

| Frozen workflow | Stable cache claim | Required last database write before commit |
| --- | --- | --- |
| `successful_login` | idempotency key + request digest | Insert sealed credential response cache after session/family/refresh, audit/history/Outbox writes. |
| `real_enrollment_challenge_creation` | idempotency key + request digest | Insert sealed exact Challenge response after durable Challenge and audit/history/Outbox writes. |
| `failed_enrollment_proof` | idempotency key + request digest | Insert sealed rejection response after single Challenge consumption, audit/history/Outbox writes. |
| `device_revocation`, `session_revocation`, `owner_mutation` | idempotency key + request digest | Insert sealed result after effect and every required audit/notification/history/Outbox write. |
| `password_failure` | request ID + request digest | Insert sealed generic rejection after throttle updates and any threshold lock audit/notification/history/Outbox writes. |
| `ordinary_device_enrollment`, `replacement_device_enrollment` | idempotency key + request digest | Insert sealed credential response after the complete ordered enrollment/revocation effect and all event writes. |
| `refresh_rotation` | idempotency key + request digest | Insert sealed rotation/reuse response after family change and any reuse audit/notification/history/Outbox writes. |

No cache write is permitted for `synthetic_enrollment_challenge_response`; it
has no persistence.  The write named in the table is the final durable write in
the single transaction.  Any failure before commit rolls it and all preceding
writes back; after commit no workflow resumes its mutation path.

Before database work, P1-003 entropy obtains a fresh 96-bit nonce and the
runtime keyring selects the authenticated `key_id`; the service seals the exact
response with AES-256-GCM and RFC 8785-JCS AAD made from the complete binding
above plus the stable operation ID.  Only the sealed ciphertext/tag, nonce,
key ID, binding digest, and timestamps enter PostgreSQL.  The keyring is an
injected deployment-shared secret capability, never a table, dump, migration,
log, metric, trace, or test artifact; it retains a retired key until no
unexpired row can reference it.  Every replica/restart uses that same keyring
to decrypt an unexpired cache row only after scope, stable key, digest, and AAD
all match.  Key unavailable, malformed ciphertext, nonce collision, or AAD
mismatch fails closed to reconciliation and never causes re-execution.

The ordered persistence protocol is: (a) take the transaction time snapshot
and prepare the in-memory sealed response; (b) begin the PostgreSQL
transaction, validate scope and claim idempotency; (c) write all frozen business
effects, audit/notification, event-history/Outbox rows, and the one cache row;
(d) commit; (e) only then transmit the response.  A cache write before commit
is merely a staged database row.  Abort, process kill, constraint error, or
serialization retry before commit rolls back the idempotency claim, cache row,
and every business write together.  A crash after commit but before transmit is
recovered by the cache row on any replica/restart; it cannot execute again.

On retry, the repository locks the idempotency record before reading cache.  A
different digest returns `IDEMPOTENCY_CONFLICT` without cache decryption.  At
or after expiry, a transaction deletes ciphertext/nonce, sets a nonsecret
reconciliation marker, commits, and returns
`RECONCILIATION_REQUIRED_NO_REEXECUTION`; a sweeper performs the same atomic
erasure path.  Neither normal, failure, nor cleanup paths may write plaintext
tokens, cookies, passwords, refresh values, or response bodies to PostgreSQL or
observability.  If P1-003 cannot supply the exact response bytes, canonical
AAD binding, shared restart-surviving keyring, or clock snapshot, this protocol
is an implementation-blocking remediation gate.

### 5.2.1 Capsule-conformant authority, nonce, and cache override

The preceding cache prose is narrowed by frozen interface capsule
`f61474ba4f96624f34ff478bd0047c28e72becab`; this subsection controls where
there is any ambiguity.  `idempotency_records` has a tagged, non-null authority
scope.  `OWNER` uses the exact unique key
`(user_id,trading_account_id,ownership_id,route_template,stable_key_kind,stable_key_digest)`.
`AUTH_SECURITY_SOURCE_ONLY`, `AUTH_SECURITY_RESOLVED_USER`, and `SYSTEM` use
the exact non-null unique key
`(authority_kind,control_plane_aggregate_id,route_template,stable_key_kind,stable_key_digest)`.
The row `CHECK` enforces the capsule presence table: owner claims require the
complete owner tuple; control-plane claims require their normalized aggregate
scope and forbid owner fields.  There is no ordinary nullable `UNIQUE`, no
global `NULL = NULL` scope, and no client-selected surrogate for the control
plane aggregate.

Before sealing, the transaction reserves
`(key_id,nonce,claim_id,reserved_at)` in append-only `aead_nonce_ledger` using
`INSERT ... ON CONFLICT DO NOTHING RETURNING`.  `(key_id,nonce)` is its primary
key and `claim_id` is unique.  No encryption occurs before a reservation.  A
collision obtains another nonce and retries at most three times; exhaustion
rolls back the whole transaction as `NONCE_RESERVATION_FAILED`.  Cache cleanup
at 120 seconds deletes only ciphertext/cache-nonce copies, never the ledger.
The ledger remains through cache expiry until key destruction and a `key_id` is
never rebound to different key material.  A TTL-after-collision probe is a
mandatory negative acceptance case.

The exact canonical AAD fields are cache schema version, claim ID, complete
tagged authority scope, route template, stable-key kind/digest, original
request ID, first request digest, server-authenticated-context digest, result
class, key ID, and creation/expiry milliseconds.  Arbitrary headers, bearer
values, raw request bytes, raw client authority, passwords, and tokens are
forbidden from AAD and unencrypted metadata.  The keyring is deployment-shared
across replicas/restarts and absent from PostgreSQL, source, fixtures, logs,
metrics, traces, dumps, and evidence; retired decrypt keys remain available
until no unexpired cache row references them.  A retry revalidates applicable
authority, locks the claim, then validates scope/key/digest/AAD before decrypt.
Different digest conflicts before decrypt/effect.  Missing, expired, malformed,
AAD-mismatched, or key-unavailable cache produces
`RECONCILIATION_REQUIRED_NO_REEXECUTION`, never re-execution.

### 5.3 Aggregate multi-event append protocol

After all business rows and immutable durable records are staged, the workflow
locks its aggregate head once and reads `(v, last_event_id)`.  It builds the
already-determined event plan in the frozen statement order: event 1 receives
version `v+1` and predecessor `last_event_id` (or no predecessor only when it
is version 1); each event `i>1` receives `v+i` and the immediately prior new
event ID.  For every event it inserts one independently validated history row
and one Outbox row.  Only after all `n` inserts succeed does it CAS the head
from the observed `(v,last_event_id)` to `(v+n,event_n_id)`; a zero-row CAS
rolls back the entire transaction.  Concurrent retries reread the head, retain
the same stable operation/idempotency identity, and either recover their exact
cache result or fail stale/conflict—never append an alternate chain.

The fixed event cardinalities are: device revocation = 2 (one
`DEVICE_REVOKED` AuditEvent and one internal Notification); session revocation
= 2; replacement enrollment = 2 (one `DEVICE_REPLACED` audit and notification);
refresh reuse = 2; and one newly opened WAL incident = 2.  A password threshold
locks `L` dimensions, where `L` is 0, 1, or 2, and emits exactly `2*L` events:
one audit plus one notification per newly locked dimension in source-then-user
order.  No threshold/reuse/incident means zero such events and no aggregate
head advance.  These counts are in addition to any separately frozen workflow
event already named by its manifest; an implementation discovering a different
contract-required cardinality must stop for remediation rather than merge or
coalesce events.

Ordinary enrollment consumes one real Challenge, creates a unique device and
new session/family, and does **not** revoke prior devices/sessions/families.
Replacement enrollment consumes the real Challenge, revokes all prior
devices/sessions/families, then creates the new device/session/family in one
transaction.  Refresh locks the family, applies the frozen evaluation order,
and any rotated-token reuse revokes every descendant atomically with required
Audit/Notification/Outbox facts.

Outbox publishers use an expiring lease, not a permanent ownership claim.  A
publisher may publish a claimed event more than once after a process crash;
stable `event_id` + `payload_digest`, Inbox deduplication, and post-commit
acknowledgement prevent duplicate business effects.  A stale worker cannot mark
another worker's lease published.

Transaction abort, process kill before commit, or constraint failure leaves no
partial row, audit, notification, history, Outbox, Inbox, idempotency marker,
or response-cache ciphertext.  After commit before response, retry observes
the complete effect through the stable idempotency/operation record and the
same-transaction ciphertext cache.

### 5.4 Frozen capsule addendum: Confirmation, workflows, replay, and limits

This addendum is normative and aligns this design with frozen interface capsule `f61474ba4f96624f34ff478bd0047c28e72becab`.  Where earlier prose is less specific, this addendum controls.

#### 5.4.1 Confirmation full authority and claim equality

`POST /v1/confirmations/{confirmation_id}/consume` path and body are assertions, not authority. One transaction locks active user/account/ownership/session/device, the owner-scoped result claim, durable Confirmation, and pre-linked `AWAITING_CONFIRMATION` Operation in deterministic ID order. It requires exact equality of user, account, `ownership_id`, session, device, purpose, intent ID, intent digest, recomputed confirmation hash, all covering FKs, and expected Operation state version at the one Clock snapshot.

Every related authority column is `NOT NULL`. `confirmations` has unique owner-intent `(confirmation_id,user_id,trading_account_id,ownership_id,intent_id)` and unique full authority `(confirmation_id,user_id,trading_account_id,ownership_id,session_id,device_id,purpose,intent_id,intent_digest)`. A confirmation-required Operation has unique non-null `confirmation_id`, pre-link FK `(confirmation_id,user_id,trading_account_id,ownership_id,intent_id)`, and covering unique `(operation_id,confirmation_id,user_id,trading_account_id,ownership_id,intent_id)`. `confirmation_consumptions` has unique confirmation and Operation, full-authority/Operation covering FKs, owner-scoped result-claim FK `(request_claim_id,user_id,trading_account_id,ownership_id)`, and declarative claim-equality FK `(confirmation_id,request_claim_id)` to `confirmations(confirmation_id,consumed_request_claim_id)`. Two independent claim FKs are insufficient. Confirmation state checks require `ISSUED` to have neither terminal field, `CONSUMED` to have only `consumed_at` plus non-null consumed claim, and `EXPIRED` to have only `expired_at`; `confirmation_nonce` is globally unique and immutable.

On a valid unexpired request, exactly one `ISSUED -> CONSUMED` Confirmation CAS and one `AWAITING_CONFIRMATION -> CONFIRMED` expected-version Operation CAS occur. The same commit writes the claim-equality consumption link, one owner audit, one history/Outbox pair on `fit.platform.v1.owner.owner-mutation-committed` with aggregate type `OPERATION` and ID `operation_id`, nonce reservation, and sealed Operation response. At equality/after expiry, the valid matching path CASes both to `EXPIRED` and emits that same single event shape. Cross-owner, inactive, hash, intent, or same user/account but different ownership/session/device mismatch writes a sealed generic rejection with zero CAS, audit, history, or Outbox. Exact replay revalidates the complete authority; cache loss can only return `DURABLE_OPERATION_RECONCILIATION` through immutable consumption/Operation/claim links, never a second CAS, event, Attempt, Order, signing, dispatch, or exchange write.

#### 5.4.2 Eleven atomic request write sets

Every WF below is exactly one PostgreSQL transaction at one Clock snapshot. The listed lock/check order is mandatory. For a nonzero event plan, lock the one aggregate head, insert exactly one history and one Outbox row for each ordered `v+1..v+n` event with exact predecessor, then perform one final head CAS from `v` to `v+n`; no zero-event path advances a head. For every workflow the nonce ledger reservation precedes sealing, `commit_response_cache` is the **last durable write**, and commit precedes response. Same scoped stable key/digest revalidates authority then replays only the unexpired exact AEAD response; different digest is `IDEMPOTENCY_CONFLICT` before decrypt/effect; missing/expired/malformed/AAD-mismatch/key-unavailable cache is `RECONCILIATION_REQUIRED_NO_REEXECUTION`, except WF-11's more-specific durable Operation reconciliation. The ordered write lists below are the EV-041 cut inventory: every item and every final commit is a mandatory fault cut.

##### WF-01 `successful_login`

- Authority and stable key: resolved active `OWNER`; `IDEMPOTENCY_KEY`.
- Lock/check order: source throttle, resolved-user throttle, active ownership, result claim, `SESSION` aggregate head.
- Atomic writes in order: source/user throttle state; pre-auth invalidation; new session; new refresh family with fixed deadline; initial refresh-token digest; immutable login audit; history `LOGIN_SUCCEEDED` at `v+1`; matching Outbox; nonce reservation; sealed exact credential cache last.
- Event plan: one audit event, so one history/Outbox pair and one CAS `v -> v+1`.
- Replay/reconcile: exact credentials only from unexpired cache; no cache never creates another session/family/token or event.

##### WF-02 `real_enrollment_challenge_creation`

- Authority and stable key: resolved active owner; `IDEMPOTENCY_KEY`.
- Lock/check order: shared password throttles, active ownership, result claim, `ENROLLMENT_CHALLENGE` aggregate head.
- Atomic writes in order: throttle state; real single-use Challenge with opaque handle digest/candidate fingerprint; immutable challenge audit; history `ENROLLMENT_CHALLENGE_ISSUED` at `v+1`; matching Outbox; nonce reservation; sealed exact Challenge cache last.
- Event plan: one audit event, one history/Outbox pair and one CAS `v -> v+1`. A synthetic Challenge path writes nothing and is not WF-02.
- Replay/reconcile: exact Challenge only from unexpired cache; no cache never creates or consumes another Challenge.

##### WF-03 `failed_enrollment_proof`

- Authority and stable key: server-resolved real Challenge owner; `IDEMPOTENCY_KEY`.
- Lock/check order: result claim, real Challenge, proof binding, `ENROLLMENT_CHALLENGE` aggregate head.
- Atomic writes in order: consume the real Challenge's one attempt; immutable rejected-proof audit; history `ENROLLMENT_PROOF_REJECTED` at `v+1`; matching Outbox; nonce reservation; sealed exact generic rejection cache last.
- Event plan: one audit event, one history/Outbox pair and one CAS `v -> v+1`; device, session, family, credential, and second proof attempt are prohibited.
- Replay/reconcile: exact rejection only from cache; no cache never consumes another proof attempt.

##### WF-04 `password_failure`

- Authority and stable key: `AUTH_SECURITY_SOURCE_ONLY` for unknown/source-only input or `AUTH_SECURITY_RESOLVED_USER` only after unambiguous server resolution; `REQUEST_ID`.
- Lock/check order: source throttle, optional resolved-user throttle, result claim, control-plane `AUTH_THROTTLE` aggregate head only when a new threshold lock occurs.
- Atomic writes in order: source failure; optional resolved-user failure; next-allowed/lock state; for every newly locked dimension, immutable audit then notification/history/Outbox in source-before-user order; nonce reservation; sealed generic rejection cache last.
- Event plan: exactly `2*L`, `L in {0,1,2}`, with each pair audit then notification; therefore `v+1..v+2L` and one final CAS only when `L>0`. Source-only writes no owner row and performs no owner lookup.
- Replay/reconcile: same request ID/digest never increments a throttle again; cache loss reconciles without repeating the failure.

##### WF-05 `ordinary_device_enrollment`

- Authority and stable key: real Challenge owner with active-owner recheck; `IDEMPOTENCY_KEY`.
- Lock/check order: result claim, Challenge, active ownership, global device-key uniqueness, `DEVICE` aggregate head.
- Atomic writes in order: consume Challenge; create unique server-owned device; retain every prior device/session/family; create independent new session, refresh family, and token; immutable audit; internal notification; audit history/Outbox `DEVICE_ENROLLED` at `v+1`; notification history/Outbox at `v+2`; nonce reservation; sealed exact credential cache last.
- Event plan: two ordered events, audit then notification, with predecessors and one CAS `v -> v+2`.
- Replay/reconcile: exact credential response only from cache; no cache never creates a second device/session/family/token.

##### WF-06 `replacement_device_enrollment`

- Authority and stable key: real Challenge owner with active-owner recheck; `IDEMPOTENCY_KEY`.
- Lock/check order: result claim, Challenge, ownership, all prior device/session/family rows in deterministic ID order, global device-key uniqueness, `DEVICE` aggregate head.
- Atomic writes in order: consume Challenge; revoke every prior device/session/family; create unique replacement device; create new independent session/family/token; immutable audit; internal notification; audit history/Outbox `DEVICE_REPLACED` at `v+1`; notification history/Outbox at `v+2`; nonce reservation; sealed exact credential cache last.
- Event plan: two ordered events, audit then notification, predecessors exact, one CAS `v -> v+2`; revocations precede new session issuance.
- Replay/reconcile: exact credential response only from cache; no cache never repeats revocation or replacement.

##### WF-07 `refresh_rotation`

- Authority and stable key: server-resolved refresh-family owner; `IDEMPOTENCY_KEY`.
- Lock/check order: result claim, refresh family, then evaluate family deadline, rotated-token reuse, individual expiry, and active rotation in exactly that order; lock `REFRESH_FAMILY` aggregate only for reuse events.
- Atomic writes in order: rotate active digest **or** revoke family and descendants on reuse; on reuse write immutable audit then notification, history/Outbox `REFRESH_REUSE_DETECTED` at `v+1/v+2`; nonce reservation; sealed exact rotation/reuse cache last.
- Event plan: active rotation has zero events/no head CAS; reuse has one ordered audit/notification pair, exactly two events (`v+1`, `v+2`), and one CAS `v -> v+2`.
- Replay/reconcile: same key replay never becomes reuse; different key against rotated digest uses the accepted revocation rule; cache loss never rotates/revokes a second time.

##### WF-08 `device_revocation`

- Authority and stable key: active owner and server-loaded target-device owner recheck; `IDEMPOTENCY_KEY`.
- Lock/check order: target device, owner, result claim, dependent sessions/families, `DEVICE` aggregate head.
- Atomic writes in order: device revocation; dependent session/family revocations; immutable audit; internal notification; audit history/Outbox `DEVICE_REVOKED` at `v+1`; notification history/Outbox at `v+2`; nonce reservation; sealed exact non-credential cache last.
- Event plan: two ordered audit/notification events with exact predecessor and one CAS `v -> v+2`.
- Replay/reconcile: cache replay has no second revocation/event; cache loss does not re-execute the mutation.

##### WF-09 `session_revocation`

- Authority and stable key: active owner and server-loaded target-session owner recheck; `IDEMPOTENCY_KEY`.
- Lock/check order: target session, owner, result claim, refresh family, `SESSION` aggregate head.
- Atomic writes in order: session/family revocation; immutable audit; internal notification; audit history/Outbox `SESSION_REVOKED` at `v+1`; notification history/Outbox at `v+2`; nonce reservation; sealed exact non-credential cache last.
- Event plan: two ordered audit/notification events with exact predecessor and one CAS `v -> v+2`.
- Replay/reconcile: cache replay has no second revocation/event; cache loss does not re-execute the mutation.

##### WF-10 `owner_mutation`

- Authority and stable key: active owner and typed allowlisted target-owner recheck; `IDEMPOTENCY_KEY`.
- Lock/check order: owner/target, result claim, business aggregate head.
- Atomic writes in order: typed allowlisted business effect; immutable audit; history/Outbox `OWNER_MUTATION_COMMITTED` at `v+1`; nonce reservation; sealed exact non-credential cache last.
- Event plan: one audit event, one history/Outbox pair and one CAS `v -> v+1`. Arbitrary operation names, trading action invention, account/object authority fields, or absent allowlist fail before effect.
- Replay/reconcile: cache replay returns the exact result only; cache loss cannot repeat the mutation.

##### WF-11 `confirmation_consume`

- Authority and stable key: active server-authored user/account/ownership/session/device; `IDEMPOTENCY_KEY`; the path confirmation ID and body hash grant no authority.
- Lock/check order: owner/ownership, result claim, durable Confirmation, pre-linked `AWAITING_CONFIRMATION` Operation; then exact full authority, intent/hash, covering FK, and expected-version checks from §5.4.1.
- Consumed writes in order: Confirmation `ISSUED -> CONSUMED` CAS; claim-equality `confirmation_consumptions` link; Operation `AWAITING_CONFIRMATION -> CONFIRMED` CAS; immutable owner audit; owner-subject history/Outbox at `v+1` with aggregate type `OPERATION`; nonce reservation; sealed accepted Operation cache last.
- Expired writes in order: at equality/later Confirmation `ISSUED -> EXPIRED` CAS; linked Operation `AWAITING_CONFIRMATION -> EXPIRED` CAS; the same one audit/history/Outbox pair at `v+1`; nonce reservation; sealed typed rejection cache last. Mismatch/cross-owner/inactive paths have zero CAS/history/Outbox and only sealed generic rejection.
- Event plan: success or first expiry has one event and one CAS `v -> v+1`; exact cache replay has no second CAS/event.
- Replay/reconcile: a missing cache may only use full-authority `DURABLE_OPERATION_RECONCILIATION` through immutable consumption/Operation/result-claim links, never an Attempt, Order, signer, dispatch, or exchange write.

`logout` is absent from the accepted ten-workflow manifest and must not be silently mapped to session revocation.

#### 5.4.3 Quarantine, controlled replay, Inbox, Clock, and transport

P1-004 provides typed `quarantine_records` before P1-005 may send `TERM`, use a DLQ, or replay. A validated delivery is unique `(consumer,event_id)` and binds first payload digest; rejected JSON uses candidate identity when valid, otherwise `(consumer,received_bytes_digest)`; undecodable/oversized evidence stores full digest/length and at most 4096 forensic bytes. Same identity/different digest is immutable integrity evidence, never Inbox success/effect. `IsolateDelivery` is one transaction: validate typed failure/scope, lock/insert first quarantine evidence, write one immutable non-business isolation audit, write no owner effect/success Inbox/business Outbox, commit. Only then may P1-005 `TERM`; validation/persistence/audit failure sends delayed `NAK`, never ACK/TERM.

`replay_authorizations` has database checks: `AUTHORIZE_ONCE` is only `ACTIVE`, `CONSUMED`, `EXPIRED`, or `REVOKED`, requires full exact binding and exact `issued_at+900000ms` expiry; only active with no consumption and `snapshot < expires_at` is consumable. `DENY_PERMANENTLY` is only `DENIED_FINAL` and `REQUIRE_MANUAL_RECONCILIATION` only `MANUAL_REVIEW_FINAL`; both forbid binding/expiry/consumption and require `terminal_at=issued_at`. Partial unique is `UNIQUE(quarantine_id) WHERE decision='AUTHORIZE_ONCE' AND state='ACTIVE'`. Deny/manual-active, missing binding, equality expiry, already consumed, wrong owner, or changed digest fail before Inbox/effect; wrong owner/digest is never automatically authorizable. Controlled replay does not publish: in one transaction it locks exact active authorization, revalidates quarantine/envelope/digest/scope/owner, locks/inserts Inbox, applies normal version/predecessor CAS and permitted effect/audit/output Outbox, marks Inbox `APPLIED`, consumes authorization, marks quarantine, then commits. Crash applies/consumes neither.

The only durable Inbox status is `APPLIED`; an internal received stage is not durable. Consumer verifies outer/inner event identity/kind/subject/scope/aggregate/version/predecessor/digest before Inbox insert; it writes effect/audit/output Outbox and `APPLIED` in the same commit, then ACKs. Same `(consumer,event_id)`/digest is idempotent; different digest, wrong owner, stale/future version, or predecessor mismatch cannot succeed. One `ClockSnapshotDTO(utc_now_ms,monotonic_tick,watermark_version)` is taken at entry, drives every timestamp, and is backed by a locked durable watermark; UTC rollback fails `CLOCK_ROLLBACK`, equality expiry is expired, and retry cannot revive an expired claim.

The sole accepted source is `TransportLimitsDTO` version `fit.transport-limits.dev.v1`; it is frozen field-for-field:

| Field group | Exact fields |
| --- | --- |
| HTTP bytes | `max_request_target_bytes=8192`, `max_header_bytes=16384`, `max_unauthenticated_body_bytes=16384`, `max_authenticated_body_bytes=65536`, `max_credential_workflow_body_bytes=32768`, `max_response_body_bytes=65536` |
| WebSocket bytes | `max_websocket_data_frame_bytes=65536`, `max_websocket_application_control_bytes=4096`, `max_websocket_protocol_control_bytes=125` |
| HTTP deadlines | `http_header_read_deadline_ms=5000`, `http_read_deadline_ms=10000`, `http_write_deadline_ms=15000`, `http_idle_deadline_ms=60000` |
| WebSocket deadlines | `websocket_read_deadline_ms=75000`, `websocket_write_deadline_ms=10000`, `websocket_ping_interval_ms=30000`, `websocket_reauth_issue_before_expiry_ms=60000`, `websocket_reauth_max_response_ms=60000` |
| Admission/concurrency | `max_http_connections_global=256`, `max_websocket_connections_global=128`, `max_inflight_requests_global=256`, `max_handler_goroutines_per_connection=2`, `max_admission_queue_depth=128` |
| Shutdown | `shutdown_grace_ms=10000`, `forced_close_at_ms_from_shutdown=10000`, `cleanup_deadline_ms_from_shutdown=15000` |

Equality is accepted at every byte/count maximum and one byte/item over is rejected before handler/control dispatch; every deadline is expired at equality. WebSocket reauthorization deadline is `min(issued_at+60000ms,current_access_expiry)`. No environment variable, command flag, framework default, package constant, or test-only override may change the DTO; missing, unknown-version, or altered DTO fails startup. The accepted per-source/per-session limits from `http-rate-limits-v1.json` combine with every global limit above using **deny-if-any-denies**; passing a global limit cannot override a source/session denial and vice versa. EV-044 must test exact equality, one-over, deadline equality, reauthorization boundaries, the max-two handler boundary, startup rejection for every override class, and this manifest composition rule.

## 6. Isolation, RLS, roles, and immutability

The migrator owns schemas/tables/functions.  The application role is not an
owner, has `NOBYPASSRLS`, no DDL/admin privileges, and cannot disable/alter row
security.  All owner tables have RLS enabled and forced.  Policies require the
transaction-local validated `(user_id, trading_account_id)` scope; owner rows
must match both.  Control-plane tables use separate policies and allow only
the frozen `AUTH_SECURITY` or `SYSTEM` kinds with required source/system
operation linkage; they cannot join into or mutate owner business rows.

The database connection is an internal service capability, never a client
credential.  Direct table mutation is revoked from the application role where
repository functions can provide the narrower operation; otherwise it is
limited to necessary `SELECT/INSERT/UPDATE` columns and guarded by forced RLS,
transition constraints, and immutable triggers.  `SECURITY DEFINER` helpers,
if any, use a pinned safe `search_path`, validate all inputs, expose no generic
SQL, and are executable only by the application role.  Migration and recovery
verification roles are separate; recovery verification is read-only.

Audit/history/notification event content is insert-only.  Outbox allows only
the documented lease and publication transition; Inbox allows only the
transactional `RECEIVED -> APPLIED` transition and no success rewrite.  The
migration owner installs triggers to reject forbidden UPDATE/DELETE even if a
future grant is mistaken.  RLS, grants, constraints, and triggers are all
tested: none alone is accepted as sufficient isolation evidence.

## 7. Migration, retention, and recovery evidence

Migrations are linear, immutable, numbered `NNNN_name.up.sql`, and listed with
SHA-256 in an immutable manifest plus an applied-revision/checksum table.  The
migrator takes a transaction-scoped advisory lock; checksum mismatch, skipped
revision, dirty/incomplete revision, unsupported PostgreSQL version, or missing
required role fails closed before application startup.  Avoid nontransactional
DDL in Phase 1 so each revision has one observable commit/rollback boundary.

Revision 0001 applies from empty.  Every later revision is tested from empty
and from its directly prior numbered revision.  Expand changes retain current
and immediately prior application compatibility.  There are no destructive
down migrations or Phase-1 cleanup migrations; application rollback uses the
compatible expanded schema.  Phase-1 business Outbox rows are never deleted,
truncated, cascaded, or partition-dropped.  Expiry cleanup may only erase the
PostgreSQL `commit_response_cache` ciphertext/nonce after its frozen 120-second
TTL; durable idempotency metadata remains
for reconciliation.  Any future retention/pruning need is a separately
reviewed contract and migration task.

WAL/PITR proof records the frozen synthetic workload (`p1-recovery-synthetic-v1`),
manifest hash, seed `20260729`, rate `20/s`, UTC clock source, injected failure,
committed/recovered sequence, restore start/end, component versions/hashes,
gate outcomes, and measured RPO/RTO arithmetic.  Failed recovery has at least
one failed gate, a stop marker/reason, distinct start/terminal markers, and no
observed RTO claim.  Restore validation checks constraints, aggregate versions,
Outbox continuity, audit linkage, and fixture checksums before success.

## 8. Required implementation handoff

The implementation handoff must contain the exact accepted P1-003 base and
candidate commit, branch/worktree, migration manifest/checksums, PostgreSQL
version/image digest, changed paths, clean/ownership proof, dependency-intake
result, commands and full test outcomes, failure-cut coverage, RLS/role probes,
secret scan, migration compatibility result, retention statement, and every
unresolved risk.  `PASS` does not authorize merge, deploy, production, exchange
connectivity, wallet/signer use, automatic trading, or live orders.

## 9. Known integration risks

1. The P1-003 accepted Clock, exact-response bytes, canonical AAD binding, and
   shared restart-surviving keyring capabilities are not present at this
   baseline.  They are explicit implementation gates; absence or an interface
   unable to satisfy the frozen protocol blocks P1-004 for remediation.
2. P1-004 cannot use a P1-003 Git commit as a schema revision.  The numbered
   manifest and predecessor-compatibility harness are mandatory from migration
   0001 onward.
3. P1-005 must consume the chosen Outbox lease and Inbox projection semantics;
   P1-006 must consume recovery-evidence and WAL-incident facts.  Neither may
   change P1-004's table/transaction contract without a new assigned task.
