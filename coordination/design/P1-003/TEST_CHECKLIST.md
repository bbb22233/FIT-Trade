# P1-003 R2 test checklist

## Evidence rule

Every item is a future acceptance requirement, not a result. The corresponding
machine manifest marks every oracle `NOT_RUN`; no unchecked line and no design
statement is a PASS. Evidence must record fixed commit, command, input seed and
Clock snapshot, artifact SHA-256, observed output, `-race` applicability, and
P0/P1/P2 findings.

The governing base is `19b778785565d31dc095996ce4d4b0f7d58bebdf`; interface
requirements are frozen at `f61474ba4f96624f34ff478bd0047c28e72becab`. Tests
never use an old candidate as API/fixture/hash oracle and never contact public
network, exchange, wallet, signer, NATS, PostgreSQL, production model, or
external notification service.

## Baseline and typed-boundary gates

- [ ] `P1R2-GATE-001` Prove the implementation commit descends from the named
  candidate and consumes the capsule DTO/enums without copied validators.
- [ ] `P1R2-GATE-002` Reject mutable maps, raw decision strings, nullable
  discriminators, unknown enum values, cross-tag identifiers, unknown limits
  version, and any environment/flag limits override before mutation.
- [ ] `P1R2-GATE-003` From `services/trading-core`, run formatting, vet, the
  focused unit/integration/failure-path suites, clean-cache, and applicable
  race tests at one fixed commit; a repository-root `go test ./services/...`
  command is not acceptable evidence.
- [ ] `P1R2-GATE-004` Prove no changed implementation path reaches an exchange,
  signer, live wallet, production listener, order submission, or trading
  subject.

## Authority, identity, and idempotency

- [ ] `P1R2-AUTH-001` `OWNER` rechecks active server user/account/ownership and
  server-loaded target in the same transaction; URL/body/header authority fails.
- [ ] `P1R2-AUTH-002` `AUTH_SECURITY_SOURCE_ONLY` validates only source key plus
  control-plane aggregate, does no owner lookup, and neither reads nor writes an
  owner business table.
- [ ] `P1R2-AUTH-003` resolved-user security checks active user plus source
  binding, retains no account/ownership, and rejects fabricated fields.
- [ ] `P1R2-AUTH-004` system authority requires server-created operation and
  aggregate; client input cannot create it.
- [ ] `P1R2-IDEMP-001` owner and control-plane scopes are non-null and distinct;
  equal user/account with distinct ownership cannot collapse.
- [ ] `P1R2-IDEMP-002` ten workflows use `IDEMPOTENCY_KEY`, password failure
  uses `REQUEST_ID`, while every claim stores request ID and first digest.
- [ ] `P1R2-IDEMP-003` same scoped key/different first digest conflicts before
  decrypt/effect; same key/equal digest cannot cross authority or route.

## Exact cache, crypto, and reconciliation

- [ ] `P1R2-CACHE-001` Each claim binds immutable `(original_request_id,
  first_request_digest)` and full `ResponseCacheAADDTO`; substitution of any
  authority/route/key/request/context/result/time field fails authentication.
- [ ] `P1R2-CACHE-002` New records use active AES-256-GCM key, 96-bit unique
  nonce reservation before encryption, key ID without key material, and exact
  ciphertext-only cache persistence.
- [ ] `P1R2-CACHE-003` Duplicate `(key_id,nonce)` retries at most three times;
  exhaustion rolls back all facts and returns `NONCE_RESERVATION_FAILED`.
- [ ] `P1R2-CACHE-004` retained decrypt key replays only unexpired sealed bytes;
  unknown/retired key, malformed seal, unavailable key, or AAD mismatch gives
  `RECONCILIATION_REQUIRED_NO_REEXECUTION` with no secret/effect leak.
- [ ] `P1R2-CACHE-005` TTL is exactly 120000 ms from transaction snapshot;
  before/equality/after tests prove equality erases ciphertext, retains claim
  plus nonce ledger, and cannot re-execute.
- [ ] `P1R2-CACHE-006` generic rejections have the same AEAD/AAD/nonce/TTL rule
  as success; headers are strictly allowlisted and cookie only appears sealed
  for credential results.
- [ ] `P1R2-CACHE-007` response-loss replay performs applicable authority
  revalidation, returns exact bytes only when decryptable, and allocates no
  new state/audit/event/nonce.
- [ ] `P1R2-REJECT-001` source-only password threshold uses request ID plus
  first digest as durable claim; keyed locator is accelerator only.
- [ ] `P1R2-REJECT-002` same request ID/equal digest replays without increment;
  different digest conflicts; expired/unreadable cache reconciles without
  another failure.

## Clock, transport, and WebSocket

- [ ] `P1R2-CLOCK-001` exactly one transaction snapshot supplies every time;
  backward UTC watermark rolls back; equal UTC order uses monotonic tick.
- [ ] `P1R2-CLOCK-002` every expiry treats equality as expired; a retry obtains
  a new snapshot without changing stable identity or extending a deadline.
- [ ] `P1R2-LIMIT-001` all exact `TransportLimitsDTO` maxima accept equality and
  reject one above before handler/frame dispatch; all exact deadlines expire at
  equality.
- [ ] `P1R2-LIMIT-002` altered/missing version, env, flag, framework default,
  and test override cannot alter startup limits; shutdown rejects new work,
  drains/force-closes/cleans at 10s/10s/15s.
- [ ] `P1R2-WS-001` upgrade/frame binding is server-authored and rechecks
  revocation; revocation rejects/closes within five seconds.
- [ ] `P1R2-WS-002` reauth is issued 60s before expiry, deadline is
  `min(issue+60000,current expiry)`, nonce is one-use, and success atomically
  rebinds; expiry/deadline rejects. WS loss has no authority.

## Workflow atomicity

- [ ] `P1R2-WF-01` Login writes throttle/pre-auth invalidation/new
  session-family-token/audit-history-Outbox/nonce/sealed credential atomically,
  with one `LOGIN_SUCCEEDED` and no plaintext persistence.
- [ ] `P1R2-WF-02` Real enrollment challenge creates one real challenge and one
  event; synthetic challenge writes no claim, event, cache, or nonce.
- [ ] `P1R2-WF-03` Failed proof consumes exactly one real challenge attempt and
  emits only rejected-proof facts; it never creates credentials.
- [ ] `P1R2-WF-04` Password failure updates source throttle and, only after
  server resolution, user throttle; source-only has no owner lookup; source
  dimension is first and all other applicable atomic facts still commit/roll
  back together.
- [ ] `P1R2-WF-05` Ordinary enrollment preserves prior device/session/family
  records and emits ordered audit/notification `DEVICE_ENROLLED` events.
- [ ] `P1R2-WF-06` Replacement enrollment deterministically revokes every prior
  device/session/family before issuing replacement credentials and emits
  `DEVICE_REPLACED` audit then notification.
- [ ] `P1R2-WF-07` Refresh checks family deadline, ancestor reuse, individual
  expiry, active rotation in order; normal rotation emits zero security events;
  reuse revokes all descendants and emits two ordered events.
- [ ] `P1R2-WF-08` Device revoke checks loaded target ownership, revokes dependent
  session/family, and emits exactly two `DEVICE_REVOKED` events.
- [ ] `P1R2-WF-09` Session revoke checks loaded target ownership, revokes family,
  and emits exactly two `SESSION_REVOKED` events.
- [ ] `P1R2-WF-10` Owner mutation accepts only typed allowlisted operation and
  target, rechecks applicable owner authority, and emits exact owner subject.
- [ ] `P1R2-WF-11` Confirmation consume locks full authority/claim/ticket/
  pre-linked operation, validates every tuple/hash/intent/FK, and makes only
  allowed CAS/link/audit/history/Outbox/cache writes.
- [ ] `P1R2-WF-ROLLBACK` For each workflow, inject failure before/after every
  write and head CAS; assert no partial business/audit/history/Outbox/claim/
  nonce/cache record.

## Confirmation and reconciliation

- [ ] `P1R2-CONF-001` path ID/body hash are assertions, never authority;
  user/account/ownership/session/device all must match server durable values.
- [ ] `P1R2-CONF-002` prove non-null candidate keys/composite FKs cover
  Confirmation, Operation, consumption, and owner result claim; a different
  request claim fails `(confirmation_id,request_claim_id)` FK before CAS.
- [ ] `P1R2-CONF-003` valid consume performs one `ISSUED->CONSUMED` plus one
  `AWAITING_CONFIRMATION->CONFIRMED` CAS, one immutable link, one owner event
  on the exact accepted subject with `OPERATION` aggregate.
- [ ] `P1R2-CONF-004` expiry equality performs the matching one-time EXPIRED
  transitions/event; cross tuple/hash/purpose/intent/inactive mismatch seals a
  generic rejection with zero CAS/history/Outbox.
- [ ] `P1R2-CONF-005` replay has no second CAS/event; absent cache uses only
  full-authority durable Operation reconciliation, never a new claim link,
  Attempt, Order, signing, dispatch, or exchange write.

## Outbox, subjects, and gaps

- [ ] `P1R2-OUTBOX-001` `BeginAggregateAppend` produces stable typed drafts;
  version/predecessor/head CAS algorithm is exact and any failure rolls back
  every effect including claim/cache/nonce.
- [ ] `P1R2-OUTBOX-002` each history/Outbox/EventEnvelope field is identical in
  both directions; publisher consumes only persisted Outbox, not API command.
- [ ] `P1R2-OUTBOX-003` each subject/kind/scope/role/aggregate combination is one
  of the accepted 20 rows; wildcard, alias, invented subject, and wrong
  WF-10/WF-11 aggregate type fail before materialization.
- [ ] `P1R2-OUTBOX-004` applicable authority is checked before an Outbox plan:
  source-only writes only source/control-plane authority, resolved-user writes
  only its security scope, and owner plan rechecks the loaded target; no plan
  invents owner/account fields.
- [ ] `P1R2-GAP-001` Logout remains blocked until a separate accepted workflow,
  cache/idempotency/write-set, and subject decision exists; it is not silently
  mapped to session revocation.

## Required handoff

The implementation handoff must include exact base/commit/branch/worktree,
changed paths, every manifest oracle result/artifact hash, test commands and
outputs, P0/P1/P2 counts, unresolved risks, and dependency commits. It must
explicitly state `MERGE=false`, `DEPLOY=false`, `PRODUCTION=false`, and
`LIVE=false` unless separately authorized.
