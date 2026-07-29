# Phase 1 execution plan — platform and data foundation

## Frozen starting point

- Accepted Phase 0 and `main`: `60850b69dd06c46bbe2a9cfab19d27cdb2ef0412`
- Phase: development, local integration, and failure testing only
- Live wallet, signer, exchange write API, deployment, production enablement,
  automatic trading, and live orders remain unauthorized.

## Objective

Build a recoverable Go, PostgreSQL, and NATS foundation in which identity,
ownership, trading state, audit state, and message effects cannot be lost,
silently reassigned, or applied twice.

Phase 1 does not connect Hyperliquid. Exchange connectivity starts with a
separately reviewed read-only Phase 2.

## Workstreams and ownership

| Task | Owner | Branch | Primary owned paths | Depends on |
| --- | --- | --- | --- | --- |
| P1-001 | Local Codex coordinator | `codex/p1-platform-contracts` | `contracts/**` | Accepted task packets |
| P1-002 | Server root Codex | `codex/p1-go-api-auth` | `services/trading-core/**` | Accepted P1-001 |
| P1-003 | Server root Codex | `codex/p1-postgres-persistence` | `services/trading-core/**`, `infra/postgres/**` | Accepted P1-002 |
| P1-004 | Server root Codex | `codex/p1-nats-observability` | `services/trading-core/**`, `infra/nats/**`, `infra/observability/**` | Accepted P1-003 |
| P1-005 | Server root Codex | `codex/p1-recovery-infra` | `infra/**` | Accepted P1-003 and P1-004 |
| P1-006 | Local Codex coordinator with independent reviewers | `codex/p1-exit-review` | Integration only; no independent semantics | Accepted P1-001 through P1-005 |

The local Hermes development profile does not add production Agent behavior in
Phase 1. It participates only in adversarial identity and capability-boundary
review for P1-006. This avoids inventing Agent dependencies before the platform
has a stable authenticated API.

## Dependency and integration order

1. Freeze platform, identity, persistence, messaging, and recovery semantics in
   P1-001.
2. Build the Go API/authentication foundation in P1-002.
3. Add real PostgreSQL transactions and persistence in P1-003.
4. Add NATS delivery and observability in P1-004.
5. Add default-disabled deployment and measured recovery tooling in P1-005.
6. Assemble exact accepted commits and run P1-006.

P1-003 through P1-005 are deliberately sequential because they share the same
transaction, event, migration, and recovery boundaries. They must not be
developed as conflicting parallel branches.

## Required handoff for every task

- Exact base and candidate commit IDs
- Changed paths and proof that no forbidden path changed
- Commands and complete results for tests, linters, dependency checks, and
  secret scans
- Failure-path tests and unresolved risks
- Migration, rollback, and compatibility notes when applicable
- Independent fixed-commit review with findings and exact P0/P1/P2 counts

Green tests or review `PASS` do not authorize merge, deployment, production,
wallet access, exchange writes, or automatic trading.

## Phase 1 exit gate

- Duplicate API requests and NATS redelivery produce one business effect.
- A crash at every named transaction boundary recovers without lost or
  duplicated authoritative state.
- Cross-user and cross-account identifier mutation cannot read or change
  another owner's data.
- Revoked sessions and consumed/expired device challenges fail closed.
- PostgreSQL point-in-time recovery is exercised and records observed RPO/RTO.
- NATS can be rebuilt from the PostgreSQL Outbox without inventing business
  facts.
- Audit records remain correlated with the authoritative transaction.
- All services and infrastructure remain default-disabled and non-public.
- Independent P1-006 review reports zero P0/P1/P2 and `PASS`.
