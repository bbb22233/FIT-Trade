# Repository Agent Rules

## Product boundary

This repository builds the Hyperliquid Hermes personal trading system. The
current phase is development and simulation only.

- Do not connect a live wallet, signer, exchange write API, or production model.
- Do not deploy, enable automatic trading, or submit a live order without a
  separate explicit authorization.
- Never commit or print credentials, private keys, tokens, cookies, pairing
  values, or recovery material.
- Preserve the approved BTC, ETH, SOL perpetual-only scope and the mandatory
  confirmation/protection semantics documented in `docs/`.

## Coordination model

The local Codex task is the integration coordinator and owns:

- repository-wide files and shared configuration;
- `contracts/`, `apps/web/`, `docs/`, and `coordination/`;
- integration order, fixed-commit review, and final acceptance.

The local Hermes development profile owns:

- `services/hermes-agent/`;
- model routing, prompts, tool adapters, trading-intent translation, and
  evaluation scenarios.

The server Codex development account owns:

- `services/trading-core/`;
- `services/hyperliquid-adapter/`;
- `infra/`, Linux integration tests, PostgreSQL, and NATS setup.

Only the coordinator may change shared contracts or high-coupling root files.
If another agent needs such a change, it must propose the change in its handoff
instead of editing the file.

## Git and task discipline

- One task has one owner, one branch, one worktree, and a bounded file set.
- Use `codex/<task-id>-<slug>` branches unless a task packet specifies another
  branch.
- Do not push directly to `main`.
- Every implementation handoff must provide the exact commit, changed paths,
  tests run, test results, unresolved risks, and integration dependencies.
- A green test result does not authorize merge, deployment, production, or live
  trading.
- Preserve user changes and unrelated worktree content.

## Required implementation order

1. Freeze shared schemas and behavioral contracts.
2. Generate or implement clients from the frozen contracts.
3. Develop disjoint services in parallel.
4. Run contract, unit, integration, failure-path, and UI checks.
5. Perform independent fixed-commit review.
6. Integrate only in the dependency order stated in the task packets.

## Reliability requirements

- Trading state is server-authored and idempotent.
- Unknown execution results fail closed and require reconciliation.
- Stop Market protection is mandatory for every position.
- Protection failure may trigger only the documented `reduce-only` emergency
  exit path.
- Hermes text cannot directly sign, submit, or authorize an order.
- Development mocks must be visibly distinguishable from real exchange data.
