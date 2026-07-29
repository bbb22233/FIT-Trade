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

## Required semantic vocabulary

The `x-fit-*` schema keywords are normative, not annotations. Every Phase 1
validator must implement them before accepting a record:

- `x-fit-time-window` requires the named UTC timestamps to differ by exactly
  the frozen number of seconds.
- `x-fit-refresh-window` requires
  `expires_at = min(issued_at + 604800 seconds, family_deadline)` and a family
  deadline after issuance.
- `x-fit-websocket-deadline` requires
  `deadline = min(issued_at + 60 seconds, current_access_expires_at)`.
- `x-fit-payload-integrity` requires an embedded payload, matching payload
  schema version, and SHA-256 over its RFC 8785 JCS bytes.
- `x-fit-recovery-consistency` checks the frozen workload, temporal ordering,
  observed RPO arithmetic, and observed RTO arithmetic.

Raw mutation bodies must be decoded with duplicate-object-key detection at
every nesting depth before ordinary JSON decoding. An already-parsed object is
not sufficient evidence that the input was unambiguous.

Ed25519 signing uses
`UTF8(domain) || 0x00 || UTF8(RFC8785_JCS(challenge))`. Public keys are raw
32-byte lowercase hex with the `ed25519-public:` prefix; signatures are raw
64-byte lowercase hex with the `ed25519-signature:` prefix; fingerprints are
`ed25519:` plus SHA-256 of the raw public key. The golden vectors are verified
cryptographically and every bound challenge field is mutation-tested.

Node.js 24 or newer is the contract-review runtime because its built-in
Argon2id implementation recomputes the production golden vector. Older local
Node versions may execute the rest of the suite, but their output explicitly
reports `NODE_CRYPTO_ARGON2_UNAVAILABLE` and is not sufficient for fixed-commit
acceptance.

All fixture identifiers, fingerprints, digests, and timestamps are synthetic.
No fixture contains a credential, real account, real device, or routable
service endpoint.
