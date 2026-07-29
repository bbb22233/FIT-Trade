# 项目代码地图

更新日期：2026-07-29

## 1. 当前仓库结构

```text
FIT-Trade/
├── contracts/                    跨语言契约、安全规则和可执行验证
│   ├── jsonschema/               交易领域与 MCP JSON Schema
│   ├── openapi/                  客户端 HTTP 契约
│   ├── proto/                    Go/Python 内部服务契约
│   ├── fixtures/                 正向、负向、黄金向量和安全场景
│   └── scripts/                  契约、秘密和语义验证器
├── prototype/web-desktop/        React/Vite 网页与桌面主界面原型
├── services/
│   ├── trading-core/             Phase 0 Go 领域契约一致性代码和测试
│   └── hermes-agent/             Phase 0 Python 契约/安全一致性代码和测试
├── docs/                         产品、架构、验收、计划和状态文档
└── coordination/                 多代理任务包、范围和验收证据
```

当前文档分支以 `main@60850b6` 为父提交。以下内容已经通过审查，但仍只存在于
尚未合并的功能分支：

| 功能分支 | 新增或扩展路径 |
| --- | --- |
| `codex/p1-platform-contracts@9b4b113` | `contracts/platform/{schemas,manifests}`、`contracts/fixtures/platform` 与平台验证器 |
| `codex/frontend-command-center-shell@b18aa1f` | `prototype/web-desktop` 的 Command Center 状态和交互 |
| `codex/oss-dependency-intake@d3a219a` | `docs/08_OPEN_SOURCE_ADOPTION.md` 与 `coordination/evidence/OSS-001` |

以下运行时能力或计划目录尚未实现，不能把 Phase 0 一致性代码、文档或原型
误认为可运行平台：

```text
services/trading-core 的 API、PostgreSQL、NATS 和平台运行时    尚未实现
services/hyperliquid-adapter/                                  目录不存在
services/hermes-agent 的真实 Agent runtime、模型路由和隔离     尚未实现
apps/ios/                                                     目录不存在
infra/                                                       目录不存在
```

## 2. 当前代码事实

| 区域 | 当前事实 | 权威边界 |
| --- | --- | --- |
| Phase 0 交易契约 | 已在 `main@60850b6` 建立共用领域契约，并在 `services/trading-core` 与 `services/hermes-agent` 建立 Go/Python 一致性代码和测试 | 这些服务目录目前只证明契约一致性，不是平台或 Agent 运行时；业务实现不能自行重定义交易语义 |
| Phase 1 平台契约 | `codex/p1-platform-contracts@9b4b113` 已通过独立固定提交审查并推送，尚未合并 | 定义 PostgreSQL 事务、身份、会话、事件、Inbox/Outbox、NATS、恢复和故障语义；没有 Go 平台运行时 |
| 网页/桌面原型 | `codex/frontend-command-center-shell@b18aa1f` 已通过独立固定提交审查并推送，尚未合并 | 仅本地模拟；没有钱包、签名器、交易所、模型 API 或生产后端 |
| 开源准入 | `codex/oss-dependency-intake@d3a219a` 固定 13 个候选的版本、许可证和使用边界，已通过独立固定提交审查并推送，尚未合并 | 准入不等于已经安装，也不授权交易所写入或生产 |
| 产品与架构文档 | `docs/00`—`07` 是当前产品和技术基线 | 文档不能替代实现、测试、合并、部署或实盘授权 |

三条功能分支的审查链和命令证据记录在
[`coordination/evidence/ROUND-01/review-results.json`](../coordination/evidence/ROUND-01/review-results.json)。

## 3. 关键依赖关系

| 上游事实 | 下游使用方 | 规则 |
| --- | --- | --- |
| `contracts/jsonschema`、`openapi`、`proto` | Go、Python、React、iOS | 下游只生成或消费类型，不能复制后独立修改 |
| `codex/p1-platform-contracts:contracts/platform/manifests/transaction-boundaries-v1.json` | 后续 Go PostgreSQL 事务实现 | 该契约尚未合并；合并后实现必须先完成身份/owner scope 验证，再做 scoped idempotency、Inbox 去重和业务效果 |
| `codex/p1-platform-contracts:contracts/platform/schemas/platform-v1.schema.json` | 后续 API、NATS publisher/consumer、恢复工具 | 该契约尚未合并；事件身份、聚合版本、Inbox/Outbox 和响应缓存必须保持一致 |
| `codex/oss-dependency-intake:coordination/evidence/OSS-001/dependency-intake.json` | 后续依赖安装与升级任务 | 该清单尚未合并；只能使用固定 ref/commit，`REFERENCE_ONLY` 和 `PROHIBITED` 不能进入运行时 |
| `prototype/web-desktop` | 后续真实客户端 | 当前 UI 状态只能替换为服务端权威状态，不能把模拟器升级成交易权威 |

## 4. 下一批落点

下一轮优先建立 `services/trading-core/` 的 Go 平台骨架和 PostgreSQL
迁移，把已审查的平台契约变成最小可运行实现。Hyperliquid 仍保持只读；
Hermes、签名器、交易所写 API、部署和实盘不在下一轮授权内。
