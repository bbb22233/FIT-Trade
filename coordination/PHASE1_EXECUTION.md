# Phase 1 execution plan — platform and data foundation

## Frozen starting point

- Accepted Phase 0 and `main`: `60850b69dd06c46bbe2a9cfab19d27cdb2ef0412`
- Phase: development, local integration, and failure testing only
- Live wallet, signer, exchange API, deployment, production enablement,
  automatic trading, and live orders remain unauthorized.

## Objective

Build a recoverable Go, PostgreSQL, and NATS foundation in which identity,
ownership, trading state, audit state, and message effects cannot be lost,
silently reassigned, or applied twice.

Phase 1 contains no Hyperliquid connectivity. Exchange connectivity begins only
in a separately authorized and reviewed read-only Phase 2.

## Workstreams, ownership, and reserved worktrees

| Task | Owner | Branch | Worktree | Primary owned paths | Base |
| --- | --- | --- | --- | --- | --- |
| P1-001 | Local Codex coordinator | `codex/p1-platform-contracts` | `/Users/guanlan/Documents/FIT-Trade-worktrees/p1-platform-contracts` | Narrow platform contract manifest | Accepted task packets |
| P1-002 | Server root Codex | `codex/p1-go-contract-client` | `/root/fit-trade-dev/worktrees/p1-go-contract-client` | `services/trading-core/platformcontract/**` | Accepted P1-001 |
| P1-003 | Server root Codex | `codex/p1-go-api-auth` | `/root/fit-trade-dev/worktrees/p1-go-api-auth` | `services/trading-core/**` | Accepted P1-002 |
| P1-004 | Server root Codex | `codex/p1-postgres-persistence` | `/root/fit-trade-dev/worktrees/p1-postgres-persistence` | `services/trading-core/**`, `infra/postgres/**` | Accepted P1-003 |
| P1-005 | Server root Codex | `codex/p1-nats-observability` | `/root/fit-trade-dev/worktrees/p1-nats-observability` | `services/trading-core/**`, `infra/nats/**`, `infra/observability/**` | Accepted P1-004 |
| P1-006 | Server root Codex | `codex/p1-recovery-infra` | `/root/fit-trade-dev/worktrees/p1-recovery-infra` | `infra/**` | Accepted P1-005 |
| P1-007 | Local Codex coordinator | `codex/p1-exit-review` | `/Users/guanlan/Documents/FIT-Trade-worktrees/p1-exit-review` | `coordination/evidence/P1-007/**` | Accepted P1-006 |

Each task branch and worktree is created only after its predecessor is accepted.
Every branch must be a direct descendant of the accepted predecessor. The
sequence is linear: no task assembly may use a merge commit or cherry-pick.
Detached, read-only reviewer worktrees are separate from the task worktree and
must prove the same fixed candidate commit.

The local Hermes development profile adds no Agent behavior in Phase 1. It
participates only in the P1-007 adversarial review with a read-only filesystem,
no MCP/tool connection to the integration harness, and no wallet, exchange,
database, NATS, or service credentials.

## Dependency and integration order

1. P1-001 freezes platform, identity, persistence, messaging, and recovery
   semantics.
2. P1-002 implements and accepts the Go consumer of those contracts before API
   development.
3. P1-003 builds identity and API behavior on the accepted consumer.
4. P1-004 adds PostgreSQL atomic persistence.
5. P1-005 adds NATS delivery and observability.
6. P1-006 adds default-disabled infrastructure and measured recovery tooling.
7. P1-007 reviews the exact P1-006 descendant without reassembling components.

P1-003 through P1-006 are sequential because they share identity, transaction,
event, migration, and recovery boundaries. If a later task discovers an earlier
component must change, it stops and proposes a bounded remediation task owned by
that component owner. The coordinator assigns a new descendant branch and
requires fixed-commit acceptance before the blocked task resumes. P1-007 cannot
start while such a remediation remains unresolved.

## Frozen Phase 1 security values

- Device Challenge TTL: 120 seconds, single use.
- New-device enrollment first verifies the account password, then issues a
  single-use 120-second enrollment Challenge. The device proves possession of
  its Ed25519 private key by signing the Challenge and public-key fingerprint.
  The server generates the device ID, binds the globally unique public key to
  the server-owned user, and creates the device and its first session atomically.
  Client-supplied owner IDs, duplicate keys, cross-owner key rebinding, invalid
  proof, and replay fail closed and are audited.
- Argon2id production parameters are versioned and have minimums of 64 MiB
  memory, 3 iterations, parallelism 1, a 16-byte random salt, and a 32-byte
  result. A visibly non-production test profile may reduce cost, but tests must
  verify production-profile metadata and one production-profile vector.
- Session access token TTL: 15 minutes.
- Rotating refresh-token family maximum lifetime: 30 days.
- Refresh-token reuse revokes the complete token family, including every
  already-issued descendant. Each descendant must be rejected after reuse.
- Only token digests are stored; plaintext tokens never enter logs or storage.
- Successful authentication and every privilege transition create a new random
  session ID and token family. Pre-authentication or prior session identifiers
  are never promoted and are invalidated, which is the Phase 1 fixation defense.
- Session and device revocation must deny every HTTP, WebSocket, and internal
  tool ingress within 5 seconds.
- Device Challenge signatures use a registered Ed25519 public key and bind the
  user, account, device, session, action, payload hash, nonce, issued time, and
  expiry. Revoked keys fail closed.
- Login responses are enumeration-resistant. Account and source counters are
  independent and atomic. For either dimension, failures one through four impose
  delays of 1, 2, 4, and 8 seconds before another attempt; the fifth failure in
  15 minutes imposes a 15-minute lock. A request is denied when either counter
  is delayed or locked. Successful authentication resets only that account's
  consecutive-failure counter; it does not clear source-abuse state. Concurrent
  attempts cannot skip a delay or threshold. Every lock creates an AuditIntent
  and internal NotificationIntent.
- Non-login authenticated HTTP is limited to 20 requests/second with burst 40
  per session and 50 requests/second with burst 100 per source. Unauthenticated
  non-login endpoints are limited to 5 requests/minute per source. WebSocket
  creation is limited to 2/second and 5 concurrent connections per session.
  In Phase 1, source means the direct peer IP; forwarding headers are untrusted
  and cannot select a rate-limit identity.
- Request idempotency digest is SHA-256 over RFC 8785 JCS of a versioned object
  containing the uppercase method, route template, strictly decoded path/query
  parameter objects, strictly decoded JSON body, and server-authored user and
  account IDs. Duplicate query keys, non-JSON mutation requests, unknown fields,
  and ambiguous path normalization fail closed. Authorization, cookie, raw
  token, forwarding, and client-supplied identity headers are excluded.
- Phase 1 never deletes Outbox business events. Archival/deletion policy is
  deferred until a later reviewed phase, so a restored PostgreSQL source can
  rebuild all Phase 1 derived NATS delivery state.
- Database migrations support the accepted application version and its
  immediate predecessor. Destructive cleanup requires a later task after the
  predecessor is retired.

## Required handoff for every task

- Exact base and candidate commit IDs
- Branch, task worktree, detached review worktree, and clean-state proof
- Changed paths and proof that no forbidden path changed
- Commands and complete results for tests, linters, dependency checks, and
  secret scans
- Failure-path tests and unresolved risks
- Migration, rollback, compatibility, and retention notes when applicable
- Independent fixed-commit review with findings and exact P0/P1/P2 counts

Green tests or review `PASS` do not authorize merge, deployment, production,
wallet access, exchange connectivity, exchange writes, or automatic trading.

## Phase 1 exit gate

- Duplicate API requests and NATS redelivery produce one business effect.
- A crash at every frozen transaction point recovers without lost or duplicated
  authoritative state.
- Cross-user and cross-account mutation fails at API, WebSocket, internal tool,
  repository, SQL, event, Inbox, Outbox, and audit boundaries.
- Revoked sessions, revoked devices, invalid signatures, and consumed or
  expired challenges fail closed within the frozen limits.
- PostgreSQL point-in-time recovery runs the frozen synthetic workload and
  records observed RPO/RTO using the frozen evidence schema.
- NATS is rebuilt from the complete Phase 1 PostgreSQL Outbox without inventing
  or dropping authoritative events.
- Audit records remain in the same transaction as their business effect.
- Abnormal login and security events create durable internal notifications in
  the same transaction; Phase 1 has no Email, Push, or external delivery.
- Services are disabled by default, non-public, authenticated, deny external
  runtime egress, and contain no connector, executor, signer, or exchange route.
- Independent P1-007 review reports zero P0/P1/P2 and `PASS`.
