# H1 设计证据摘要（修订版 R-H1，FIX-R3 纪元修复）

## 基线

- 历史快照 commit：`f949b0e47a1b911a9a8bfebf117df7a49af0c837`（HISTORICAL_PRE_AMEND_SNAPSHOT_NOT_CURRENT_EVIDENCE — 此证据文件本身位于该历史快照中，不能自签名为当前仓库基线。当前固定候选身份由外部审查员从 Git 对象确定，candidate_binding=EXTERNAL_FIXED_OBJECT_REVIEW_REQUIRED。）
- 隔离分支：`codex/ai-agent-swarm-bootstrap`
- 隔离工作树：`/Users/guanlan/Documents/FIT-Trade-worktrees/ai-agent-swarm-bootstrap`
- Hermes CLI：`Hermes Agent v0.19.0 (2026.7.20)`
- 已验证 profile/model：`default` / `deepseek-v4-pro`（当前会话环境，非文档验证）
- 设计文件路径：`coordination/design/AI-AGENT/H1_MULTI_MODEL_ADAPTER_DESIGN.md`
- 前序修订任务：`t_6dc220db`（AI-AGENT-FIX-R2 R-H1）
- 当前修订任务：`t_7753f6ae`（AI-AGENT-FIX-R3 R-H1 收据与纪元修复）

## 输入来源

| 来源 | 关键信息 | 验证方式 |
| --- | --- | --- |
| Hermes Agent Memory（持久记忆） | deepseek-v4-pro 特性：推理模型、max_tokens≥4096、无 response_format json_object、content 可能为空需读 reasoning_content | 本 session 已验证（当前模型即 deepseek-v4-pro） |
| Hermes Agent Memory | OpenRouter Provider Allowlist：Anthropic/OpenAI/Google→403、Meta/DeepSeek/Mistral 可用 | 历史 session 已验证 |
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
- 全部条目标为 UNVERIFIED（本文档为文档阶段，未执行实时发现；任何条目 absent a complete auditable capability receipt 均为 UNVERIFIED）
- Anthropic/Google 条目与其余条目同等对待：历史 Provider Allowlist 观察不构成当前纪元运行时能力收据，不可作为排除于强制发现的依据。仅运行时产生收据（含 failure_class=PROVIDER_DENIED）后方可标记 UNAVAILABLE
- §2.3 路由规则强制：仅 VERIFIED 条目可用于路由决策；历史观察/静态配置不得排除任何 provider 于强制发现

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

### R-H1-006：FIX-R3 能力收据与发现纪元修复

**修订前问题**（FIX-R2 残留）：设计文档中 Anthropic/Google 标记为 UNAVAILABLE，依据是历史 Provider Allowlist 观察。§6.1 和 §6.3 的发现流程排除了 UNAVAILABLE 条目。UNAVAILABLE 定义未与发现纪元绑定，存在跨纪元"永久排除"的语义风险。缺少过期收据处理和跨纪元禁止传递的显式规范。

**修订后**：
- §2.1 状态定义重构：VERIFIED/UNVERIFIED/UNAVAILABLE 三态均绑定"当前发现纪元+完整运行时能力收据"
- §2.2：Anthropic（条目 6）和 Google（条目 7）从 UNAVAILABLE 改为 UNVERIFIED。缺失字段列改为"完整收据"，说明栏明确"历史 Allowlist 观察不可成为排除于强制发现的理由"
- §2.3 路由规则新增第 4/5/7 条：UNAVAILABLE 仅当前纪元收据有效且不跨纪元传承；历史 allowlist/静态配置不得排除任何 provider 于强制发现；历史"被拦截"记忆不可作为当前不可用证据
- §4.4 回退约束：历史 Provider Allowlist 观察不可排除 provider 于回退尝试
- §6.1 启动发现：从"所有非 UNAVAILABLE 候选模型"改为"所有 UNVERIFIED 候选模型"
- §6.3 发现流程：补充 gpt-5.6-sol/terra、Anthropic、Google 进入流程；统一所有模型的失败处理为 UNAVAILABLE(当前纪元)（移除 llama/mistral 失败→UNVERIFIED 的不一致）
- §6.4 重定义为"过期/失效收据处理"：新增 UNAVAILABLE 过期回归 UNVERIFIED、跨纪元禁止传递（上一纪元 UNAVAILABLE 不得作为当前纪元排除依据）、收据存档（已脱敏，审计用途但不替代当前纪元验证）
- §6.5 移除"Anthropic/Google 不期望未来变化"断言，改为"由运行时收据决定当前纪元状态，不预判"

**关键决策**：
- UNAVAILABLE 必须来自当前纪元运行时完整收据，历史观察不满足条件
- 每个发现纪元独立，所有 UNVERIFIED provider（含曾 UNAVAILABLE 的）均进入强制发现
- 收据过期（TTL 300s）→ UNVERIFIED → 重新进入发现流程
- 这确保系统不会因静态配置或过时观察而永久排除任何 provider

## 设计覆盖度检查

| 检查项 | 覆盖 | 位置 |
| --- | --- | --- |
| VERIFIED 7 项必需字段定义 | ✅ | §2.1 |
| 所有模型条目标记 UNVERIFIED | ✅ | §2.2 |
| UNVERIFIED 禁止路由决策 | ✅ | §2.3 |
| 所有 UNVERIFIED 进入强制发现 | ✅ | §2.3.3, §6.1, §6.3 |
| UNAVAILABLE 仅当前纪元收据有效 | ✅ | §2.1, §2.3.4 |
| 历史观察禁止作为排除依据 | ✅ | §2.3.5, §6.1, §6.5 |
| 过期收据处理（VERIFIED + UNAVAILABLE） | ✅ | §6.4 |
| 跨纪元禁止传递 | ✅ | §6.4.5 |
| 收据存档（审计，不替代当前纪元验证） | ✅ | §6.4.6 |
| Anthropic/Google 纳入强制发现 | ✅ | §2.2(条目6-7), §6.3 |
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
| 所有模型条目状态 | UNVERIFIED（条目 1-7，包括 Anthropic/Google） |
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
2. **Anthropic/Google 的运行时行为未确认**：历史上被 Provider Allowlist 拦截（403），运行时很可能仍是 PROVIDER_DENIED，但仅在产生当前纪元收据后确认为 UNAVAILABLE
3. **ErrorClass 与 Go 层对齐**：§5 定义的错误类尚未与 Go 风控核心的错误码对齐，需实现阶段协调
4. **UNKNOWN 类过于保守**：立即 NO_TRADE + 不回退的策略在边缘情况下可能阻断合法操作，需在生产模拟中观察
5. **审计字段脱敏范围**：已定义脱敏字段结构，但实现时需确保 log 框架不会意外记录完整 payload
6. **回退链顺序**：设计基于 memory 观察的优先级（deepseek → llama → mistral），但运行时发现可能改变此顺序（所有模型均 VERIFIED 后按能力/延迟重排）
7. **全量发现开销**：每个发现纪元对所有 UNVERIFIED provider（含历史上被 Allowlist 拦截的）执行 API 调用，需实现时评估开销和限速策略

## 标准结果包（证据版本）

```
TASK_ID:         t_7753f6ae
ROLE:            AI-AGENT-FIX-R3 R-H1 — 能力收据与发现纪元修复
BASELINE:        f949b0e47a1b911a9a8bfebf117df7a49af0c837  (HISTORICAL_PRE_AMEND_SNAPSHOT_NOT_CURRENT_EVIDENCE — 当前固定候选身份为 EXTERNAL_FIXED_OBJECT_REVIEW_REQUIRED)
STATUS:          完成（设计修复+证据）
FILES_MODIFIED:
  - coordination/design/AI-AGENT/H1_MULTI_MODEL_ADAPTER_DESIGN.md
  - coordination/evidence/AI-AGENT/H1_DESIGN_EVIDENCE.md
CHECKS_RUN:
  - SWARM_CHARTER H1 需求 7/7 覆盖（继承自原版+FIX-R2，未退化）
  - R-H1 FIX-R2 修订检查 10/10 通过（继承）
  - R-H1 FIX-R3 修订检查 8/8 通过（新增）
  - 无凭据泄漏检查通过
  - 授权标志全部 false
  - 未修改其他文件
KEY_FINDINGS:
  - FIX-R3 修复了 FIX-R2 残留问题：Anthropic/Google 基于历史 Allowlist 观察被永久排除于发现范围
  - 三态（VERIFIED/UNVERIFIED/UNAVAILABLE）全部绑定"当前发现纪元+完整运行时能力收据"
  - UNAVAILABLE 仅由当前纪元运行时收据产生，不可跨纪元传承
  - 历史观察、静态配置、文档记忆均不得排除任何 provider 于强制发现
  - 所有 7 个模型条目（含 Anthropic/Google）现在均为 UNVERIFIED，均进入强制启动发现
  - 过期收据处理覆盖 VERIFIED 和 UNAVAILABLE 两种收据类型
  - §6.3 发现流程补充了 gpt-5.6-sol/terra、Anthropic、Google
UNRESOLVED_RISKS:
  - 运行时能力未验证（文档阶段限制）
  - Anthropic/Google 运行时很可能仍是 PROVIDER_DENIED，但需运行时收据确认
  - ErrorClass 与 Go 层错误码对齐待实现阶段
  - UNKNOWN 类 fail-closed 策略可能过于保守
  - 审计日志脱敏实现需验证
  - 每个纪元全量发现的开销（含被 Allowlist 拦截的 provider）需评估
INTEGRATION_DEPENDENCIES:
  - services/hermes-agent/ 现有 contracts/validator/mcp_tools 管线
  - 外部 OpenRouter API（需运行时验证，含 Anthropic/Google 端点）
  - Go 风控核心（确定性的下游消费方）
  - ErrorClass 枚举需在实现时与 Go 层错误码对齐
MERGE=false
DEPLOY=false
PRODUCTION=false
LIVE_TRADING=false
```
