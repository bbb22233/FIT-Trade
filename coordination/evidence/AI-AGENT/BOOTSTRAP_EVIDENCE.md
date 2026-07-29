# AI-AGENT-SWARM-BOOTSTRAP Evidence

**candidate_binding**: EXTERNAL_FIXED_OBJECT_REVIEW_REQUIRED — this file cannot self-certify any commit containing itself as the current evidence baseline.

## Fixed inputs

- Historical bootstrap checkout base (not current evidence): `60850b69dd06c46bbe2a9cfab19d27cdb2ef0412` — **HISTORICAL_BOOTSTRAP_CHECKOUT_BASE_NOT_CURRENT_EVIDENCE**. This was the repository HEAD at the time the AI-AGENT worktree was first created. It is not the current repository base, not a fixed object, and not current evidence for any verification gate.
- Isolated branch: `codex/ai-agent-swarm-bootstrap`
- Isolated worktree: `/Users/guanlan/Documents/FIT-Trade-worktrees/ai-agent-swarm-bootstrap`
- Hermes CLI: `Hermes Agent v0.19.0 (2026.7.20)`
- Verified installed profile/model: `default` / `deepseek-v4-pro`

## Bootstrap controls

- No secrets were read, printed, copied, or changed.
- No `--yolo`, no automatic subagent approval, no live wallet/signer/exchange write API/production model/production database/order write was used.
- The Kanban workers are design-only. Their task instructions must restrict writes to the two `coordination/**/AI-AGENT/**` allowlists in this worktree.
- Dispatcher execution is deferred until each generated card has the scope instruction attached and the coordinator explicitly asks for runtime execution. A dry-run may be used to validate scheduling only.

## Standard worker result package

```text
TASK_ID:
ROLE:
STATUS:
ALLOWED_PATHS:
FILES_WRITTEN:
CHECKS_RUN:
ROUTING_PROFILE_AND_MODEL:
UNRESOLVED_RISKS:
INTEGRATION_DEPENDENCIES:
MERGE=false
DEPLOY=false
PRODUCTION=false
LIVE_TRADING=false
```

## Expected verification evidence

- Kanban graph: parent → H1/H2/H3/H4 → verifier → synthesizer.
- Each task's full prompt includes the allowlist and prohibited systems.
- `hermes kanban dispatch --dry-run` contains only the six known design roles, if scheduling is requested.
- Any model override absence is recorded as an environment limitation, not silently substituted.
