# P1 interface capsule review checklist

## 1. Review identity and boundary

Use this checklist against one fixed candidate commit descended directly from
`9b4b113a4032f7f0eee88297781e3b2d069b1f18`.

This is a design-only review. `PASS` means only that the interface capsule is
internally consistent and sufficiently exact for successor implementation
tasks. It does not authorize merge, deployment, production, exchange/wallet
access, signing, automatic trading, or a live order.

Required review output:

```text
fixed_candidate_commit=<40 hex>
P0=<count>
P1=<count>
P2=<count>
VERDICT=PASS|CHANGES_REQUIRED
MERGE=false
DEPLOY=false
PRODUCTION=false
LIVE_TRADING=false
```

## 2. Mechanical evidence gate

Set explicit values; do not review a moving working tree:

```bash
set -euo pipefail
: "${CANDIDATE:?export exact candidate commit}"
BASE=9b4b113a4032f7f0eee88297781e3b2d069b1f18
WT=/Users/guanlan/Documents/FIT-Trade-worktrees/p1-interface-capsule
```

Run from `$WT`:

```bash
test "$(git rev-parse "$BASE")" = "$BASE"
test "$(git merge-base "$BASE" "$CANDIDATE")" = "$BASE"
test "$(git rev-list --count "$BASE..$CANDIDATE")" = 1
test "$(git diff-tree --no-commit-id --name-only -r "$CANDIDATE" | sort)" = \
$'coordination/design/P1-INTERFACE-CAPSULE/INTERFACE_CAPSULE.md\ncoordination/design/P1-INTERFACE-CAPSULE/MACHINE_EVIDENCE_MANIFEST.json\ncoordination/design/P1-INTERFACE-CAPSULE/REVIEW_CHECKLIST.md'
git diff --check "$BASE" "$CANDIDATE"
git show \
  "${CANDIDATE}:coordination/design/P1-INTERFACE-CAPSULE/MACHINE_EVIDENCE_MANIFEST.json" |
  jq -e .
test -z "$(git status --porcelain --untracked-files=all)"
```

Fail on a second commit, wrong base, deletion, rename/copy, binary file, or any
path outside the three exact design files.

## 3. Source integrity

- [ ] `SRC-001` Candidate base and tree equal the manifest values.
- [ ] `SRC-002` Both P1-003 files are explicitly recorded as untracked,
  non-authoritative advisory snapshots; the observed worktree HEAD is not
  presented as containing them, and hashes are drift notices only.
- [ ] `SRC-003` P1-004 design commit/path/hash all match the manifest.
- [ ] `SRC-004` P1-005 design commit/path/hash all match the manifest.
- [ ] `SRC-005` All thirteen accepted-contract file hashes match the exact
  accepted base object, including confirmation fields/hash/OpenAPI/state/error
  taxonomy and the two confirmation fixtures.
- [ ] `SRC-006` P1-005 independent review evidence is recorded as
  `P0=0/P1=0/P2=0/PASS` without converting PASS into merge/deploy/live
  authority.
- [ ] `SRC-007` Accepted contracts take precedence over prose and local adapter
  names.

Candidate-pinned accepted-input hash command:

```bash
set -euo pipefail
: "${CANDIDATE:?set exact candidate commit}"
BASE=9b4b113a4032f7f0eee88297781e3b2d069b1f18
ACCEPTED_WT=/Users/guanlan/Documents/FIT-Trade-worktrees/p1-platform-contracts
while IFS=$'\t' read -r input_file expected; do
  actual=$(git -C "$ACCEPTED_WT" show "${BASE}:${input_file}" |
    shasum -a 256 | awk '{print $1}')
  test "$actual" = "$expected"
done < <(git show \
  "${CANDIDATE}:coordination/design/P1-INTERFACE-CAPSULE/MACHINE_EVIDENCE_MANIFEST.json" |
  jq -r '.accepted_contract_inputs[] | [.path,.sha256] | @tsv')

P1003_WT=/Users/guanlan/Documents/FIT-Trade-worktrees/p1-003-design-checklist
test "$(git -C "$P1003_WT" rev-parse HEAD)" = \
  95e77c967e2f9832b6804101d7ff8567c0530fed
! git -C "$P1003_WT" ls-files --error-unmatch \
  coordination/design/P1-003/DESIGN.md \
  coordination/design/P1-003/TEST_CHECKLIST.md >/dev/null 2>&1
test "$(git -C "$P1003_WT" status --porcelain --untracked-files=all -- \
  coordination/design/P1-003/DESIGN.md \
  coordination/design/P1-003/TEST_CHECKLIST.md)" = \
$'?? coordination/design/P1-003/DESIGN.md\n?? coordination/design/P1-003/TEST_CHECKLIST.md'
```

## 4. Typed boundary

- [ ] `TYPE-001` Every primitive has a strict representation and tagged
  identifiers are not freely assignable.
- [ ] `TYPE-002` Authority, stable-key, replay, cache, Outbox, Inbox,
  quarantine, and replay-authorization decisions are closed enums.
- [ ] `TYPE-003` Unknown enum values and nullable/ambiguous union
  discriminators fail before mutation.
- [ ] `TYPE-004` No mutable map, `any`, raw string decision, client-owned
  decoded object, or local duplicate wire DTO crosses the interface.
- [ ] `TYPE-005` Direction is P1-002 -> P1-003 -> P1-004 -> P1-005; no reverse
  authority dependency exists.

## 5. Response and rejection cache

- [ ] `CACHE-001` A durable claim stores `original_request_id` and immutable
  `first_request_digest`.
- [ ] `CACHE-002` The stable claim is a tagged idempotency key or request ID;
  owner and control-plane scopes use separate non-null uniqueness keys.
- [ ] `CACHE-003` AES-256-GCM ciphertext is persisted while key material stays
  in a deployment-shared runtime keyring outside PostgreSQL.
- [ ] `CACHE-004` Canonical AAD binds claim ID, typed authority scope, route,
  stable-key kind/digest, original request ID, first request digest,
  authenticated context digest, result class, key ID, and exact timestamps.
- [ ] `CACHE-005` TTL is exactly 120,000 ms from the one transaction snapshot
  and equality is expired.
- [ ] `CACHE-006` Same scoped key/same digest replays exact bytes without a new
  effect.
- [ ] `CACHE-007` Same scoped key/different digest conflicts before decrypt or
  mutation.
- [ ] `CACHE-008` Missing, expired, erased, malformed, AAD-mismatched, or
  key-unavailable cache returns reconciliation-required/no-reexecution.
- [ ] `CACHE-009` Generic rejections use the same AEAD, atomicity, replay, and
  TTL rules.
- [ ] `CACHE-010` Cache is the last durable workflow write and response occurs
  only after commit.
- [ ] `CACHE-011` Plaintext credentials, cookies, password values, raw requests,
  and client authority never enter cache metadata or observability.

## 6. Nonce ledger

- [ ] `NONCE-001` `(key_id,nonce96)` is unique in a separate append-only
  ledger.
- [ ] `NONCE-002` The ledger reservation and sealed cache row commit in the
  same transaction as the effect.
- [ ] `NONCE-003` Cache TTL cleanup does not delete the ledger reservation.
- [ ] `NONCE-004` Reservation succeeds before encryption; a collision obtains a
  new nonce, and three failed reservations roll back the full transaction
  without encrypting under a reused pair.

## 7. Authority and idempotency scope

- [ ] `AUTH-001` `OWNER` requires user, account, ownership ID and active
  in-transaction owner/target checks.
- [ ] `AUTH-002` source-only `AUTH_SECURITY` requires source key plus
  control-plane aggregate and forbids user/account.
- [ ] `AUTH-003` source-only processing performs no owner lookup and cannot
  mutate owner business rows.
- [ ] `AUTH-004` resolved-user `AUTH_SECURITY` may include only an unambiguously
  server-resolved user and always forbids trading-account ID.
- [ ] `AUTH-005` `SYSTEM` requires a server-created operation and forbids owner
  fields.
- [ ] `AUTH-006` ownerless/control-plane idempotency has a non-empty aggregate,
  route, stable-key kind, and stable-key digest.
- [ ] `AUTH-007` no uniqueness relies on nullable owner columns or a global null
  scope.
- [ ] `AUTH-008` Owner idempotency scope and every linked result claim include
  non-null `ownership_id`; equal user/account values cannot collapse distinct
  ownership records.

## 8. Outbox and event ordering

- [ ] `OUTBOX-001` The typed event plan includes stable event identity, exact
  subject/kind/scope, aggregate identity, payload bytes/digest, and durable
  link IDs.
- [ ] `OUTBOX-002` One aggregate head is locked exactly once per plan.
- [ ] `OUTBOX-003` `n` drafts receive contiguous `v+1..v+n` versions and an
  exact predecessor chain.
- [ ] `OUTBOX-004` Version 1 forbids a predecessor; every later version requires
  the immediately previous event ID.
- [ ] `OUTBOX-005` Every draft produces one history row and one Outbox row.
- [ ] `OUTBOX-006` Final head CAS happens after all pairs and any failure rolls
  back all related facts.
- [ ] `OUTBOX-007` Event cardinalities match the table, including password
  `2*L` source-then-user ordering and zero-event refresh/non-threshold cases.
- [ ] `OUTBOX-008` Publisher reads only P1-004 Outbox and never creates or
  remaps an event.

## 9. Operation, Attempt, and Order facts

- [ ] `FACT-001` Operation, ExecutionAttempt, and Order fields preserve the
  accepted contract and add explicit non-null user/account/ownership tuples.
- [ ] `FACT-002` Composite FKs preserve exact user/account/ownership through
  `TradingAccountOwnership -> Operation -> Attempt -> Order`.
- [ ] `FACT-008` A confirmation-required Operation pre-link includes non-null
  `ownership_id` in its Confirmation composite FK and exposes the covering
  owner/intent candidate key required by the consumption relation.
- [ ] `FACT-009` ExecutionAttempt contains non-null `ownership_id`, exposes
  `(attempt_id,operation_id,user_id,trading_account_id,ownership_id)`, and has
  composite FKs to Operation and `TradingAccountOwnership`.
- [ ] `FACT-010` Order contains non-null `ownership_id`, exposes its complete
  owner candidate key, and has ownership-complete composite FKs to Attempt,
  Operation, and `TradingAccountOwnership`.
- [ ] `FACT-003` Request, event, attempt ID,
  `(operation,ownership,attempt_number)`, client-order ID, order/attempt, and
  consumer Inbox dedup constraints are explicit.
- [ ] `FACT-004` `exchange_order_id` is unique per
  account/ownership when present.
- [ ] `FACT-005` Operation state changes require expected-version CAS.
- [ ] `FACT-006` Unknown execution result cannot trigger a blind replacement
  order or automatic retry.
- [ ] `FACT-007` No accepted trading-domain/execution JetStream subject exists
  for Operation, Attempt, or Order; the generic accepted owner audit event is
  not presented as such a subject.
- [ ] `FACT-011` `OWNER-FK-NEG-ATTEMPT-OWNERSHIP` and
  `OWNER-FK-NEG-ORDER-OWNERSHIP` prove equal user/account with different
  ownership fails a constraint before any Attempt/Order insert or state change.

## 10. Confirmation consumption

- [ ] `CONF-001` The public consume request carries only the accepted
  confirmation ID, hash, and idempotency key; all
  user/account/ownership/session/device authority is server-authored and
  rechecked in the transaction.
- [ ] `CONF-002` The durable Confirmation freezes owner tuple, ownership,
  device, session, purpose, canonical intent/digest, all accepted risk snapshot
  fields, hash, unique one-time nonce, issue/expiry times, and
  `ISSUED|CONSUMED|EXPIRED` presence rules; the nonce is never observable.
- [ ] `CONF-003` Purpose is derived from the accepted intent position effect,
  and the hash is recomputed from the complete accepted ticket field set.
- [ ] `CONF-004` Confirmation has the exact unique owner-intent key
  `(confirmation_id,user_id,trading_account_id,ownership_id,intent_id)` and the
  exact unique full-authority/intent key covering ownership, session, device,
  purpose, intent ID, and intent digest.
- [ ] `CONF-011` The existing `AWAITING_CONFIRMATION` Operation pre-link FK
  includes ownership; successful consumption stores `ownership_id`, purpose,
  intent digest, session, and device and has composite FKs to the full
  Confirmation key, Operation covering key, and non-null owner result claim.
- [ ] `CONF-005` Success uses one-use Confirmation CAS plus expected-version
  Operation CAS in the same transaction; it cannot create a second Operation,
  Attempt, or Order.
- [ ] `CONF-006` The same scoped key and first request digest replay exactly;
  a different digest conflicts before mutation, and an already consumed ticket
  resolves only through the durable full-authority consumption, Operation, and
  result-claim relations.
- [ ] `CONF-007` Expiry at equality transitions Confirmation and linked
  Operation once; cross-owner, replay, inactive, hash, purpose, intent, or
  relation mismatch fails closed without an authority leak or trading effect.
- [ ] `CONF-008` Success/expiry atomically include audit, history, Outbox,
  nonce, cache, Operation CAS, and consumption link as applicable.
- [ ] `CONF-009` The only event is
  `fit.platform.v1.owner.owner-mutation-committed` with
  `OWNER_MUTATION_COMMITTED`, `OWNER`, and aggregate type `OPERATION`; it is an
  audit/projection event, not trading execution authority.
- [ ] `CONF-010` Lost HTTP response or missing/delayed/duplicate WebSocket
  delivery uses exact cache replay or `DURABLE_OPERATION_RECONCILIATION` plus
  Operation GET; it never re-executes consume.
- [ ] `CONF-012` Adversarial FK/constraint tests prove that equal user/account
  with different ownership, equal ownership with different session, and equal
  ownership/session with different device all fail before CAS, relation insert,
  cached result, or reconciliation. The four machine case IDs are exactly
  `CONF-AUTH-NEG-OWNERSHIP`, `CONF-AUTH-NEG-SESSION`, and
  `CONF-AUTH-NEG-DEVICE`, plus `CONF-AUTH-NEG-CLAIM` for the same complete
  owner tuple with a different consume claim.
- [ ] `CONF-013` Confirmation exposes unique
  `(confirmation_id,consumed_request_claim_id)` and consumption has declarative
  FK `(confirmation_id,request_claim_id)` to it; independent claim FKs or
  application ordering are insufficient.
- [ ] `CONF-014` Original exact replay proves both stored claim IDs are equal
  through that FK. A new reconciliation claim links separately to the same
  owner-bound Operation and never rewrites the immutable consumption claim.

## 11. Quarantine and replay

- [ ] `QR-001` Quarantine is a typed P1-004 transactional interface, not a
  P1-005 local database or broker invention.
- [ ] `QR-002` Terminal failure isolation and immutable audit commit atomically
  with no business effect or successful Inbox.
- [ ] `QR-003` TERM occurs only after that commit; failed isolation gets delayed
  NAK and never ACK/TERM.
- [ ] `QR-004` Validated quarantine binds consumer, event ID, first digest,
  subject, aggregate, delivery attempt, failure code, and bounded canonical
  envelope; rejected strict JSON retains tagged non-authoritative candidates;
  undecodable/oversized input binds consumer, complete byte digest/length, and
  at most a 4,096-byte prefix without inventing event identity.
- [ ] `QR-005` Same event ID/different digest cannot overwrite the first
  quarantine fact.
- [ ] `QR-006` Replay authorization is typed, durable, reviewed, bound,
  one-use, limited to one active authorization per quarantine, and expires
  exactly 900,000 ms after its decision transaction snapshot.
- [ ] `QR-007` API, publisher, ordinary consumer, NATS principal, headers,
  subjects, and message body cannot create replay authority.
- [ ] `QR-008` Wrong-owner/digest conflicts are never automatic and require
  explicit reconciliation proof for any reviewed decision.
- [ ] `QR-009` Controlled replay uses the identical envelope and normal strict
  scope/digest/Inbox/CAS/audit path; it never publishes.
- [ ] `QR-010` Authorization consumption, Inbox/effect, audit/output Outbox, and
  quarantine state commit together.
- [ ] `QR-011` Rejected JSON can receive `AUTHORIZE_ONCE` only after current
  strict revalidation creates a complete replay binding; undecodable,
  truncated, malformed, or oversized evidence cannot receive it.
- [ ] `QR-012` Recording a replay decision locks the quarantine and atomically
  writes one immutable control-plane review audit without a business effect or
  business Outbox event.
- [ ] `QR-013` The database decision/state matrix permits `ACTIVE` only with
  `AUTHORIZE_ONCE`, complete binding, exact expiry, and absent consumption
  fields; deny and manual-review decisions have distinct terminal states.
- [ ] `QR-014` The active partial unique index and worker
  `SELECT ... FOR UPDATE` predicate both include
  `decision=AUTHORIZE_ONCE AND state=ACTIVE`, complete binding, unconsumed
  fields, and `snapshot < expires_at`.
- [ ] `QR-015` `DENY_PERMANENTLY+ACTIVE`,
  `REQUIRE_MANUAL_RECONCILIATION+ACTIVE`, missing binding, expiry equality, and
  already-consumed authorization are five machine negative cases that fail
  before Inbox/effect.

## 12. Clock and transport limits

- [ ] `TIME-001` One immutable UTC/monotonic snapshot supplies every time in a
  transaction.
- [ ] `TIME-002` durable watermark rollback fails `CLOCK_ROLLBACK`; it is not
  clamped.
- [ ] `TIME-003` equality is expired and serialization retry retains stable
  identities.
- [ ] `LIMIT-001` `fit.transport-limits.dev.v1` lists exact request/header/body/
  response/frame/control/deadline/connection/inflight/queue/shutdown values.
- [ ] `LIMIT-002` exact maximum is accepted and one over is rejected before
  dispatch.
- [ ] `LIMIT-003` WebSocket reauth uses the accepted minimum deadline formula.
- [ ] `LIMIT-004` shutdown reject/drain/force-close/cleanup times are
  deterministic.
- [ ] `LIMIT-005` environment, flag, framework default, package constant, and
  test override cannot change the values.
- [ ] `LIMIT-006` accepted per-source/session limits combine with global limits
  using deny-if-any-denies.

## 13. Eleven atomic request workflows

- [ ] `WF-001` Manifest and document contain exactly eleven request workflow
  sets.
- [ ] `WF-002` Names are exactly successful login, real Challenge, failed
  proof, password failure, ordinary enrollment, replacement enrollment, refresh,
  device revoke, session revoke, owner mutation, and confirmation consume.
- [ ] `WF-003` Ten use idempotency key and password failure uses request ID.
- [ ] `WF-004` All eleven use one PostgreSQL transaction and final sealed cache
  write.
- [ ] `WF-005` Each set names authority, locks/checks, complete effects,
  audit/notification/event cardinality, nonce reservation, and response class.
- [ ] `WF-006` synthetic Challenge, NATS consumer, Outbox publish, and WAL
  incident are correctly excluded from the eleven request sets.
- [ ] `WF-007` logout is called out as a contract gap rather than silently
  folded into session revocation.

Machine count:

```bash
set -euo pipefail
: "${CANDIDATE:?set exact candidate commit}"
git show \
  "${CANDIDATE}:coordination/design/P1-INTERFACE-CAPSULE/MACHINE_EVIDENCE_MANIFEST.json" |
  jq -e '(.request_workflows | length == 11) and
    (.frozen_counts.request_workflow_atomic_sets == 11) and
    (.frozen_counts.confirmation_authority_negative_cases == 4) and
    (.frozen_counts.owner_tuple_negative_cases == 2) and
    (.frozen_counts.replay_authorization_negative_cases == 5)'
test "$(git show \
  "${CANDIDATE}:coordination/design/P1-INTERFACE-CAPSULE/INTERFACE_CAPSULE.md" |
  rg '^### WF-[0-9]{2} ' | wc -l | tr -d ' ')" = 11
```

## 14. Three-layer field mapping

- [ ] `MAP-001` Request/authority/claim fields map exactly from P1-003 to
  PostgreSQL and are explicitly absent from JetStream where required.
- [ ] `MAP-002` Every EventEnvelope field maps to immutable history/Outbox and
  exact JetStream body/subject.
- [ ] `MAP-003` Outer/inner event identity, kind, subject, scope, aggregate,
  version, predecessor, and digest parity are required before Inbox mutation.
- [ ] `MAP-004` The document lists exactly all 20 accepted subject bindings.
- [ ] `MAP-009` Every subject row additionally freezes one exact aggregate
  binding and rejects a subject/kind/scope/aggregate mismatch; the owner subject
  has only the two typed WF-10 `OWNER_OPERATION` and WF-11 `OPERATION`
  bindings, not an aggregate wildcard.
- [ ] `MAP-005` `FIT_PLATFORM_V1` and both durable consumer names match the
  accepted manifest.
- [ ] `MAP-006` API has no NATS credential and broker role never supplies owner
  authority.
- [ ] `MAP-007` Operation/Attempt/Order database/API mappings explicitly have
  no trading-domain JetStream mapping; WF-11's generic owner audit is
  distinguished from an Operation execution fact.
- [ ] `MAP-008` Quarantine, isolation audit, replay authorization, controlled
  replay, Clock, and TransportLimits map exactly to internal/database/JetStream
  absence or disposition without creating a public authority route.
- [ ] `MAP-010` Confirmation request, durable fact,
  user/account/ownership/session/device binding, Operation relation,
  consume/expiry CAS, API reconciliation, and owner audit fields have exact
  database/API/JetStream mappings.
- [ ] `MAP-011` `ownership_id` is non-null in owner idempotency, Confirmation,
  Operation pre-link, consumption, and result-reconciliation mappings; session
  and device are covered by the full Confirmation/consumption FK.
- [ ] `MAP-012` Attempt and Order mapping includes non-null ownership candidate
  keys plus ownership-complete FKs; no new JetStream subject is introduced.
- [ ] `MAP-013` Replay mapping carries the exact decision/state/presence matrix,
  partial unique predicate, worker predicate, and terminal deny/manual states;
  it is never broker authority.

Machine subject parity:

```bash
set -euo pipefail
: "${CANDIDATE:?set exact candidate commit}"
BASE=9b4b113a4032f7f0eee88297781e3b2d069b1f18
ACCEPTED_WT=/Users/guanlan/Documents/FIT-Trade-worktrees/p1-platform-contracts
DOC=$(git show \
  "${CANDIDATE}:coordination/design/P1-INTERFACE-CAPSULE/INTERFACE_CAPSULE.md")
test "$(git -C "$ACCEPTED_WT" show \
  "${BASE}:contracts/platform/manifests/nats-permissions-v1.json" |
  jq '.stream.subjects | length')" = 20
test "$(printf '%s\n' "$DOC" | rg '^\| `fit\.platform\.v1\.' |
  wc -l | tr -d ' ')" = 20
while IFS=$'\t' read -r subject kind scope consumer; do
  printf '%s\n' "$DOC" |
    rg -F "| \`$subject\` | \`$kind\` | \`$scope\` | \`$consumer\` |" \
    >/dev/null
done < <(git -C "$ACCEPTED_WT" show \
  "${BASE}:contracts/platform/manifests/nats-permissions-v1.json" |
  jq -r '.subject_bindings[] |
    [.subject,.event_kind,.scope,.consumer] | @tsv')
```

## 15. Contract and content checks

Run the official verifiers only from the clean accepted-contract worktree at
the exact base. Do not execute contract code from the candidate worktree:

```bash
set -euo pipefail
BASE=9b4b113a4032f7f0eee88297781e3b2d069b1f18
ACCEPTED_WT=/Users/guanlan/Documents/FIT-Trade-worktrees/p1-platform-contracts
test "$(git -C "$ACCEPTED_WT" rev-parse HEAD)" = "$BASE"
test -z "$(git -C "$ACCEPTED_WT" status --porcelain --untracked-files=all)"
(cd "$ACCEPTED_WT" &&
  node contracts/scripts/verify-platform.mjs &&
  node contracts/scripts/verify.mjs)
```

Run the independent capsule oracle only against the fixed candidate object:

```bash
set -euo pipefail
: "${CANDIDATE:?set exact candidate commit}"
BASE=9b4b113a4032f7f0eee88297781e3b2d069b1f18
DOC=$(git show \
  "${CANDIDATE}:coordination/design/P1-INTERFACE-CAPSULE/INTERFACE_CAPSULE.md")
MANIFEST=$(git show \
  "${CANDIDATE}:coordination/design/P1-INTERFACE-CAPSULE/MACHINE_EVIDENCE_MANIFEST.json")
printf '%s\n' "$MANIFEST" | jq -e '
  .requirements | length == 11 and
  ([.[].id] | unique | length == 11) and
  all(.[]; .status == "FROZEN")'
printf '%s\n' "$DOC" |
  rg -F '### 7.3 Confirmation consumption and durable Operation relation'
printf '%s\n' "$DOC" | rg -F '### WF-11 `confirmation_consume`'
printf '%s\n' "$DOC" | rg -F '`DURABLE_OPERATION_RECONCILIATION`'
printf '%s\n' "$DOC" |
  rg -F '(confirmation_id,user_id,trading_account_id,ownership_id,intent_id)'
printf '%s\n' "$DOC" |
  rg -F '(confirmation_id,user_id,trading_account_id,ownership_id,session_id,device_id,purpose,intent_id,intent_digest)'
printf '%s\n' "$DOC" |
  rg -F '(operation_id,confirmation_id,user_id,trading_account_id,ownership_id,intent_id)'
printf '%s\n' "$DOC" |
  rg -F '(confirmation_id,consumed_request_claim_id)'
printf '%s\n' "$DOC" |
  rg -F '(confirmation_id,request_claim_id)'
printf '%s\n' "$DOC" |
  rg -F '(attempt_id,operation_id,user_id,trading_account_id,ownership_id)'
printf '%s\n' "$DOC" |
  rg -F '(order_id,attempt_id,operation_id,user_id,trading_account_id,ownership_id)'
printf '%s\n' "$DOC" |
  rg -F "decision='AUTHORIZE_ONCE' AND state='ACTIVE'"
printf '%s\n' "$MANIFEST" | jq -e '
  (.frozen_counts.confirmation_authority_negative_cases == 4) and
  (.frozen_counts.owner_tuple_negative_cases == 2) and
  (.frozen_counts.replay_authorization_negative_cases == 5) and
  ((.confirmation_authority_negative_cases | map(.mutated_field)) ==
    ["ownership_id","session_id","device_id","request_claim_id"]) and
  ((.replay_authorization_negative_cases | map(.id)) ==
    ["REPLAY-AUTH-NEG-DENY-ACTIVE","REPLAY-AUTH-NEG-MANUAL-ACTIVE",
     "REPLAY-AUTH-NEG-MISSING-BINDING","REPLAY-AUTH-NEG-EXPIRED",
     "REPLAY-AUTH-NEG-CONSUMED"])'
git diff --check "$BASE" "$CANDIDATE"
! git diff --no-color "$BASE" "$CANDIDATE" -- \
  coordination/design/P1-INTERFACE-CAPSULE |
  rg '^\+.*[[:blank:]]$'
```

Also run every copyable command recorded in `self_checks` from the candidate's
manifest, including fixed-object input hashes, subject parity, advisory drift,
secret scan, scope, and whitespace. Scan the complete candidate diff for
credentials/secrets and prohibited execution/deployment capability. A word in
a prohibition statement is not an implementation capability; inspect every
match.

## 16. Findings and verdict rule

Classify:

- `P0`: introduces or authorizes live/exchange/wallet/signer/order/deployment
  capability; loses owner authority; allows duplicate effect; or permits
  ACK/TERM/replay before required durable proof.
- `P1`: incomplete/contradictory typed field, transaction set, cache/replay,
  nonce, version, dedup, Clock/limit, or three-layer mapping that blocks safe
  implementation.
- `P2`: material clarity/evidence defect that does not by itself create an
  unsafe implementation path.

`PASS` requires `P0=0`, `P1=0`, `P2=0`, every mechanical command passing, and
no unreported scope change. Otherwise return `CHANGES_REQUIRED` with findings
first and exact file/line evidence.
