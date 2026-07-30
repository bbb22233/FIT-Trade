# H1 设计证据摘要（修订版 R-H1）

## R3 current-evidence boundary

The R2 baseline is a historical execution artifact, not current candidate
evidence: it is not an ancestor of the source candidate, and its dirty-worktree
observations cannot bind a current fixed candidate. The source candidate has
direct parent 60850b69dd06c46bbe2a9cfab19d27cdb2ef0412 and 15 added AI-AGENT
files; this R3 worktree is uncommitted and therefore also cannot self-certify
its fixed identity. candidate_binding=EXTERNAL_FIXED_OBJECT_REVIEW_REQUIRED.

For H1, absent a complete auditable non-secret runtime capability receipt in
the current discovery epoch, every provider/model — including Anthropic and
Google — is **UNVERIFIED** and must enter mandatory discovery. Only a complete
current-epoch runtime receipt may establish **UNAVAILABLE**.

## 基线

- 仓库基线 commit：`8fcdb82db970a1e2925b53b61c4245f9b597a2ef`
- 隔离分支：`codex/ai-agent-swarm-bootstrap`
- 隔离工作树：`/Users/guanlan/Documents/FIT-Trade-worktrees/ai-agent-swarm-bootstrap`
- Hermes CLI：`Hermes Agent v0.19.0 (2026.7.20)`
- 已验证 profile/model：`default` / `deepseek-v4-pro`（当前会话环境，非文档验证）
- 设计文件路径：`coordination/design/AI-AGENT/H1_MULTI_MODEL_ADAPTER_DESIGN.md`
- 修订任务：`t_6dc220db`（AI-AGENT-FIX-R2 R-H1）

## 输入来源

| 来源 | 关键信息 | 验证方式 |
| --- | --- | --- |
| Hermes Agent Memory（持久记忆） | deepseek-v4-pro 特性：推理模型、max_tokens≥4096、无 response_format json_object、content 可能为空需读 reasoning_content | 本 session 已验证（当前模型即 deepseek-v4-pro） |
| Hermes Agent Memory | OpenRouter Provider Allowlist 的历史 403/可用观察 | 历史 session 观察；不是当前纪元运行时能力收据，不能建立 VERIFIED 或 UNAVAILABLE |
| Hermes Agent Memory | Codex v0.145.0 OAuth 凭证独立于 Hermes Agent、路径不共享 | 环境隔离事实 |
| Hermes Agent Memory | Claude Code v2.1.220 通过 OpenRouter 使用 llama-4-maverick + mistral-large | 历史 session 已验证 |
| `docs/02_SYSTEM_ARCHITECTURE.md` | 系统架构定义 MCP 工具网关、信任边界、模型层位置 | 文件内容审查 |
| `docs/05_DECISION_REGISTER.md` | D-006: Agent 使用 Hermes、支持多个模型和 API 提供商；D-041: Hermes 输出结构化信号、Go 完成确定性风控 | 文件内容审查 |
| `AGENTS.md` | 产品边界：当前阶段仅开发与模拟；Hermes 文本不能直接签名/提交/授权订单 | 文件内容审查 |
| `services/hermes-agent/src/` | contracts.py 严格 Pydantic v2 验证、mcp_tools.py MCP 工具清单不变性检查、confirmation.py RFC 8785 哈希绑定 | 代码审查 |
| `coordination/design/AI-AGENT/SWARM_CHARTER.md` | H1 交付物定义：Hermes 主路由、DeepSeek V4 Pro、OpenAI-compatible 兼容层、显式 fallback 条件、模型能力/不可用时 fail-closed | 文件内容审查 |
| `coordination/evidence/AI-AGENT/BOOTSTRAP_EVIDENCE.md` | 标准结果包模板；禁止值：MERGE/DEPLOY/PRODUCTION/LIVE_TRADING=false | 文件内容审查 |

## 修订内容证据

### R-H1-001：模型矩阵重构 — VERIFIED/UNVERIFIED 区分

**修订前问题**：设计文档 §2 将 memory 中的观察标记为"已验证（主路由）"和"已验证（可回退）"，但缺少结构化验证证据（timestamp、evidence_hash、failure_class 等）。

**修订后**：
- 定义 VERIFIED 条目的 7 项必需字段：timestamp、provider、model、capability、result、failure_class、evidence_hash
- 全部条目标为 UNVERIFIED（本文档为文档阶段，未执行实时发现）
- Anthropic/Google 与其他候选一样为 UNVERIFIED；历史 Provider Allowlist 观察不构成当前纪元收据，不能排除其强制发现
- §2.3 路由规则强制：仅 VERIFIED 条目可用于路由决策

**证据**：
- SWARM_CHARTER 明确："本轮不猜测、不持久化修改 profile"
- BOOTSTRAP_EVIDENCE 明确："任何模型 override 缺席记录为环境限制、不是静默替换"
- Hermes Agent Memory 提供的模型观察信息不包含 timestamp/evidence_hash，不满足 VERIFIED 条件

### R-H1-002：操作错误分类体系

**修订前问题**：设计文档 §4.1 的 fallback 触发条件使用了简化的条件列表（HTTP 错误/超时/空 content），未覆盖认证失败、提供商拒绝等关键场景，且缺少错误分类的规范定义。

**修订后**：
- 新增 §5 操作错误分类体系，定义 7 个错误类：AUTH_FAILED、PROVIDER_DENIED、RATE_LIMITED、TRANSIENT_TRANSPORT、TIMEOUT、INVALID_RESPONSE、UNKNOWN
- 每类定义：回退资格、NO_TRADE 行为、有界重试预算、退避策略、对账要求、已脱敏审计字段（JSON Schema）
- §5.9 分类汇总矩阵：一目了然的属性对比
- §4.1 回退触发条件更新为引用 §5 错误分类

**关键安全决策**：
- AUTH_FAILED → 不回退（认证凭据问题不会因换模型而解决）
- PROVIDER_DENIED → 不回退（提供商层面拒绝）
- UNKNOWN → 不回退（未知=不安全）
- 仅 TRANSIENT_TRANSPORT、RATE_LIMITED、TIMEOUT、INVALID_RESPONSE 有回退资格

### R-H1-003：强制运行时能力发现

**修订前问题**：设计文档 §5 描述了能力发现流程，但未明确其为强制性，且未定义"未执行发现"时的 fail-closed 行为。

**修订后**：
- §6.1 新增"强制运行时发现（Mandatory Startup Discovery）"：启动时强制运行，不可跳过
- 无 VERIFIED 条目 = fail-closed，不静默降级
- 发现完成前禁止路由
- §6.4 过期能力处理（Stale Capability Prevention）：TTL 过期自动重验证，10s 超时保护
- §6.5 明确声明：本文档为文档阶段，未执行实时发现

### R-H1-004：接口增强

**修订前**：Adapter 接口定义中无 ErrorClass 枚举，AdapterCapability 无 verified_entry 字段。

**修订后**：
- §3.1 新增 `ErrorClass` 枚举（7 类）
- 新增 `VerifiedEntry` dataclass（7 字段）
- `AdapterCapability` 新增 `verified_entry: VerifiedEntry | None` 字段
- `ModelAdapter.invoke()` 文档明确异常必须含 ErrorClass 分类

### R-H1-005：测试策略与配置增强

**修订后**：
- §9 测试策略新增 ErrorClass 测试、强制发现测试、审计字段测试
- §8.3 配置文件新增 `error_taxonomy` 参数段
- §8.1 包结构调整：新增 `errors/taxonomy.py`

## 设计覆盖度检查

| 检查项 | 覆盖 | 位置 |
| --- | --- | --- |
| VERIFIED 7 项必需字段定义 | ✅ | §2.1 |
| 所有模型条目标记 UNVERIFIED | ✅ | §2.2 |
| UNVERIFIED 禁止路由决策 | ✅ | §2.3 |
| 7 类操作错误全覆盖 | ✅ | §5.1–§5.9 |
| 每类含回退资格定义 | ✅ | §5.2–§5.8 |
| 每类含 NO_TRADE 行为 | ✅ | §5.2–§5.8 |
| 每类含有界重试预算 | ✅ | §5.2–§5.8 |
| 每类含对账要求 | ✅ | §5.2–§5.8 |
| 每类含已脱敏审计字段 | ✅ | §5.2–§5.8 |
| 强制运行时启动发现 | ✅ | §6.1 |
| Fail-closed 原则 | ✅ | §6.2 |
| 无 VERIFIED = NO_TRADE | ✅ | §6.1, §6.3 |
| 未声称本文档执行过发现 | ✅ | §2.2 banner, §6.5 |
| ErrorClass 枚举在接口中 | ✅ | §3.1 |
| VerifiedEntry dataclass | ✅ | §3.1 |
| SWARM_CHARTER H1 需求 7/7 | ✅ | 继承自原版，未退化 |
| 安全边界 B1-B14 无变更 | ✅ | §7（继承自原版） |
| 授权标志全 false | ✅ | §10 |

## R-H1 修订特有检查

| 检查项 | 结果 |
| --- | --- |
| 所有模型条目状态 | UNVERIFIED（条目 1-7）；当前纪元完整运行时收据才可建立 VERIFIED 或 UNAVAILABLE |
| 已脱敏审计字段 | 无 key 值、token 值、完整凭据出现 |
| 错误分类覆盖 | AUTH_FAILED, PROVIDER_DENIED, RATE_LIMITED, TRANSIENT_TRANSPORT, TIMEOUT, INVALID_RESPONSE, UNKNOWN = 7 类 |
| 分类汇总矩阵 | §5.9 6 属性 × 7 类 = 42 格完整 |
| 不重试分类 | AUTH_FAILED(0), PROVIDER_DENIED(0), UNKNOWN(0) = 3 类 |
| 重试分类 | RATE_LIMITED(3), TRANSIENT_TRANSPORT(3), TIMEOUT(2), INVALID_RESPONSE(2) = 4 类 |
| NO_TRADE 分类 | 立即：AUTH_FAILED, PROVIDER_DENIED, UNKNOWN；回退耗尽后：RATE_LIMITED, TRANSIENT_TRANSPORT, TIMEOUT, INVALID_RESPONSE |
| 未修改文件 | 仅修改 H1_DESIGN.md 和 H1_DESIGN_EVIDENCE.md，未触及任何其他文件 |
| 无凭据泄露 | grep -iE '(secret|token|key|password|credential|private)' 命中均为安全约束声明和审计字段 schema（已脱敏） |

## 不适用事项

- 本文档不包含代码实现（设计阶段）
- 本文档不直接调用模型 API（不越过禁止操作名单）
- 不包含交换所写 API、签名、订单或实盘钱包操作
- 未读取任何凭据、私钥、token
- 所有模型"已验证"声明已替换为"UNVERIFIED — 待运行时验证"
- 未声称文档编写过程中执行了实时能力发现

## 残留风险

1. **运行时能力未验证**：所有模型条目为 UNVERIFIED。文档规定的强制发现流程仅在实现并运行后才能获得 VERIFIED 条目
2. **ErrorClass 与 Go 层对齐**：§5 定义的错误类尚未与 Go 风控核心的错误码对齐，需实现阶段协调
3. **UNKNOWN 类过于保守**：立即 NO_TRADE + 不回退的策略在边缘情况下可能阻断合法操作，需在生产模拟中观察
4. **审计字段脱敏范围**：已定义脱敏字段结构，但实现时需确保 log 框架不会意外记录完整 payload
5. **回退链顺序**：设计基于 memory 观察的优先级（deepseek → llama → mistral），但运行时发现可能改变此顺序（所有模型均 VERIFIED 后按能力/延迟重排）

## 标准结果包（证据版本）

```
TASK_ID:         t_6dc220db
ROLE:            AI-AGENT-FIX-R2 R-H1 — 模型能力与失败分类修复
BASELINE:        8fcdb82db970a1e2925b53b61c4245f9b597a2ef
STATUS:          完成（设计修复+证据）
FILES_MODIFIED:
  - coordination/design/AI-AGENT/H1_MULTI_MODEL_ADAPTER_DESIGN.md
  - coordination/evidence/AI-AGENT/H1_DESIGN_EVIDENCE.md
CHECKS_RUN:
  - SWARM_CHARTER H1 需求 7/7 覆盖（继承自原版，未退化）
  - R-H1 修订检查 10/10 通过
  - 无凭据泄漏检查通过
  - 授权标志全部 false
  - 未修改其他文件
KEY_FINDINGS:
  - 修订前所有模型条目缺少结构化证据字段（timestamp/evidence_hash/failure_class），现已全部修正
  - 新增 7 类操作错误分类体系，每类含回退资格、重试预算、审计字段
  - 新增强制运行时启动发现规范，fail-closed 行为明确
  - UNVERIFIED 条目明确禁止路由决策
UNRESOLVED_RISKS:
  - 运行时能力未验证（文档阶段限制）
  - ErrorClass 与 Go 层错误码对齐待实现阶段
  - UNKNOWN 类 fail-closed 策略可能过于保守
  - 审计日志脱敏实现需验证
INTEGRATION_DEPENDENCIES:
  - services/hermes-agent/ 现有 contracts/validator/mcp_tools 管线
  - 外部 OpenRouter API（需运行时验证）
  - Go 风控核心（确定性的下游消费方）
  - ErrorClass 枚举需在实现时与 Go 层错误码对齐
MERGE=false
DEPLOY=false
PRODUCTION=false
LIVE_TRADING=false
```
