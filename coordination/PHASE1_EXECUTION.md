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
- Session access token TTL: 15 minutes.
- Rotating refresh-token family maximum lifetime: 30 days.
- Refresh-token reuse revokes the complete token family.
- Only token digests are stored; plaintext tokens never enter logs or storage.
- Session and device revocation must deny every HTTP, WebSocket, and internal
  tool ingress within 5 seconds.
- Device Challenge signatures use a registered Ed25519 public key and bind the
  user, account, device, session, action, payload hash, nonce, issued time, and
  expiry. Revoked keys fail closed.
- Login responses are enumeration-resistant. Five failures for the same account
  or source within 15 minutes trigger a 15-minute lock and audit event.
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
- Services are disabled by default, non-public, authenticated, deny external
  runtime egress, and contain no connector, executor, signer, or exchange route.
- Independent P1-007 review reports zero P0/P1/P2 and `PASS`.
