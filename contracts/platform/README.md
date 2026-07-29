# FIT Trade Phase 1 platform contracts

This directory is the machine-readable boundary shared by the Phase 1 Go,
PostgreSQL, NATS, client, and recovery implementations. It freezes semantics;
it does not implement a listener, database, broker, exchange connection,
signer, wallet, or trading action.

## Contract groups

- `schemas/platform-v1.schema.json` defines strict identity, authentication,
  request, WebSocket, persistence, event, audit, and notification records.
- `schemas/remediation-proposal-v1.schema.json` defines the only accepted
  cross-task remediation handoff.
- `schemas/recovery-evidence-v1.schema.json` separates measured recovery
  evidence from an SLA target.
- `manifests/security-values-v1.json` and
  `manifests/http-rate-limits-v1.json` freeze authentication and ingress
  constants.
- `manifests/nats-permissions-v1.json` freezes all Phase 1 service principals
  and exact subjects.
- `manifests/transaction-boundaries-v1.json`,
  `manifests/migration-policy-v1.json`,
  `manifests/recovery-policy-v1.json`, and
  `manifests/failure-injection-v1.json` freeze durable ownership and recovery
  behavior.
- `manifests/state-machines-v1.json` makes replay, expiry, throttle, refresh,
  enrollment, WebSocket reauthorization, and WAL-incident behavior executable.
- `manifests/phase0-file-manifest-v1.json` proves the pre-existing Phase 0
  contract surface remains byte-identical, except for the explicitly permitted
  `package.json` test-script addition.

The verifier at `contracts/scripts/verify-platform.mjs` compiles every schema,
executes positive and negative fixtures, replays semantic scenarios, checks
golden canonical-digest vectors, checks exact manifests, and verifies the
Phase 0 file hashes.

All fixture identifiers, fingerprints, digests, and timestamps are synthetic.
No fixture contains a credential, real account, real device, or routable
service endpoint.
