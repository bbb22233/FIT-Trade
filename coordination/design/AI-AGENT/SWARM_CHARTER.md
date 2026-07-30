# AI-AGENT-SWARM-BOOTSTRAP

## 状态与边界

- 阶段：开发与模拟设计；不是交易执行、部署或生产授权。
- 工作树：`/Users/guanlan/Documents/FIT-Trade-worktrees/ai-agent-swarm-bootstrap`
- 分支：`codex/ai-agent-swarm-bootstrap`
- 允许写入：仅 `coordination/design/AI-AGENT/**` 与 `coordination/evidence/AI-AGENT/**`。
- 禁止：任何凭据、私钥、token、cookie、signer、交易所写 API、订单、实盘钱包、生产数据库、`services/trading-core/`、`services/hyperliquid-adapter/`、`core` auth/trading code。
- 任何未能确认的执行结果必须标为 `UNKNOWN_REQUIRES_RECONCILIATION`；不得推断为成功。

## Kanban 依赖图

```text
Parent: AI Agent Design Kanban Swarm
  ├─ H1  多模型与 API adapter 设计
  ├─ H2  交易风格学习、记忆与离线评估设计
  ├─ H3  内置聊天与开仓/加仓确认 UX 设计
  └─ H4  AI 安全与滥用测试计划
                 \        |        /
                   Verifier（独立检查）
                            |
                   Synthesizer（设计汇总包）
```

每个 worker 只可产出其自己的设计文件和证据文件；Verifier 只可写复核证据；Synthesizer 只可写汇总设计与证据。所有卡片必须在最终结果中重复这条路径白名单。

## Worker 定义

| 卡片 | 交付物 | 必须覆盖 |
| --- | --- | --- |
| H1 | adapter 设计 | Hermes 主路由、DeepSeek V4 Pro、OpenAI-compatible 兼容层、显式 fallback 条件、模型能力/不可用时 fail-closed；禁止直接发单或签名。 |
| H2 | 学习与评估设计 | 审计化数据谱系、记忆隔离、冻结数据窗口、离线评估、禁止从实盘自我修改；不得把回测通过写成策略/交易授权。 |
| H3 | UX 设计 | 内置聊天、清晰模拟数据标记、开仓与加仓的人工确认、Stop Market 保护可见性、取消/超时、重复提交防护。 |
| H4 | 安全测试计划 | prompt injection、确认绕过、重复订单、秘密泄漏、工具越权、未知结果与 reconciliation；每项需给出可验证的 fail-closed 预期。 |

## 路由规则

- 经验证的当前 profile 仅为 `default`，主模型 `deepseek-v4-pro`。
- 所有本轮卡片使用该已验证 route。`gpt-5.6-sol`（架构/项目管理，`ultra`）与 `gpt-5.6-terra`（编码，`high`）只在本机可安全验证 provider/model/effort 后才可通过 task override 使用；本轮不猜测、不持久化修改 profile。
- 本轮为文档与证据工作，不需要代码实现模型。

## 完成标准

1. 四份 worker 设计相互独立且满足路径白名单。
2. Verifier 检查范围、产品安全边界、依赖闭环和 fail-closed 语义。
3. Synthesizer 生成可交给协调方的标准结果包：任务 ID、输入/输出、文件清单、路由、测试/检查、风险、依赖和明确授权状态。
4. 以下值必须为 false：`MERGE`、`DEPLOY`、`PRODUCTION`、`LIVE_TRADING`。
