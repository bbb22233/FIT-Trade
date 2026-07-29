# P1-005 JetStream acceptance test checklist

## How to use this checklist

Each row has one unique evidence ID. Commands are mandatory automated
acceptance commands for the P1-005 implementation worktree; they intentionally
name only P1-005-owned packages/harnesses. Before running the integration rows,
the harness must start disposable, authenticated, loopback-only PostgreSQL and
NATS/JetStream instances with synthetic principals and no external egress.

`$P1_005_BASE` below is the exact accepted P1-004 commit and `$P1_005_CANDIDATE`
is the candidate under review. Every Go command runs from the module root shown
explicitly as `(cd services/trading-core && ...)`; no command is interpreted
from repository root. The implementation must provide the named Go tests,
`infra/nats/evidence-manifest.json`, and `infra/nats/test-harness`; a missing
command/test is a failed check, not waived evidence. Each command uses
`-count=1`; tests which mutate shared delivery state must not use `t.Parallel`.

## Fail-closed evidence-runner gate

Before executing **any** Go test command in the table, the test-only harness
must run this discovery gate from repository root:

```bash
./infra/nats/test-harness --mode evidence-runner --phase discover \
  --manifest infra/nats/evidence-manifest.json \
  --go-cwd services/trading-core --ids P1-005-EV-001:P1-005-EV-029
```

The machine-readable manifest maps each evidence ID `001` through `029` to one
explicit module-relative package and one exact `Test...` name. The runner must
invoke `(cd services/trading-core && go test -list '^<exact-test-name>$'
<explicit-package>)` for each entry, parse the result, and fail unless that
exact name occurs **once** and only once. A package wildcard, substring match,
missing mapping, duplicate mapping, unexpected ID, list-command failure, or
any discovered test name outside the manifest is a hard failure. Only after all
29 discovery checks pass may it execute the listed test commands.

EV-030 is the completion gate. It must collect `go test -json` for the full
messaging/observability suite, map every declared test result back to the same
manifest, and fail if any required named test is absent, has a `skip` action,
does not end in `pass`, or if an undeclared evidence test is observed. The
runner writes a machine-readable results record containing base/candidate,
manifest digest, package, test, action sequence, and exit status; it must not
contain credentials, message bodies, raw IP, or user/account authority data.

The same test-only harness supplies a fail-closed `diff-audit` mode for
EV-034/035. It takes both full commit IDs, never the working tree, obtains
`git diff --name-status -z`, `git diff --binary`, and the candidate blob for
every added file, and fails on an unreadable blob, binary addition, rename,
copy, deletion, empty candidate diff, or unrecognized status. Its strict
P1-005 path policy permits only descendants of
`services/trading-core/messaging/`, `services/trading-core/observability/`,
`infra/nats/`, and `infra/observability/`, plus exactly
`services/trading-core/go.mod` and `services/trading-core/go.sum`. It compares
**every** changed path, including every newly added path, against that list and
returns nonzero before producing an evidence record on any violation.

For capability audit, the harness scans every added line in the complete
base-to-candidate patch and the complete candidate blob of every added file;
it does not restrict the scan to owned paths or merely print matches. It rejects
exchange connector/client imports and endpoints, signer/key/wallet imports or
providers, order-submission/execution calls, production-enable/deploy hooks,
and external egress integrations. The implementation must make the deny rules
machine-readable, versioned with the harness, and default-deny for unknown
binary/generated content. A hit is a failed evidence item; it cannot be
suppressed by a test comment or an unreviewed allowlist entry.

| Evidence ID | Automated acceptance command | Expected result |
| --- | --- | --- |
| P1-005-EV-001 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestOutboxPublisherPublishesAcceptedEnvelope$')` | Exact accepted envelope is published only to its frozen subject after an Outbox claim; PostgreSQL remains authority. |
| P1-005-EV-002 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestOutboxPublisherDuplicatePublishStableIdentity$')` | Retry preserves event ID, payload digest, subject, aggregate identity/version, and body; no second effect. |
| P1-005-EV-003 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestOutboxClaimLeaseExclusionAndExpiry$')` | Concurrent workers cannot hold a live claim; expired lease is reclaimable; stale claimant cannot mark published. |
| P1-005-EV-004 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestOutboxCrashCutsRecover$')` | All four frozen claim/publish/ack/mark crash cuts recover with no lost authoritative event and only allowed duplicate publication. |
| P1-005-EV-005 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestOutboxNeverDeletesPhaseOneBusinessEvents$')` | PUBLISHED, claimed, and pending business Outbox rows cannot be deleted, truncated, or cascaded. |
| P1-005-EV-006 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestConsumerAcksOnlyAfterInboxTransactionCommit$')` | No ACK precedes committed Inbox/effect/audit; commit then ACK gives one effect. |
| P1-005-EV-007 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestAckLossRedeliveryUsesInboxDeduplication$')` | Lost ACK after commit redelivers and finds one consumer-scoped Inbox result, one effect, and one immutable audit chain. |
| P1-005-EV-008 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestDuplicateEventIDWithChangedDigestRejectsAndAlerts$')` | Same event ID/different digest has no mutation, no successful ACK, and one bounded integrity alert/audit record. |
| P1-005-EV-009 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestPerAggregateVersionAndPreviousEventOrdering$')` | Version 1 forbids predecessor; later versions require exact predecessor; reordered/stale/gapped messages cannot mutate. |
| P1-005-EV-010 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestCrossOwnerDeliveryRejectedBeforeInboxAndEffect$')` | Wrong user/account scope is rejected before Inbox success/effect, audited, recoverable for the correct owner, and never ACKed as consumed. |
| P1-005-EV-011 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestScopeKindSubjectDigestValidationBeforeMutation$')` | Invalid scope/kind/subject binding, RFC 8785 digest, envelope version, or inner/outer identity fails before mutation. |
| P1-005-EV-012 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestTransientFailureUsesExplicitNakAndBoundedBackoff$')` | Transient failure sends explicit delayed NAK and observes the approved finite backoff without busy loop or false ACK. |
| P1-005-EV-013 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestPoisonMessageMaxDeliverAndQuarantine$')` | At max delivery, P1-004 atomically persists one non-business quarantine fact and audit before `TERM`; failed isolation emits delayed `NAK` and never `ACK`/`TERM`; no business subject is reused and healthy work continues. |
| P1-005-EV-014 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestDLQReplayUsesNormalConsumerAndDoesNotDuplicate$')` | Only a P1-004-reviewed quarantine replay authorization invokes the normal consumer path with the identical event ID/digest; scope/Inbox/CAS/audit re-run, consumer/publisher do not republish, and no duplicate/cross-owner effect occurs. |
| P1-005-EV-015 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestMessageSizeLimitRejectsBeforeDecodeAndPublish$')` | Oversized payloads are rejected both pre-publish and pre-decode, with no effect or unbounded allocation. |
| P1-005-EV-016 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestSlowConsumerBoundedPullAndBackpressure$')` | Batch/in-flight limits bound slow consumer memory; backlog/oldest-age metrics rise; events are not dropped. |
| P1-005-EV-017 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestConsumerRestartResumesDurablePosition$')` | Named durable pull consumer restart validates redelivery and preserves one application per Inbox identity. |
| P1-005-EV-018 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestReconnectAndNetworkPartitionFailClosed$')` | Publisher/consumer partition and timeout create no inferred success; recovery reconciles stable identity and preserves authoritative state. |
| P1-005-EV-019 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestCompleteOutboxReplayRebuildsDerivedJetStream$')` | Rebuild from complete PostgreSQL Outbox preserves per-aggregate chains and invents/drops no authoritative events. |
| P1-005-EV-020 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestRetentionAndRetryPolicyRequireApprovedBootstrapValues$')` | Startup rejects absent/unreviewed ack-wait, backoff, max-deliver, retention, lease, or message-size values and verifies retention covers retry/recovery policy. |
| P1-005-EV-021 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestManifestSubjectsAndNamedConsumersMatchExactly$')` | `FIT_PLATFORM_V1`, every exact subject, `PLATFORM_CONSUMER`, and `INTERNAL_NOTIFICATION_CONSUMER` match the frozen manifest. |
| P1-005-EV-022 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestNATSPermissionLeastPrivilegeMatrix$')` | Only the three runtime principals receive exact allowed actions; API has no credential or identity claims. Test-only bootstrap permits only literal CREATE/UPDATE/INFO requests for `FIT_PLATFORM_V1`, `PLATFORM_CONSUMER`, and `INTERNAL_NOTIFICATION_CONSUMER`, plus `_INBOX.fit-platform.jetstream-bootstrap.>` and one-response/five-second `allow_responses`; it rejects delete/purge/snapshot/restore/other API/`$JS.API.>`/broad `_INBOX.>`, is destroyed before runtime, and API/runtime cannot read, dial, or administer with it. |
| P1-005-EV-023 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestPermissionEscalationAndWildcardOverreachDenied$')` | `*`, `>` outside the approved protocol prefixes, foreign subjects/inboxes/ACK prefixes, publisher subscribe, consumer application publish, JetStream administration, and wrong principal all fail. |
| P1-005-EV-024 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestNotificationConsumerInternalOnlyAndIsolated$')` | Notification consumer accepts only notification filters and cannot emit Email, Push, webhook, address, destination, or external delivery. |
| P1-005-EV-025 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestSchemaCompatibilityCurrentAndPreviousOnly$')` | Current and immediately prior approved revisions work; unknown/nonadditive drift fails before mutation and emits reconciliation evidence. |
| P1-005-EV-026 | `(cd services/trading-core && go test -count=1 -race ./observability/... -run '^TestDeliveryMetricsLogsTracesAreBoundedAndRedacted$')` | Required backlog/age/retry/DLQ/wrong-owner/duplicate/reconciliation metrics exist; secrets, tokens, raw IP, bodies, and unbounded labels are absent. |
| P1-005-EV-027 | `(cd services/trading-core && go test -count=1 -race ./observability/... -run '^TestWALArchiveIncidentStateMachine$')` | Synthetic 59/60/61s, failure, repeats, clock movement, restart, and two-success recovery produce exactly one internal audit/notification per incident. |
| P1-005-EV-028 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestAllConsumerCrashAndTransactionCuts$')` | Every manifest consumer statement/commit boundary rolls back fully before commit and reconciles without duplicate effect. |
| P1-005-EV-029 | `(cd services/trading-core && go test -count=1 -race ./messaging/... -run '^TestNoAuthorityFromBrokerMessageOrCredential$')` | Client-supplied/broker user-account claims cannot select target ownership; PostgreSQL server-authored ownership remains the sole authority. |
| P1-005-EV-030 | `sh -ceu 'tmp=$(mktemp); trap "rm -f $tmp" EXIT; (cd services/trading-core && go test -json -count=1 -race ./messaging/... ./observability/...) >"$tmp"; ./infra/nats/test-harness --mode evidence-runner --phase assert-results --manifest infra/nats/evidence-manifest.json --go-json "$tmp" --ids P1-005-EV-001:P1-005-EV-029'` | Full P1-005 unit/race suite passes; every declared evidence test has one passing result and no declared evidence test is skipped. |
| P1-005-EV-031 | `./infra/nats/test-harness --mode integration --scenario publish-restart-consumer-restart-ack-loss --no-egress` | Disposable authenticated PostgreSQL/NATS process-kill and restart scenario yields one effect and immutable audit chain. |
| P1-005-EV-032 | `./infra/nats/test-harness --mode integration --scenario partition-slow-consumer-poison-dlq-replay --no-egress` | Partition/slow-consumer failures use delayed `NAK`; poison/wrong-owner/digest/order/schema terminal states obtain a P1-004 isolation record plus audit before `TERM`; failed isolation cannot terminate; controlled authorized normal-path replay preserves event ID/digest without consumer/publisher republish, duplicate, or cross-owner effect. |
| P1-005-EV-033 | `node contracts/scripts/verify-platform.mjs` | Frozen contracts, permission manifest, digest vectors, ordering, and failure semantics remain accepted without drift. |
| P1-005-EV-034 | `git diff --check "$P1_005_BASE" "$P1_005_CANDIDATE" && ./infra/nats/test-harness --mode diff-audit --audit paths --base "$P1_005_BASE" --candidate "$P1_005_CANDIDATE" --policy strict-p1-005` | Full base-to-candidate diff has no whitespace errors; every changed and newly added file mechanically satisfies the strict P1-005 path whitelist. |
| P1-005-EV-035 | `./infra/nats/test-harness --mode diff-audit --audit prohibited-capability --base "$P1_005_BASE" --candidate "$P1_005_CANDIDATE" --policy strict-p1-005 --scan-complete-added-content` | Every added line and every complete newly added file in the full base-to-candidate diff is mechanically clear of connector/executor/signer/wallet/order submission/production/deploy/external-egress capability. |

## Evidence integrity checks for this document

Run before handoff:

```bash
test "$(rg '^\| P1-005-EV-[0-9]{3} \|' coordination/design/P1-005/TEST_CHECKLIST.md | cut -d '|' -f 2 | tr -d ' ' | sort | uniq -d | wc -l | tr -d ' ')" = 0
test "$(rg '^\| P1-005-EV-[0-9]{3} \|' coordination/design/P1-005/TEST_CHECKLIST.md | cut -d '|' -f 2 | tr -d ' ' | sort -u | wc -l | tr -d ' ')" = 35
git diff --check HEAD^
test "$(git diff-tree --no-commit-id --name-only -r HEAD | sort)" = $'coordination/design/P1-005/DESIGN.md\ncoordination/design/P1-005/TEST_CHECKLIST.md'
```

Expected: zero duplicate IDs, exactly 35 IDs, no whitespace errors, and only
`coordination/design/P1-005/DESIGN.md` plus
`coordination/design/P1-005/TEST_CHECKLIST.md` in this design-only commit.
