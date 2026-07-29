# Phase 1 execution plan — platform and data foundation

## Frozen starting point

- Accepted Phase 0 lineage base:
  `60850b69dd06c46bbe2a9cfab19d27cdb2ef0412`
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
| P1-003 | Server root Codex | `codex/p1-go-api-auth` | `/root/fit-trade-dev/worktrees/p1-go-api-auth` | `services/trading-core/{api,auth,session,ownership}/**`, module manifests | Accepted P1-002 |
| P1-004 | Server root Codex | `codex/p1-postgres-persistence` | `/root/fit-trade-dev/worktrees/p1-postgres-persistence` | `services/trading-core/persistence/**`, `infra/postgres/**`, module manifests | Accepted P1-003 |
| P1-005 | Server root Codex | `codex/p1-nats-observability` | `/root/fit-trade-dev/worktrees/p1-nats-observability` | `services/trading-core/{messaging,observability}/**`, `infra/{nats,observability}/**`, module manifests | Accepted P1-004 |
| P1-006 | Server root Codex | `codex/p1-recovery-infra` | `/root/fit-trade-dev/worktrees/p1-recovery-infra` | `infra/{runtime,recovery}/**` | Accepted P1-005 |
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

Every remediation proposal is a machine-readable record with the discovery
commit, affected component and task, severity, reproducible evidence, permitted
and forbidden paths, proposed owner, exact base, acceptance checks, rollback or
compatibility impact, and unresolved risk. A proposal authorizes no edit. The
coordinator must assign a new task ID, owner, branch, worktree, and exact
descendant base before implementation.

## Frozen Phase 1 security values

- Enrollment Challenge and Device Action Challenge TTL: 120 seconds, single use.
- New-device enrollment first verifies the account password, then issues a
  domain-separated Enrollment Challenge. It binds purpose, a server-issued
  opaque subject handle, candidate public-key fingerprint, nonce, issued time,
  and expiry; it contains no user, device, session, or trading-account ID. Only
  after a valid password does the server-side handle map to a resolved User. The
  candidate Ed25519 key signs the Challenge, proving possession of the new key
  before it is registered. The server then generates the device ID, binds the
  globally unique public key to the server-owned user, and creates the device
  and first session atomically. Client-supplied owner IDs, duplicate keys,
  cross-owner key rebinding, invalid proof, and replay fail closed and are
  audited.
- Replacement enrollment when every prior device is unavailable proves
  possession of the replacement key, not a lost key. After password verification
  and valid Enrollment Challenge proof, it atomically revokes all prior devices,
  sessions, and refresh families before creating the replacement device and its
  first session. Ordinary additional-device enrollment does not revoke prior
  devices. Both paths create distinct internal security notifications.
- Argon2id production parameters are versioned and have minimums of 64 MiB
  memory, 3 iterations, parallelism 1, a 16-byte random salt, and a 32-byte
  result. A visibly non-production test profile may reduce cost, but tests must
  verify production-profile metadata and one production-profile vector.
- Session access token TTL: 15 minutes.
- Each rotating refresh token has a 7-day TTL. A rotated token expires at the
  earlier of seven days from its issuance or the family deadline.
- Rotating refresh-token family maximum lifetime is a hard 30 days measured
  from family creation. Rotation never moves that deadline.
- Refresh-token reuse revokes the complete token family, including every
  already-issued descendant. Presentation of any already-rotated token before
  the family deadline is reuse even if that token's individual TTL has passed.
  An individually expired but never-rotated token is rejected as expired and
  cannot rotate; family-expired tokens are rejected. Each descendant must be
  rejected after reuse.
- Only token digests are stored; plaintext tokens never enter logs or storage.
- Successful login creates a new random session ID and token family and
  invalidates every presented pre-authentication identifier. Ordinary enrollment
  creates an independently random first session for the new device without
  promoting a client-supplied or existing session and preserves other devices;
  replacement enrollment creates an independently random first session only
  after the prior devices, sessions, and families are revoked. Phase 1 has no
  other privilege-transition endpoint; a future transition requires a new
  fixation contract before implementation.
- Session and device revocation must deny every HTTP, WebSocket, and internal
  tool ingress within 5 seconds.
- Device Action Challenge signatures are separate from Enrollment Challenges.
  They use an already registered Ed25519 public key and bind domain, user,
  account, device, session, action, payload hash, nonce, issued time, and expiry.
  Revoked keys fail closed. Phase 1 tests this server protocol with synthetic
  keys; iOS LocalAuthentication and Windows Hello integration are later
  client-phase work and are not claimed by Phase 1.
- Every account-password verification path is enumeration-resistant and uses
  one shared verifier and the same independent, atomic account/source counters.
  The exact starts are `POST /v1/auth/login`,
  `POST /v1/auth/device-enrollments/start`, and
  `POST /v1/auth/device-replacements/start`; attempts aggregate across all three
  so changing routes cannot bypass a delay or lock. In addition to delay/lock
  state, their combined traffic is limited to 10/minute with burst 3 per source.
  "Account counter" is keyed by the server-owned product User ID, never a
  trading-account ID. The source counter increments for every failed
  verification; the account counter increments only when the submitted
  identifier resolves server-side, without changing that route's generic
  response, status, envelope, or minimum timing. An unknown identifier or wrong
  password on an enrollment start receives an indistinguishable synthetic
  Enrollment Challenge wire object with a random opaque subject handle. It has
  no accepted server-side state, is never persisted, and can never complete.
- For either throttle dimension, failures one through four impose delays of 1,
  2, 4, and 8 seconds before another attempt; the fifth failure in 15 minutes
  imposes a 15-minute lock. A request is denied when either counter is delayed
  or locked. At lock expiry, that dimension's failures, delay, and lock are
  atomically cleared, so its next failure is failure one. Successful password
  verification resets only that User's consecutive-failure counter; it does not
  clear source-abuse state. Concurrent attempts and cross-route attempts cannot
  skip a delay, reset, or threshold. Every lock creates an AuditIntent and
  internal NotificationIntent.
- Non-login authenticated HTTP is limited to 20 requests/second with burst 40
  per session and 50 requests/second with burst 100 per source. The complete
  unauthenticated route allowlist is: the three password-verification starts
  above; `POST /v1/auth/device-enrollments/complete` and
  `POST /v1/auth/device-replacements/complete`, limited together to 10/minute
  with burst 3 per source and one attempt per Challenge; `POST /v1/auth/refresh`,
  limited to 30/minute with burst 5 per source and 10/minute with burst 3 per
  token family; `/health/live`, limited to 60/minute with burst 10 per source;
  and `/health/ready`, limited to 5/minute with burst 2 per source. Every other
  route requires authentication before handler dispatch. WebSocket creation is
  limited to 2/second with burst 2 and 5 concurrent connections per session,
  plus 10/second with burst 20 and 20 concurrent connections per source. In
  Phase 1, source means the direct peer IP; forwarding headers are untrusted and
  cannot select a rate-limit identity.
- A WebSocket upgrade requires a valid access token and binds the connection to
  the server-authored user, trading account, device, session, and token expiry.
  Fresh access tokens are obtained only from the HTTPS refresh route, never over
  WebSocket. Sixty seconds before access expiry, or immediately if less than 60
  seconds remain at connection time, the server sends one `REAUTH_REQUIRED`
  control frame containing a single-use random nonce and a deadline equal to
  the earlier of 60 seconds or access-token expiry. The client must return one
  `REAUTH` control frame containing that nonce and a fresh access token from the
  same user, trading account, device, session, and refresh family. Successful
  verification atomically rebinds the connection to the new token expiry and
  consumes the nonce.
- Unsolicited, duplicated, late, cross-owner, cross-account, cross-device,
  cross-session, cross-family, expired, or malformed `REAUTH` fails closed.
  Verification must finish strictly before the deadline; equality is expired.
  Application frames are rejected while an expired token awaits reauthorization;
  missing or failed reauthorization closes with policy code 4401. Session/device
  revocation closes the socket within five seconds regardless of reauthorization.
  Control frames, bearer tokens, and nonces never enter logs, traces, metrics, or
  application messages.
- Audit and Notification scope is a required tagged union. `OWNER` requires
  server-authored user and trading-account IDs and is the only scope permitted
  for business effects. `AUTH_SECURITY` is limited to pre-authentication,
  login, session, and device security events; it requires a server-derived
  pseudonymous source key, permits user ID only after unambiguous resolution,
  and always prohibits trading-account ID, even after account selection.
  `SYSTEM` is limited to recovery, WAL, and infrastructure events and prohibits
  user and trading-account IDs. `AUTH_SECURITY` and `SYSTEM` records are
  control-plane facts, never business entities or authority to access owner
  data. Unknown identifiers and source-only locks must use `AUTH_SECURITY`
  without a phantom owner.
- The pseudonymous source key is
  `HMAC-SHA-256(runtime_source_key, canonical_direct_peer_ip)`, where the peer is
  encoded as the normalized 16-byte IPv6 form (IPv4 uses IPv4-mapped form).
  The runtime key and raw IP are never persisted or logged. The key ID is stored
  with the digest; rotation retains the prior key only for the 30-minute maximum
  throttle/audit-correlation window. Forwarding headers never enter this value.
- AuditEvent common required fields are event ID, actor type and actor ID,
  tagged scope, server occurrence time, causation ID, correlation ID, schema
  version, and payload digest. `OWNER` and request-triggered `AUTH_SECURITY`
  events additionally require request ID. Background `AUTH_SECURITY` events
  instead require a security incident/operation ID and prohibit a fabricated
  request ID. `SYSTEM` events require a system incident/operation ID and also
  prohibit a fabricated request ID. Device and session IDs are present only
  when the allowlisted kind has them.
- Internal Notification requires notification ID, frozen kind enum, severity
  (`INFO`, `WARNING`, or `CRITICAL`), tagged scope, server occurrence time,
  message-code enum, schema-validated message arguments, causation ID,
  correlation ID, and payload digest. Phase 1 kinds are
  `LOGIN_ACCOUNT_LOCKED` (`WARNING`), `LOGIN_SOURCE_LOCKED` (`WARNING`),
  `DEVICE_ENROLLED` (`INFO`), `DEVICE_REPLACED` (`CRITICAL`),
  `DEVICE_REVOKED` (`WARNING`), `SESSION_REVOKED` (`INFO`),
  `REFRESH_REUSE_DETECTED` (`CRITICAL`), and `WAL_ARCHIVE_INTERRUPTED`
  (`CRITICAL`). A kind with any other severity fails closed. Arbitrary text,
  destination, address, credential, Email, Push, and webhook fields are
  forbidden.
- Phase 1 NATS service principals are exactly `outbox-publisher`,
  `platform-consumer`, and `internal-notification-consumer`. P1-001 freezes the
  exact versioned subjects per event kind. Publisher may publish only those
  subjects and may not subscribe; each consumer may subscribe only its explicit
  subject allowlist and may not publish. No principal may use `>` or `*`,
  administer JetStream, or carry user/account claims. The API service has no
  NATS credential.
- Request idempotency digest is SHA-256 over RFC 8785 JCS of a versioned object
  containing the uppercase method, route template, strictly decoded path/query
  parameter objects, strictly decoded JSON body, and server-authored user and
  account IDs. Duplicate query keys, non-JSON mutation requests, unknown fields,
  and ambiguous path normalization fail closed. Authorization, cookie, raw
  token, forwarding, and client-supplied identity headers are excluded.
- Phase 1 never deletes Outbox business events. Archival/deletion policy is
  deferred until a later reviewed phase, so a restored PostgreSQL source can
  rebuild all Phase 1 derived NATS delivery state.
- For every PostgreSQL schema revision, "immediate predecessor" means the
  directly prior numbered schema revision in the linear P1-004 migration
  manifest, not the P1-003 Git commit. The initial revision applies from empty;
  each later revision must apply from empty and its immediately prior revision,
  while the current and immediately prior application revisions both work
  during expand/contract. Destructive cleanup requires a later task after the
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
- If remediation is needed, the complete proposal record and its newly assigned
  task/owner/branch/worktree/base

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
- `database_wal_archive_age` measures seconds since the most recent successful
  archive while WAL is being generated. Any archive-command failure or age over
  60 seconds creates one idempotent `CRITICAL` internal
  `WAL_ARCHIVE_INTERRUPTED` notification and audit event per incident; the
  incident clears only after two consecutive successful archive cycles and age
  at or below 60 seconds.
- Services are disabled by default, non-public, authenticated, deny external
  runtime egress, and contain no connector, executor, signer, or exchange route.
- Independent P1-007 review reports zero P0/P1/P2 and `PASS`.
