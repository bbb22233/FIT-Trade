# H1: 多模型与 API Adapter 设计

## 状态与边界

- 阶段：设计文档（非实现）
- 工作树：`/Users/guanlan/Documents/FIT-Trade-worktrees/ai-agent-swarm-bootstrap`
- 分支：`codex/ai-agent-swarm-bootstrap`
- 历史快照 commit：`f949b0e47a1b911a9a8bfebf117df7a49af0c837`（HISTORICAL_PRE_AMEND_SNAPSHOT_NOT_CURRENT_EVIDENCE — 本文档编写时的历史快照。此文件本身位于该 commit 中，因此不能自引用为当前候选。当前固定候选身份由外部审查员从 Git 对象确定，candidate_binding=EXTERNAL_FIXED_OBJECT_REVIEW_REQUIRED。）
- 修订任务：`t_6dc220db`（FIX-R2 R-H1 前序修订）、`t_7753f6ae`（FIX-R3 R-H1 收据与纪元修复）
- 允许写入：`coordination/design/AI-AGENT/**`、`coordination/evidence/AI-AGENT/**`
- 禁止：凭据、私钥、token、cookie、signer、交易所写 API、订单、实盘钱包、生产数据库、`services/trading-core/`、`services/hyperliquid-adapter/`、`core` auth/trading code
- 安全约束：Hermes 文本不得直接签名、提交或授权订单
- ⚠️ **本文档编写于文档阶段，未执行实时能力发现。第 2 节所有模型声明均为 UNVERIFIED，具体路由行为须在运行时通过强制能力发现（第 6 节）确定。**

---

## 1. 架构概览

```
┌─────────────────────────────────────────────────────────┐
│                   Hermes Agent Runtime                   │
│            (profile: default, v0.19.0)                   │
│         trading-intent translation + tool routing        │
└─────────────────────┬───────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────┐
│                Model Adapter Boundary                    │
│                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │   DeepSeek   │  │  Hermes      │  │ OpenAI-       │  │
│  │   Adapter    │  │  Native      │  │ Compatible    │  │
│  │              │  │  Adapter     │  │ Adapter       │  │
│  └──────┬───────┘  └──────┬───────┘  └──────┬────────┘  │
│         │                 │                  │           │
│  ┌──────┴─────────────────┴──────────────────┴────────┐ │
│  │     Capability Discovery + Error Classification     │ │
│  │  (mandatory at startup; fail-closed; never guess)   │ │
│  └──────────────────────┬─────────────────────────────┘ │
└─────────────────────────┼───────────────────────────────┘
                          │
                          ▼
              ┌───────────────────────┐
              │    Model API Layer    │
              │  (OpenRouter / OAuth) │
              └───────────────────────┘
```

- **Hermes Agent Runtime**：聊天编排、意图翻译、工具调用。不直接调用模型 API。
- **Model Adapter Boundary**：三个适配器 + 能力发现 + 错误分类 + 回退逻辑。这是本文档的设计核心。
- **Model API Layer**：外部模型提供商。受 Provider Allowlist 限制。

---

## 2. 模型能力声明矩阵

### 2.1 VERIFIED / UNVERIFIED 定义

VERIFIED 条目必须包含以下**全部 7 项**结构化字段，缺一不可：

| # | 字段 | 类型 | 说明 |
| --- | --- | --- | --- |
| 1 | `timestamp` | UTC ISO-8601 字符串 | 验证发生的时间戳（不得为秘密信息） |
| 2 | `provider` | 字符串 | 模型提供商（如 DeepSeek、Meta、Mistral） |
| 3 | `model` | 字符串 | 完整模型 ID（如 deepseek-v4-pro） |
| 4 | `capability` | 字符串数组 | 已验证的能力列表（如 CHAT, FUNCTION_CALLING, REASONING） |
| 5 | `result` | 对象 | 验证结果（HTTP 状态码、延迟、token 消耗） |
| 6 | `failure_class` | 字符串 | 失败时的错误分类（成功则为 NONE），使用第 5 节定义的分类体系 |
| 7 | `evidence_hash` | SHA-256 十六进制字符串 | 验证过程中所有输入输出的 SHA-256 哈希（可重现证据） |

| 状态 | 含义 | 路由资格 |
| --- | --- | --- |
| **VERIFIED** | 持有当前发现纪元的完整运行时能力收据（7 项必需字段全部由运行时 API 调用填充，非文档声明） | 可用于路由决策 |
| **UNVERIFIED** | 缺少当前发现纪元的完整运行时能力收据；文档声明、历史记忆、推测、静态配置、过期收据均属于此状态 | **禁止用于路由决策；必须进入强制启动发现** |
| **UNAVAILABLE** | 持有当前发现纪元的完整运行时能力收据，收据中 result 字段证明该能力不可用/被拒绝 | 排除于当前纪元，禁止路由和重试；下一发现纪元必须重新验证 |

### 2.2 当前模型声明（文档级观察，非运行时验证）

> ⚠️ **本文档编写于文档阶段，未执行实时能力发现。以下所有条目均缺少完整运行时能力收据（缺少 timestamp、result、failure_class、evidence_hash 四个由运行时 API 调用方可填充的必需字段；provider、model、capability 字段虽有文档级记录但同样非运行时收据内容），全部为 UNVERIFIED。无论文档表格中填写何内容、无论历史记忆或静态配置记录过何信息，absent a complete auditable non-secret capability receipt, status must be UNVERIFIED。**
>
> **运行时启动时（第 6 节强制流程）必须执行实时能力发现，获得至少一个 VERIFIED 条目后才能建立路由表。**

| # | 模型 ID | 提供商 | 路径 | 状态 | 缺失字段 | 观察来源 | 说明 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | `deepseek-v4-pro` | DeepSeek | OpenRouter → DeepSeek | **UNVERIFIED** | 完整收据 | Hermes Agent Memory | 推理模型；max_tokens≥4096；不支持 response_format json_object；content 可能为空、推理内容在 reasoning_content |
| 2 | `llama-4-maverick` | Meta | OpenRouter → Meta | **UNVERIFIED** | 完整收据 | Hermes Agent Memory | 通用对话模型；标准 chat completions API |
| 3 | `mistral-large` | Mistral | OpenRouter → Mistral | **UNVERIFIED** | 完整收据 | Hermes Agent Memory | 通用对话模型；标准 chat completions API |
| 4 | `gpt-5.6-sol` | OpenAI | OpenAI 直连 | **UNVERIFIED** | 完整收据 | Hermes Agent Memory | OpenRouter Provider Allowlist 拦截（403）；OAuth 凭证与 Hermes 不共享 |
| 5 | `gpt-5.6-terra` | OpenAI | OpenAI 直连 | **UNVERIFIED** | 完整收据 | Hermes Agent Memory | 同上 |
| 6 | Anthropic 全系 | Anthropic | OpenRouter → Anthropic | **UNVERIFIED** | 完整收据 | — | 历史 Provider Allowlist 观察（403）不属于当前发现纪元的运行时能力收据；不可成为排除于强制发现的理由；运行时发现时将产生当前纪元收据 |
| 7 | Google 全系 | Google | OpenRouter → Google | **UNVERIFIED** | 完整收据 | — | 同上；历史 Provider Allowlist 观察不可替代当前纪元运行时收据 |

### 2.3 路由规则（强制）

1. 路由决策**仅可**基于 §2.1 定义的 VERIFIED 条目（持有当前发现纪元完整运行时能力收据）
2. 运行时启动必须执行强制能力发现（§6），获得至少一个 VERIFIED 条目；无 VERIFIED 条目 → `fail-closed`（NO_TRADE，不静默降级）
3. UNVERIFIED 条目仅作为"待验证候选"载入发现队列，禁止用于任何路由决策；所有 UNVERIFIED provider/model 必须进入强制启动发现
4. UNAVAILABLE 条目仅当持有当前发现纪元的完整运行时能力收据、收据中 result 字段证明该能力不可用/被拒绝时方可成立。该条目排除于当前纪元的全部路由路径，不消耗重试预算；下一发现纪元必须重新验证（UNAVAILABLE 不跨纪元传承）
5. 历史 Provider Allowlist 观察、静态配置、文档记忆、过去 session 中记录的 provider 支持状态——以上任何非运行时收据来源均**不得**作为排除 provider 于强制发现的依据
6. 任何未在运行时完成 VERIFIED 的模型不得通过 task override、环境变量、config 等方式绕过验证直接使用
7. 历史记忆中的模型经验（如"曾经可用"、"曾经被拦截"）不作为当前可用性或不可用性的证据
8. 本文档的 TRADE/ROUTE/ORDER/SIGN 授权标志全部为 false

---

## 3. Adapter 接口设计

### 3.1 公共抽象接口（Python）

```python
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from enum import Enum
from typing import Any


class AdapterStatus(Enum):
    """适配器连接状态（fail-closed 默认 UNVERIFIED）"""
    UNVERIFIED = "UNVERIFIED"
    VERIFIED = "VERIFIED"
    DEGRADED = "DEGRADED"
    UNAVAILABLE = "UNAVAILABLE"


class Capability(Enum):
    """模型能力枚举"""
    CHAT = "CHAT"
    FUNCTION_CALLING = "FUNCTION_CALLING"
    STRUCTURED_OUTPUT = "STRUCTURED_OUTPUT"     # JSON mode / response_format
    REASONING = "REASONING"                     # reasoning_content / thinking
    VISION = "VISION"


class ErrorClass(Enum):
    """操作错误分类（见 §5 完整定义）"""
    NONE = "NONE"
    AUTH_FAILED = "AUTH_FAILED"
    PROVIDER_DENIED = "PROVIDER_DENIED"
    RATE_LIMITED = "RATE_LIMITED"
    TRANSIENT_TRANSPORT = "TRANSIENT_TRANSPORT"
    TIMEOUT = "TIMEOUT"
    INVALID_RESPONSE = "INVALID_RESPONSE"
    UNKNOWN = "UNKNOWN"


@dataclass
class VerifiedEntry:
    """VERIFIED 条目：完整的 7 项字段"""
    timestamp: str                      # UTC ISO-8601
    provider: str
    model: str
    capability: set[Capability]
    result: dict[str, Any]              # {http_status, latency_ms, tokens_consumed, ...}
    failure_class: ErrorClass           # 成功时为 NONE
    evidence_hash: str                  # SHA-256 hex


@dataclass
class AdapterCapability:
    """完成发现后的能力声明"""
    adapter_id: str
    status: AdapterStatus
    capabilities: set[Capability]
    max_tokens: int
    constraints: dict[str, Any] = field(default_factory=dict)
    verified_entry: VerifiedEntry | None = None  # None 当 status != VERIFIED
    # 约束示例：
    # {"response_format_unsupported": ["json_object"],
    #  "min_tokens_required": 4096,
    #  "content_may_be_empty": true,
    #  "reasoning_field": "reasoning_content"}


class ModelAdapter(ABC):
    """所有模型适配器的抽象基类"""

    @abstractmethod
    async def discover_capabilities(self) -> AdapterCapability:
        """连接并验证模型可用性。返回的 AdapterCapability.verified_entry
        必须包含完整的 7 项 VERIFIED 字段（§2.1）。
        必须做实际 API 调用来确认可用性。
        失败 → status=UNAVAILABLE，禁止默默替换。
        """

    @abstractmethod
    async def invoke(
        self,
        messages: list[dict[str, Any]],
        tools: list[dict[str, Any]] | None = None,
        **kwargs: Any,
    ) -> dict[str, Any]:
        """调用模型。kwargs 透传 provider 特定参数。
        异常时必须抛出 AdapterException（含 ErrorClass 分类）。
        """

    @property
    @abstractmethod
    def adapter_id(self) -> str:
        """适配器唯一标识"""

    @property
    @abstractmethod
    def model_id(self) -> str:
        """模型标识（如 deepseek-v4-pro）"""
```

### 3.2 DeepSeek Adapter

```
适配器 ID:    deepseek-v4-pro
模型 ID:      deepseek-v4-pro
提供商:       OpenRouter → DeepSeek
状态:         运行时强制发现前为 UNVERIFIED
```

**预期特性与约束（待运行时验证）：**

| 项目 | 值 |
| --- | --- |
| 预期支持能力 | CHAT、FUNCTION_CALLING、REASONING |
| 预期不支持 | STRUCTURED_OUTPUT（`response_format: json_object`） |
| max_tokens 下限 | 4096（低于此值 `content` 可能空） |
| reasoning_content | 在 `reasoning_content` 字段，不在 `content` |
| tool use | 标准 OpenAI-compatible function calling |
| 超时 | 建议 120s（推理模型耗时） |

**调用约束（实现时必须校验）：**
- 请求必须 `max_tokens >= 4096`
- 不得设置 `response_format`
- 解析响应时优先 `reasoning_content` → 其次 `content`
- 当 `content` 为空字符串、仅空格时使用 `reasoning_content` 作为模型输出

**能力发现步骤（运行时执行）：**
1. 发送最小对话调用 `[{"role": "user", "content": "ping"}]`（`max_tokens=1`）→ 验证连接
2. 发送 function calling 调用 → 验证 tool use 能力
3. 记录实际模型名、HTTP 状态码、延迟、token 消耗
4. 计算所有输入输出的 SHA-256 证据哈希
5. 失败 → status=UNAVAILABLE + failure_class=对应错误类（§5）；成功 → status=VERIFIED + 完整的 VerifiedEntry（7 字段）+ 写入 Capability 缓存

### 3.3 OpenAI-Compatible Adapter

```
适配器 ID:    openai-compatible
模型 ID:      动态（由回退逻辑选择）
提供商:       OpenRouter（受 Provider Allowlist 限制）
状态:         运行时强制发现前为 UNVERIFIED
```

**候选模型池（待运行时验证）：**
- `meta/llama-4-maverick`（高能力）
- `mistral/mistral-large`（快速）

**预期特性与约束：**
- 标准 OpenAI chat completions API
- FUNCTION_CALLING 预期可用（取决于具体模型）
- 无 REASONING 能力（无 `reasoning_content` 字段）
- 不预期支持 STRUCTURED_OUTPUT

**能力发现步骤：**
1. 对候选列表逐一发送 ping 调用
2. 确认 HTTP 200 + 非空 completion
3. 对每个可用模型生成完整的 VerifiedEntry（7 字段 + 证据哈希）
4. 标记可用/不可用（Provider Allowlist 可能拦截某些模型）
5. 所有模型都不满足 VERIFIED → 整个适配器 AggregateStatus=UNAVAILABLE

### 3.4 Hermes Native Adapter（Hermes Agent 原生路由）

```
适配器 ID:    hermes-native
模型 ID:      由 Hermes profile 配置决定
提供商:       Hermes Agent Runtime
状态:         运行时强制发现前为 UNVERIFIED
```

**职责范围：**
- 当运行环境本身是 Hermes Agent 时、利用 Hermes 内置的 profile 和 provider 配置
- 不绕过 Hermes 的安全机制（confirm 协议、工具白名单等）
- 模型选择由 Hermes profile 控制、不在适配器内部硬编码

---

## 4. 回退（Fallback）模型

### 4.1 回退触发条件

回退触发条件使用第 5 节定义的错误分类体系。触发逻辑如下：

| 条件 | 对应 ErrorClass | 检测方式 | 动作 |
| --- | --- | --- | --- |
| 主模型返回 HTTP 401/403（认证层） | AUTH_FAILED | API 响应码 | 立即 NO_TRADE，不回退（认证问题不会自愈） |
| 主模型返回 HTTP 5xx | TRANSIENT_TRANSPORT | API 响应码 | 触发回退（有限重试后） |
| 主模型返回 HTTP 429 | RATE_LIMITED | API 响应码 | 触发回退（遵守 Retry-After 头） |
| 主模型超时（>120s） | TIMEOUT | 请求超时 | 触发回退 |
| 主模型返回 HTTP 403（应用层） | PROVIDER_DENIED | API 响应码 | 标记 UNAVAILABLE，不重试，触发回退 |
| 主模型返回空 content（无 reasoning_content） | INVALID_RESPONSE | 响应解析 | 触发回退（先有限重试） |
| 主模型 content 格式不可解析 | INVALID_RESPONSE | 响应解析 | 触发回退（先有限重试） |
| 连续 N 次异常（N=3，30s 窗口） | 以最新类别为准 | 适配器内部计数器 | 降级模型状态为 DEGRADED，触发回退 |

### 4.2 回退链

```
deepseek-v4-pro (候选主路由，需运行时 VERIFIED)
    │
    ├─ 不可用 ──► meta/llama-4-maverick (回退层1，需运行时 VERIFIED)
    │                    │
    │                    ├─ 不可用 ──► mistral/mistral-large (回退层2，需运行时 VERIFIED)
    │                    │                    │
    │                    │                    ├─ 不可用 ──► NO_TRADE
    │                    │                    │              + 用户通知
    │                    │                    │              + 审计事件（含 failure_class）
    │                    └─ VERIFIED ──► 使用 llama-4-maverick
    │                                  + 用户通知（降级模式）
    │                                  + 审计事件
    └─ VERIFIED ──► 使用 deepseek-v4-pro
```

### 4.3 回退恢复条件

- DEGRADED/UNAVAILABLE 状态下每 300s 自动探测一次
- 恢复前需要完整的能力发现流程（§6），生成新的 VerifiedEntry（7 字段）
- 仅当探测结果满足全部 VERIFIED 条件时才恢复路由
- 不自动恢复到 UNVERIFIED 状态、必须经过完整能力发现流程
- 探测失败时记录 failure_class（§5）

### 4.4 回退安全约束

**任何回退发生时：**
- 必须通过 `kanban_comment` 或审计事件记录，含 `failure_class`（§5）
- 必须在前端通知用户（降级模式指示器）
- 回退模型输出同样通过 contracts 验证管线
- 回退链全部耗尽 → NO_TRADE，不静默失败
- 不得将历史 Provider Allowlist 观察或非收据来源的信息作为排除 provider 于回退尝试的依据；仅当前发现纪元的运行时能力收据（UNAVAILABLE）方可排除对应 provider
- 不得为回退新建 API key 或切换认证方式
- 回退目标模型必须同样完成 §6 强制能力发现，不可使用 UNVERIFIED 条目；下一发现纪元将对当前 UNAVAILABLE 条目重新验证

---

## 5. 操作错误分类体系

所有模型适配器的操作错误必须归类到以下分类体系。每个错误类定义了回退资格、NO_TRADE 行为、有界重试预算、对账要求和审计字段。

### 5.1 错误分类概览

| 错误类 | 典型 HTTP 码 | 回退资格 | NO_TRADE | 最大重试 | 重试策略 |
| --- | --- | --- | --- | --- | --- |
| **AUTH_FAILED** | 401, 403(认证) | **无** | 立即 | 0 | 不重试 |
| **PROVIDER_DENIED** | 403(应用), 402 | **无** | 立即 | 0 | 不重试 |
| **RATE_LIMITED** | 429 | **有** | 回退耗尽后 | 3 | 指数退避，尊重 Retry-After |
| **TRANSIENT_TRANSPORT** | 5xx, 连接错误 | **有** | 回退耗尽后 | 3 | 指数退避 1s/2s/4s |
| **TIMEOUT** | — | **有** | 回退耗尽后 | 2 | 下次超时加倍 |
| **INVALID_RESPONSE** | 200（内容异常） | **有** | 回退耗尽后 | 2 | 指数退避 |
| **UNKNOWN** | 其他 | **无** | 立即 | 0 | 不重试（未知=不安全） |

### 5.2 AUTH_FAILED

- **定义**：认证凭据无效、过期或被撤销
- **典型表现**：HTTP 401 Unauthorized、403 Forbidden（认证层，如 API key 无效）
- **回退资格**：**不具备**。认证失败意味着该适配器的凭据整体无效，不得回退到使用同一凭据的同级适配器
- **NO_TRADE 行为**：立即 NO_TRADE，不进入回退链。如果所有适配器共享同一凭据 → 系统级 fail-closed
- **有界重试预算**：0。认证失败不会自愈，重试无意义
- **对账**：标记适配器 AUTH_FAILED，触发管理员通知，需人工重新配置凭据
- **审计字段（已脱敏）**：
  ```json
  {
    "error_class": "AUTH_FAILED",
    "provider": "<provider_name>",
    "model": "<model_id>",
    "timestamp_utc": "<ISO-8601>",
    "http_status": 401,
    "auth_method_type": "<bearer|api_key|oauth|...>",
    "key_prefix_redacted": "<前4字符>***",
    "retry_count": 0,
    "adapter_id": "<adapter_id>"
  }
  ```

### 5.3 PROVIDER_DENIED

- **定义**：提供商明确拒绝请求（非认证原因），如 Provider Allowlist 拦截、账户无权限、余额不足
- **典型表现**：HTTP 403（应用层，如 OpenRouter Provider Allowlist）、HTTP 402 Payment Required
- **回退资格**：**不具备**。提供商层面拒绝，换上另一模型到同一提供商同样被拒
- **NO_TRADE 行为**：立即 NO_TRADE；提供商级别的拒绝意味着该提供商的所有模型不可用，同时将整个提供商标记为 UNAVAILABLE
- **有界重试预算**：0。策略性拒绝不会因为重试而改变
- **对账**：标记该提供商所有模型为 UNAVAILABLE，触发管理员通知
- **审计字段（已脱敏）**：
  ```json
  {
    "error_class": "PROVIDER_DENIED",
    "provider": "<provider_name>",
    "model": "<model_id>",
    "timestamp_utc": "<ISO-8601>",
    "http_status": 403,
    "denied_scope": "<provider|model|region|allowlist>",
    "retry_count": 0,
    "adapter_id": "<adapter_id>"
  }
  ```

### 5.4 RATE_LIMITED

- **定义**：请求速率超过提供商限制
- **典型表现**：HTTP 429 Too Many Requests，含 Retry-After 头
- **回退资格**：**具备**。速率限制是临时的，回退到备用模型可维持服务
- **NO_TRADE 行为**：在回退链全部耗尽后 → NO_TRADE + 用户通知
- **有界重试预算**：最多 3 次（含初始请求），每次重试 = 初始请求 + 回退链尝试
  - 第 1 次触发：等待 Retry-After 秒（无头则默认 5s）
  - 第 2 次触发：等待 min(Retry-After × 2, 60s)
  - 第 3 次触发：不再重试当前模型，永久切换到回退层
- **对账**：记录触发时间、Retry-After 值、实际等待时间
- **审计字段（已脱敏）**：
  ```json
  {
    "error_class": "RATE_LIMITED",
    "provider": "<provider_name>",
    "model": "<model_id>",
    "timestamp_utc": "<ISO-8601>",
    "http_status": 429,
    "retry_after_seconds": 5,
    "retry_count": 1,
    "fallback_triggered": true,
    "adapter_id": "<adapter_id>"
  }
  ```

### 5.5 TRANSIENT_TRANSPORT

- **定义**：临时性网络/传输层错误，可能在短时间后自愈
- **典型表现**：HTTP 500/502/503/504、连接重置（ECONNRESET）、DNS 解析失败、TLS 握手失败
- **回退资格**：**具备**。传输层错误是临时的，回退可维持服务
- **NO_TRADE 行为**：在回退链全部耗尽后 → NO_TRADE
- **有界重试预算**：最多 3 次（含初始请求）
  - 退避策略：1s → 2s → 4s（指数退避）
  - 3 次全部失败 → 将该适配器降级为 DEGRADED，触发回退
- **对账**：记录每次重试的 HTTP 状态码/错误信息、退避等待时间
- **审计字段（已脱敏）**：
  ```json
  {
    "error_class": "TRANSIENT_TRANSPORT",
    "provider": "<provider_name>",
    "model": "<model_id>",
    "timestamp_utc": "<ISO-8601>",
    "http_status": 502,
    "retry_count": 2,
    "last_backoff_seconds": 4,
    "fallback_triggered": true,
    "adapter_id": "<adapter_id>"
  }
  ```

### 5.6 TIMEOUT

- **定义**：请求超过配置的超时限制
- **典型表现**：请求在 `timeout_seconds` 内未完成
- **回退资格**：**具备**。超时可能是临时性的
- **NO_TRADE 行为**：在回退链全部耗尽后 → NO_TRADE
- **有界重试预算**：最多 2 次（含初始请求）
  - 第 1 次超时：重试，超时时间加倍（如 120s → 240s）
  - 第 2 次超时：不再重试该模型，触发回退
- **对账**：记录每次超时时间配置、实际等待时间
- **审计字段（已脱敏）**：
  ```json
  {
    "error_class": "TIMEOUT",
    "provider": "<provider_name>",
    "model": "<model_id>",
    "timestamp_utc": "<ISO-8601>",
    "configured_timeout_seconds": 120,
    "retry_count": 1,
    "next_timeout_seconds": 240,
    "fallback_triggered": false,
    "adapter_id": "<adapter_id>"
  }
  ```

### 5.7 INVALID_RESPONSE

- **定义**：HTTP 200 但响应内容不符合预期格式——空 content、不可解析的 JSON、缺失必要字段
- **典型表现**：HTTP 200 + 空 body、HTTP 200 + content 为空字符串、HTTP 200 + JSON 解析失败
- **回退资格**：**具备**。可能是模型临时的异常输出
- **NO_TRADE 行为**：在回退链全部耗尽后 → NO_TRADE
- **有界重试预算**：最多 2 次（含初始请求）
  - 退避策略：2s → 4s
  - 2 次均无效 → 触发回退
- **对账**：记录原始响应长度、解析错误信息（截断）、是否触发了 reasoning_content 回退读取
- **审计字段（已脱敏）**：
  ```json
  {
    "error_class": "INVALID_RESPONSE",
    "provider": "<provider_name>",
    "model": "<model_id>",
    "timestamp_utc": "<ISO-8601>",
    "http_status": 200,
    "failure_reason": "<empty_content|json_parse_error|missing_field>",
    "response_length_bytes": 0,
    "retry_count": 1,
    "reasoning_fallback_used": false,
    "adapter_id": "<adapter_id>"
  }
  ```

### 5.8 UNKNOWN

- **定义**：无法归类到上述 6 类的任何错误
- **典型表现**：未预见的 HTTP 状态码、未文档化的异常类型
- **回退资格**：**不具备**。未知=不安全。不能用未知的错误去驱动回退
- **NO_TRADE 行为**：立即 NO_TRADE。未知错误不可信任，不尝试回退
- **有界重试预算**：0。重试未知错误可能放大风险
- **对账**：记录完整的错误上下文（已脱敏），触发管理员审查
- **审计字段（已脱敏）**：
  ```json
  {
    "error_class": "UNKNOWN",
    "provider": "<provider_name>",
    "model": "<model_id>",
    "timestamp_utc": "<ISO-8601>",
    "raw_error_type": "<exception_class_name>",
    "http_status": null,
    "raw_message_truncated_256": "<前256字符>",
    "retry_count": 0,
    "escalation": "ADMIN_REVIEW_REQUIRED",
    "adapter_id": "<adapter_id>"
  }
  ```

### 5.9 分类汇总矩阵

| 属性 | AUTH_FAILED | PROVIDER_DENIED | RATE_LIMITED | TRANSIENT_TRANSPORT | TIMEOUT | INVALID_RESPONSE | UNKNOWN |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 回退资格 | ❌ | ❌ | ✅ | ✅ | ✅ | ✅ | ❌ |
| 最大重试 | 0 | 0 | 3 | 3 | 2 | 2 | 0 |
| 退避策略 | — | — | Retry-After | 指数 1/2/4s | 超时加倍 | 2s/4s | — |
| NO_TRADE | 立即 | 立即 | 回退耗尽后 | 回退耗尽后 | 回退耗尽后 | 回退耗尽后 | 立即 |
| 对账动作 | 管理员通知 | 标记 UNAVAILABLE | 记录限速信息 | 记录退避日志 | 记录超时配置 | 记录响应异常 | 管理员审查 |
| 自动恢复 | ❌ | ❌ | ✅ | ✅ | ✅ | ✅ | ❌ |

---

## 6. 能力发现协议

### 6.1 强制运行时发现（Mandatory Startup Discovery）

能力发现**不是可选的**。以下规则为强制性：

1. **启动时强制运行**：Hermes Agent Runtime 启动后，在建立路由表之前，必须对 §2.2 中列出的所有 UNVERIFIED 候选模型执行完整能力发现流程。UNAVAILABLE 状态仅在持有当前发现纪元的完整运行时能力收据（证明不可用/被拒绝）时方可使用，不得基于历史观察或静态配置将任何 provider/model 排除于发现范围之外
2. **无 VERIFIED 条目 = fail-closed**：所有候选模型均未通过 VERIFIED → 系统以 fail-closed 模式运行（NO_TRADE），不静默降级到任何 UNVERIFIED 模型
3. **禁止跳过**：不得通过环境变量、config flag 或命令行参数跳过启动发现
4. **发现完成前禁止路由**：在任何模型完成 VERIFIED 之前，系统拒绝所有路由请求，返回 "routing unavailable — capability discovery in progress"
5. **文档阶段声明**：本文档为设计文档，未执行实时发现。以下流程描述的是**运行时实现必须遵循的规范**

### 6.2 Fail-Closed 原则

1. 默认状态：所有适配器 UNVERIFIED
2. 必须通过实际 API 调用验证后才标记 VERIFIED（含完整 7 字段 VerifiedEntry）
3. 模型文档声明、历史记忆、提供商承诺均不等于可用性验证
4. 之前可用不代表当前可用（缓存 TTL: 300s，过期需重新验证）
5. 不可用时不得自动选择"看起来能用的"模型
6. 7 项 VERIFIED 字段缺一不可——缺少 evidence_hash 视为 UNVERIFIED

### 6.3 发现流程（运行时执行顺序）

```
Hermes Agent 启动
  │
  ├─ 1. 加载候选模型列表（§2.2 中状态 = UNVERIFIED 的所有条目；不得基于历史观察跳过任何条目）
  │
  ├─ 2. 对每个候选模型执行发现：
  │     ├─ deepseek-v4-pro →
  │     │     ├─ ping 调用 → 记录 http_status, latency_ms
  │     │     ├─ function calling 调用 → 验证 tool use
  │     │     ├─ SHA-256 哈希所有输入输出
  │     │     ├─ 成功 → 生成 VerifiedEntry(7 字段) → VERIFIED
  │     │     └─ 失败 → 分类 ErrorClass(§5) → 生成能力收据(含 failure_class) → UNAVAILABLE(当前纪元)
  │     │
  │     ├─ llama-4-maverick →
  │     │     ├─ ping 调用 → 同上流程
  │     │     ├─ 成功 → VERIFIED
  │     │     └─ 失败 → 生成能力收据 → UNAVAILABLE(当前纪元)
  │     │
  │     ├─ mistral-large →
  │     │     ├─ ping 调用 → 同上流程
  │     │     ├─ 成功 → VERIFIED
  │     │     └─ 失败 → 生成能力收据 → UNAVAILABLE(当前纪元)
  │     │
  │     ├─ gpt-5.6-sol →
  │     │     ├─ ping 调用 → 同上流程
  │     │     ├─ 成功 → VERIFIED
  │     │     └─ 失败 → 生成能力收据 → UNAVAILABLE(当前纪元)
  │     │
  │     ├─ gpt-5.6-terra →
  │     │     ├─ ping 调用 → 同上流程
  │     │     ├─ 成功 → VERIFIED
  │     │     └─ 失败 → 生成能力收据 → UNAVAILABLE(当前纪元)
  │     │
  │     ├─ Anthropic 全系 →
  │     │     ├─ ping 调用 → 同上流程（历史 Allowlist 观察不构成排除理由）
  │     │     ├─ 成功 → VERIFIED
  │     │     └─ 失败 → 生成能力收据(含 failure_class=PROVIDER_DENIED) → UNAVAILABLE(当前纪元)
  │     │
  │     └─ Google 全系 →
  │           ├─ ping 调用 → 同上流程
  │           ├─ 成功 → VERIFIED
  │           └─ 失败 → 生成能力收据 → UNAVAILABLE(当前纪元)
  │
  ├─ 3. 汇总结果：
  │     ├─ VERIFIED 条目 ≥ 1 → 建立路由表（主路由 + 回退链）
  │     │     ├─ 主路由 = 优先级最高的 VERIFIED 条目
  │     │     └─ 回退层 = 其余 VERIFIED 条目（按优先级排序）
  │     └─ VERIFIED 条目 = 0 → fail-closed
  │           └─ 返回 "routing unavailable"，拒绝所有交易相关请求
  │
  └─ 4. 缓存：VERIFIED 条目 TTL=300s
        └─ 过期后在下一次请求前自动重新验证
```

### 6.4 过期/失效收据处理（Stale Receipt Handling）

能力收据（无论是 VERIFIED 还是 UNAVAILABLE）均绑定于当前发现纪元，不可跨纪元传承。

1. **收据 TTL**：VERIFIED 和 UNAVAILABLE 收据均带 TTL（默认 300s）。过期后条目自动降级为 UNVERIFIED，在下一次请求或后台探测时重新进入强制发现
2. **VERIFIED 过期**：在任何使用过期 VERIFIED 条目的路由请求之前，必须触发重新验证。重新验证期间可使用最后的已知良好路由（如有），最长允许 10s 等待新验证结果。10s 内无法完成重新验证 → fail-closed（NO_TRADE）
3. **UNAVAILABLE 过期**：过期后条目回归 UNVERIFIED 状态，进入下一个发现纪元。不得因上一纪元的 UNAVAILABLE 收据而跳过新纪元的强制发现。新纪元运行时收据可产生不同的结果
4. **后台健康检查**：独立于请求触发，每 300s 主动探测以保持缓存新鲜。探测时对所有 UNVERIFIED 条目和即将过期的 VERIFIED 条目执行完整发现
5. **跨纪元禁止传递**：上一纪元的 UNAVAILABLE 收据不得作为当前纪元排除任何 provider/model 的依据；每个发现纪元独立执行完整发现，结果仅当前纪元有效
6. **收据存档**：所有收据（VERIFIED + UNAVAILABLE）持久化存档（已脱敏），用于审计追溯。存档收据不等同于当前纪元有效收据，仅作历史记录

### 6.5 不可验证声明（当前文档阶段）

以下事项当前无法验证、必须明确记录为未来工作：

- **所有 §2.2 模型条目的运行时验证**：本文档编写于文档阶段，未进行任何实时 API 调用。`deepseek-v4-pro`、`llama-4-maverick`、`mistral-large` 等模型的实际可用性须在实现阶段通过 §6.3 流程验证
- `gpt-5.6-sol` 和 `gpt-5.6-terra` 的可用性：OpenRouter 层面被 Provider Allowlist 拦截，OpenAI 直连路径使用独立 OAuth 凭证、与当前 Hermes 运行环境不共享。是否可共享该凭证路径为协调方决策（未来工作）
- Anthropic 和 Google 模型的可用性：历史上在 OpenRouter 层面被 Provider Allowlist 拦截（403）。但该历史观察不属于当前发现纪元的运行时能力收据，不可作为永久排除的依据。实现阶段必须对两个 provider 同样执行 §6.3 强制发现流程，由运行时收据决定当前纪元状态（VERIFIED 或 UNAVAILABLE）。不预判运行时结果
- 新模型注册：未来添加任何新模型时，必须通过完整 §6.3 发现流程获得 VERIFIED 状态后方可进入候选列表
- 不得在本文档中声称任何模型"可用"、"可配置"或"可回退"而不注明其 UNVERIFIED 状态

---

## 7. 安全边界

### 7.1 Adapter 禁止事项

对所有适配器（包括回退）：
- 禁止直接签名、提交或授权订单
- 禁止读取私钥、token、cookie 或 recovery material
- 禁止执行任意 shell 或 SQL
- 禁止绕过 contracts 验证管线
- 禁止修改 MCP 工具清单
- 禁止注入 `user_id`、`account_id`、`session_id`
- 禁止访问生产数据库或交易所写 API
- 禁止为回退新建 API key 或切换认证方式

### 7.2 信任边界

```
┌──────────────────────────────────────┐
│  低信任：模型输出（任意适配器）       │
│  - 不可直接作为签名请求               │
│  - 不可直接作为风控输入               │
│  - 必须通过 contracts 验证管线        │
│  - UNVERIFIED 模型输出不得进入信任链  │
└──────────────┬───────────────────────┘
               ▼
┌──────────────────────────────────────┐
│  已验证：contracts.py + validator.py │
│  - Pydantic v2 严格验证               │
│  - JSON Schema 格式检查               │
│  - MCP 工具清单不变性检查             │
│  - ErrorClass 审计字段脱敏检查        │
└──────────────┬───────────────────────┘
               ▼
┌──────────────────────────────────────┐
│  已授权：ConfirmationTicket           │
│  - RFC 8785 哈希绑定                  │
│  - 用户确认消费                       │
│  - 一次性 nonce                       │
└──────────────┬───────────────────────┘
               ▼
┌──────────────────────────────────────┐
│  已准入：Go 风控核心                  │
│  - 确定性计算                         │
│  - 数据新鲜度检查                     │
│  - 全部拒绝条件检查                   │
└──────────────────────────────────────┘
```

- 模型输出不能跳级：原始文本 → 结构化意图 → 确认 → 风控 → 签名
- 回退模型输出同样通过完整信任链
- UNVERIFIED 模型输出不得进入信任链
- 无 VERIFIED 条目时系统 fail-closed，不产生任何输出进入信任链

---

## 8. 实现注意事项

### 8.1 Python 包结构（设计建议）

```
services/hermes-agent/src/hermes_agent/
├── adapters/
│   ├── __init__.py
│   ├── base.py           # ModelAdapter ABC + AdapterCapability + ErrorClass + VerifiedEntry
│   ├── deepseek.py       # DeepSeekAdapter
│   ├── openai_compat.py  # OpenAICompatibleAdapter
│   ├── hermes_native.py  # HermesNativeAdapter
│   └── fallback.py       # FallbackChain + fallback 逻辑 + ErrorClass 路由
├── discovery/
│   ├── __init__.py
│   └── capability.py     # CapabilityDiscovery + 健康检查 + TTL 管理 + VerifiedEntry 生成
├── errors/
│   ├── __init__.py
│   └── taxonomy.py       # ErrorClass 枚举 + 分类函数 + 审计日志构建器
├── contracts.py          # 已有
├── validator.py          # 已有
├── mcp_tools.py          # 已有
└── confirmation.py       # 已有
```

### 8.2 依赖

当前 `pyproject.toml` 不含 HTTP 客户端依赖。实现时需要添加：

- `httpx`（异步 HTTP 客户端）— 用于 OpenRouter API 调用
- `openai`（可选，用于 OpenAI-compatible 适配器简化实现）
- `redis`（暂缓，回退状态缓存使用内存）

### 8.3 配置文件（设计建议）

```yaml
# coordination/config/AI-AGENT/model-routing.yaml
models:
  primary:
    adapter: deepseek
    model_id: deepseek-v4-pro
    provider: openrouter
    base_url: https://openrouter.ai/api/v1
  fallback:
    - adapter: openai-compatible
      model_id: meta/llama-4-maverick
      provider: openrouter
    - adapter: openai-compatible
      model_id: mistral/mistral-large
      provider: openrouter

routing:
  discovery:
    mandatory_startup: true           # 启动时强制运行
    initial_timeout_seconds: 30
    cache_ttl_seconds: 300
    health_check_interval_seconds: 300
    stale_reverify_timeout_seconds: 10
  fallback:
    degradation_threshold: 3
    degradation_window_seconds: 30
    recovery_probe_interval_seconds: 300
  timeout:
    primary_model_seconds: 120
    fallback_model_seconds: 60
  error_taxonomy:                     # §5 错误分类参数
    retry:
      transient_max: 3
      timeout_max: 2
      invalid_response_max: 2
      rate_limited_max: 3
      transient_base_delay_seconds: 1
    auth_failed_no_retry: true
    provider_denied_no_retry: true
    unknown_no_retry: true
```

### 8.4 环境变量

```
OPENROUTER_API_KEY=...        # 必需：OpenRouter API 密钥
OPENROUTER_BASE_URL=...       # 可选：OpenRouter API 地址
MODEL_ROUTING_CONFIG=...      # 可选：路由配置文件路径
```

---

## 9. 测试策略

| 测试类型 | 覆盖内容 | 实现阶段 |
| --- | --- | --- |
| 单元测试 | Adapter 接口契约、Fallback 链逻辑、ErrorClass 分类函数、VerifiedEntry 验证逻辑、能力发现状态机 | 实现 |
| 集成测试 | 与 OpenRouter 实际连接、ping 验证、function calling 验证、VerifiedEntry 完整性验证 | 实现 |
| 故障注入 | 主模型超时/500/429/401/403/空响应 → 回退测试、回退耗尽 → NO_TRADE | 实现 |
| ErrorClass 测试 | 每个错误类（7 类）的触发、分类准确性、重试预算耗尽、回退资格正确性 | 实现 |
| 回退安全 | 回退不绕过 contracts 验证管线、不泄漏凭据、AUTH_FAILED/PROVIDER_DENIED/UNKNOWN 不回退 | 实现 |
| 不可用验证 | gpt-5.6-sol/terra 的 403 断言、Anthropic/Google 的 403 断言 | 实现 |
| 强制发现测试 | 启动时发现流程完整性、无 VERIFIED = fail-closed、TTL 过期重验证、stale 检测 | 实现 |
| 审计字段测试 | 所有 7 个 ErrorClass 的审计字段完整性、脱敏验证（无 key/secrets） | 实现 |

---

## 10. 标准结果包

```
TASK_ID:         t_7753f6ae
ROLE:            AI-AGENT-FIX-R3 R-H1 — 能力收据与发现纪元修复
BASELINE:        f949b0e47a1b911a9a8bfebf117df7a49af0c837  (HISTORICAL_PRE_AMEND_SNAPSHOT_NOT_CURRENT_EVIDENCE — 当前固定候选身份为 EXTERNAL_FIXED_OBJECT_REVIEW_REQUIRED)
STATUS:          完成（设计修复阶段）
ALLOWED_PATHS:   coordination/design/AI-AGENT/H1_MULTI_MODEL_ADAPTER_DESIGN.md,
                 coordination/evidence/AI-AGENT/H1_DESIGN_EVIDENCE.md
FILES_MODIFIED:
  - coordination/design/AI-AGENT/H1_MULTI_MODEL_ADAPTER_DESIGN.md（§2.1 状态定义重构 + §2.2 Anhropic/Google 改为 UNVERIFIED + §2.3 路由规则强化纪元绑定 + §4.4 回退约束修复 + §6.1/§6.3 发现流程修正为所有 UNVERIFIED + §6.4 过期收据处理重定义 + §6.5 移除永久不可用断言）
  - coordination/evidence/AI-AGENT/H1_DESIGN_EVIDENCE.md（对齐设计全部变更）
KEY_CHANGES:
  1. §2.1 状态定义增强：VERIFIED/UNVERIFIED/UNAVAILABLE 三态均绑定"当前发现纪元+完整运行时能力收据"；
     UNVERIFIED 必须进入强制启动发现；UNAVAILABLE 仅当前纪元收据有效，下一纪元必须重新验证
  2. §2.2 Anhropic/Google 从 UNAVAILABLE 改为 UNVERIFIED：历史 Provider Allowlist 观察不属于当前纪元运行时能力收据，
     不可成为排除于强制发现的理由
  3. §2.3 路由规则新增：历史 allowlist/静态配置/文档记忆不得排除任何 provider 于强制发现
  4. §4.4 回退安全约束：历史 Provider Allowlist 观察不可作为排除回退的依据
  5. §6.1 强制启动发现：从"非 UNAVAILABLE"改为"所有 UNVERIFIED"；UNAVAILABLE 仅运行时收据方可成立
  6. §6.3 发现流程：补充 gpt-5.6-sol/terra、Anthropic、Google 进入流程；统一失败处理为 UNAVAILABLE(当前纪元)
  7. §6.4 重定义为"过期/失效收据处理"：新增 UNAVAILABLE 过期回归 UNVERIFIED、
     跨纪元禁止传递、收据存档（审计但不替代当前纪元验证）
  8. §6.5 移除"不期望未来变化"断言：Anthropic/Google 由运行时收据决定当前纪元状态，不预判
UNRESOLVED_RISKS:
  - 所有 §2.2 模型条目为 UNVERIFIED（文档阶段），运行时可用性未验证
  - Anthropic/Google 历史上被 Provider Allowlist 拦截（403），运行时很可能仍是 PROVIDER_DENIED，但仅在产生当前纪元收据后才能确认为 UNAVAILABLE
  - 生产环境中 OpenAI 直连路径（Codex OAuth）可否共享给 Hermes Agent 路由，待协调方决定
  - 回退模型的能力差异（无 REASONING 能力）可能导致交易意图质量下降
  - OpenRouter Provider Allowlist 未来变化可能影响所有 provider 可用性
  - UNKNOWN 错误类的 fail-closed 策略（不重试、不回退）在极端边缘情况下可能过于保守
  - 每个发现纪元对所有 UNVERIFIED provider 执行 API 调用的开销（含被 Allowlist 拦截的 provider），需实现时评估
INTEGRATION_DEPENDENCIES:
  - services/hermes-agent/ 现有 contracts/validator/mcp_tools 管线
  - 外部 OpenRouter API（需运行时验证，包括 Anthropic/Google 端点）
  - Go 风控核心（确定性的下游消费方）
  - ErrorClass 枚举需在实现时与 Go 层错误码对齐（协调方决策）
MERGE=false
DEPLOY=false
PRODUCTION=false
LIVE_TRADING=false
```
