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
- `manifests/websocket-protocol-v1.json` is the versioned Phase 1 WebSocket
  overlay. It preserves Phase 0 application messages while superseding the
  Phase 0-only empty client-message list for Phase 1 control frames.
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
  the frozen number of seconds. Contract timestamps are UTC with either
  second or exact millisecond precision; finer fractions are rejected.
- `x-fit-time-order` rejects optional lifecycle timestamps that precede their
  creation or enrollment timestamp.
- `x-fit-refresh-window` requires
  `expires_at = min(issued_at + 604800 seconds, family_deadline)` and a family
  deadline exactly 2592000 seconds after `family_created_at`; rotated records
  retain that original family creation time and cannot point to their own
  digest.
- `x-fit-refresh-family-lineage` validates the complete family as one state:
  every token has the same family/session/deadline, successor digests resolve
  inside that family, each descendant is issued before its parent expires,
  lineage is acyclic, and family revocation covers every already-issued
  descendant.
- `x-fit-websocket-deadline` requires
  issuance at exactly the later of connection time or 60 seconds before access
  expiry, and
  `deadline = min(issued_at + 60 seconds, current_access_expires_at)`.
- `x-fit-payload-integrity` requires the registered
  `fit.platform.event-payload.v1` payload, exact subject/kind/scope agreement,
  a schema-valid complete AuditEvent or Notification durable record with exact
  record/time/causation/correlation linkage, a durable-record digest over the
  record with its digest field omitted, and SHA-256 over the payload's RFC 8785
  JCS bytes. Unknown or scope-inappropriate payloads are invalid even when
  their outer digest is recomputed.
- `x-fit-request-digest` recomputes `canonical_request_digest` from the exact
  method, route template, strictly decoded path/query/body, and server-authored
  user/account fields. A digest-shaped placeholder or stale digest is invalid.
- `x-fit-event-payload` makes standalone payload validation context-complete
  for record kind, scope, identifier, and notification subject class.
- `x-fit-durable-record-digest` binds every AuditEvent and Notification digest
  to the RFC 8785 JCS bytes of that record with `payload_digest` omitted.
- `x-fit-aggregate-history` validates one complete stateful aggregate history
  from version 1: exact aggregate identity, unique event IDs and versions,
  contiguous versions, nondecreasing occurrence time, stable correlation, and
  predecessor linkage.
- `x-fit-enrollment-challenge-state` binds the public wire Challenge to a
  server-owned user/account/source/request record and a one-way subject-handle
  digest. Only this durable state can be consumed.
- Credential-bearing commit responses are retained for 120 seconds only as
  AES-256-GCM ciphertext under an ephemeral runtime key. Same-key retries can
  recover the exact result during that window; after expiry they require
  reconciliation and never re-execute the committed mutation.
- `x-fit-authority-field-names` rejects server-authored fields recursively and
  case-insensitively. `ModelToolMutationInput` additionally rejects
  confirmation ID/hash authority while the authenticated HTTP confirmation
  route can carry its legitimate path/body fields.
- `x-fit-outbox-causality` requires event occurrence at or before Outbox
  creation and creation at or before publication.
- `x-fit-recovery-consistency` checks the frozen workload, temporal ordering,
  observed RPO and successful RTO arithmetic, distinct terminal markers, exact
  component set, and fail-closed result/gate binding. A failed verification
  emits a stop marker and reason but never claims an observed RTO.

Raw mutation bodies must be decoded with duplicate-object-key detection at
every nesting depth before HTTP handler or WebSocket control-frame dispatch.
Only JSON's four whitespace bytes are accepted, prototype-mutation keys are
rejected, and an already-parsed object is not sufficient evidence that the
input was unambiguous.

The raw HTTP request target is decoded before framework routing. Phase 1 uses
an exact ASCII path with no percent-encoded path bytes, forbids dot segments,
backslashes, fragments, empty path segments, duplicate decoded query keys, and
multi-valued query parameters, and constructs the digest path/query objects
only after those checks.

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
