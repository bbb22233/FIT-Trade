# H3 Evidence: 内置聊天与开仓/加仓确认 UX 设计

## Fixed inputs

- Historical execution baseline: `8fcdb82db970a1e2925b53b61c4245f9b597a2ef` (HISTORICAL_SIBLING_NOT_CURRENT_EVIDENCE — dirty worktree with 0 commits and 12 uncommitted files; NOT an ancestor of f949b0e; `git merge-base --is-ancestor` returns exit code 1)
- candidate_binding: EXTERNAL_FIXED_OBJECT_REVIEW_REQUIRED
- Isolated branch: `codex/ai-agent-swarm-bootstrap`
- Isolated worktree: `/Users/guanlan/Documents/FIT-Trade-worktrees/ai-agent-swarm-bootstrap`
- Hermes CLI: `Hermes Agent v0.19.0 (2026.7.20)`
- Verified installed profile/model: `default` / `deepseek-v4-pro`

## Controls

- No secrets were read, printed, copied, or changed.
- No signers, exchange write APIs, production models, production databases, or order writes were used.
- Only `coordination/design/AI-AGENT/H3*` and `coordination/evidence/AI-AGENT/H3*` were written.
- `services/trading-core/` and `services/hyperliquid-adapter/` were read for reference; no modifications made.

## Coverage verification

| Requirement | Coverage |
| --- | --- |
| Built-in chat UX | Sections 2.1–2.7: layout, message classification, mode badges, missing-field flow, context, quick commands |
| Mock-data labeling | Section 2.4: SIMULATION/HYPOTHETICAL mode badges, DEV-API marker, SIMULATED_ORDER_ prefix, orange indicator bars |
| Open/add-position confirmation UX | Sections 3.1–3.7: confirmation card lifecycle, component structure, field layout, button spec, execution progress tracker |
| Explicit human confirmation | Sections 3.4–3.5: button wording with action+environment+symbol, button locking, execution progress display |
| Stop Market protection visibility | Sections 4.1–4.4: independent section in confirmation card, protection state→UX mapping, failure escalation, emergency exit path |
| Cancellation/timeout | Sections 3.6–3.7: expiry countdown, stale state, version/snapshot invalidation, re-generation requirement |
| Idempotency/duplicate submission resistance | Sections 5.1–5.4: client-side button lock, confirmation_nonce, planned compare-and-consume dedup (NOT_IMPLEMENTED — existing IdempotencyStore only handles client_order_id, not confirmation_id), client_order_id conflict handling (implemented), reconnection reconciliation (planned, depends on compare-and-consume) |
| Server-side atomic confirmation compare-and-consume semantics | Section 6: 6-context binding (user/account/device/session/purpose/intent_hash), atomic compare-and-consume algorithm, confirmation_id/nonce persistent uniqueness, 11 fail-closed scenarios, UNKNOWN_REQUIRES_RECONCILIATION bridge, IdempotencyStore gap explicitly marked NOT_IMPLEMENTED/NOT_EXECUTED |
| UNKNOWN_REQUIRES_RECONCILIATION | Sections 6.1–6.4 → Sections 7.1–7.4 (renumbered): trigger scenarios, global alert UX, operation detail page, resolution paths, MANUAL_RECONCILIATION flow |

## Design decision rationale

All 16 design decisions in H3-D-001 through H3-D-016 are traced to existing decision records (`docs/05_DECISION_REGISTER.md`), UX spec requirements (`docs/06_FRONTEND_UX_SPEC.md`), or domain contracts (`services/trading-core/domain/`). The 6 new decisions (H3-D-011 through H3-D-016) define server-side atomic compare-and-consume confirmation semantics and explicitly mark the implementation gap with IdempotencyStore. No new architectural or safety assumptions were introduced.

## Interface dependencies verified

All referenced contracts, state machines, and components were verified to exist at the specified paths:

- `ConfirmationTicket` and `TradeIntent` types: verified at `services/trading-core/domain/types.go`
- `OperationState` and `ProtectionState` state machines: verified at `services/trading-core/domain/state.go`
- `IdempotencyStore` and dispatch outcomes: verified at `services/trading-core/domain/idempotency.go`
- Frozen failure scenarios: verified at `services/trading-core/domain/scenarios.go`
- Confirmation hash binding (Python): verified at `services/hermes-agent/src/hermes_agent/confirmation.py`
- Pydantic contracts (Python): verified at `services/hermes-agent/src/hermes_agent/contracts.py`
- Confirmation compare-and-consume storage: **NOT_IMPLEMENTED** — `services/trading-core/domain/confirmation_consume.go` does not exist; no `ConfirmationRecord`, `ConfirmationNonce`, or `CompareAndConsume()` method exists anywhere in the codebase
- Confirmation nonce index: **NOT_IMPLEMENTED** — `services/trading-core/domain/confirmation_nonce.go` does not exist
- Existing IdempotencyStore (`services/trading-core/domain/idempotency.go`): verified to contain only `client_order_id`-based order-level dedup; no confirmation_id key, no compare-and-consume implementation

## Standard worker result package

```text
TASK_ID: t_64632ac8
ROLE: AI-AGENT-FIX-R2 R-H3 — 原子确认消费修复（compare-and-consume semantics repair）
STATUS: REPAIR_COMPLETE
ALLOWED_PATHS: coordination/design/AI-AGENT/H3*, coordination/evidence/AI-AGENT/H3*
FILES_WRITTEN:
  - coordination/design/AI-AGENT/H3_BUILT_IN_CHAT_AND_CONFIRMATION_UX.md
  - coordination/evidence/AI-AGENT/H3_EVIDENCE.md
CHECKS_RUN:
  - Coverage checklist: 9/9 requirements met
  - Interface verifications: 6/6 existing contract sources confirmed; 3/3 NOT_IMPLEMENTED gaps documented
  - Design decisions: 16/16 traced to existing decisions/specs
ROUTING_PROFILE_AND_MODEL: default / deepseek-v4-pro
UNRESOLVED_RISKS:
  - 服务端 compare-and-consume 原子确认消费路径未实现（NOT_IMPLEMENTED/NOT_EXECUTED），需独立于 IdempotencyStore 开发
  - ConfirmationRecord、ConfirmationNonce、CompareAndConsume() 三个组件需在后续开发阶段创建
  - nonce 重放检测、并发消费冲突处理、持久化结果未知的 UNKNOWN 桥接均仅停留在设计规范层面
INTEGRATION_DEPENDENCIES:
  - H1: 多模型设计（Hermes 模型路由和 fallback 影响聊天面板中的模型错误显示）
  - H2: 学习与评估设计（retrospective review flow 与聊天中的反馈收集流程交叉）
  - H4: 安全测试计划（prompt injection、确认绕过、重复订单等场景需 H3 确认卡流程作为测试目标）
  - Verifier: 独立范围检查和安全验证
  - Synthesizer: 汇总所有 worker 结果
MERGE=false
DEPLOY=false
PRODUCTION=false
LIVE_TRADING=false
```
