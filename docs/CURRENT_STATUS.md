# 当前项目进度

更新日期：2026-07-29  
当前轮次：第 1 轮（预计约 15 轮的完整产品计划）

## 1. 本轮结果

| 工作流 | 固定提交 | 独立审查 | Git 状态 |
| --- | --- | --- | --- |
| Phase 1 平台安全契约收口 | `9b4b113a4032f7f0eee88297781e3b2d069b1f18` | `PASS`，P0/P1/P2 = 0/0/0 | 已推送功能分支，未合并 |
| 网页/桌面 Command Center 原型 | `b18aa1f18d2c646cfcac569a1a6af81b91ca8743` | `PASS`，P0/P1/P2 = 0/0/0 | 已推送功能分支，未合并 |
| 开源依赖准入基线 | `d3a219a305930e6f9019e90c9301900493a60084` | `PASS`，P0/P1/P2 = 0/0/0 | 已推送功能分支，未合并 |

本轮修掉的主要风险：

- 登录、注册、刷新和授权字段使用完整 route/body/owner scope 参与幂等；
- exact response cache 可跨副本和重启恢复，过期密文可主动清理；
- 149 个进程终止切点和 165 个事务中止切点不产生部分业务效果；
- EventPayload、聚合身份、Inbox、Outbox lease 和 owner scope 有可执行约束；
- camelCase、连字符、全角和 Unicode 分隔形式不能绕过服务器权威字段禁令；
- 前端确认票据冻结交易意图，切换品种或发送新聊天不能覆盖活跃 Operation；
- Kill Switch、`STALE`、`RECONCILING` 和保护状态保持 fail-closed；
- 13 个开源候选固定版本、commit、许可证和 `ADOPT / REFERENCE_ONLY / PROHIBITED` 分类。

## 2. 验证证据

平台契约：

- `npm test`：通过；
- Node 24 独立执行 Argon2id 黄金向量：`VERIFIED_BY_NODE_CRYPTO_ARGON2`；
- 平台 Schema 59、正向 fixture 35、负向 fixture 28；
- 派生故障切点 329；
- 执行事务中止切点 165、进程终止切点 149、响应丢失核对 10；
- Git 历史秘密扫描：通过。

网页/桌面原型：

- TypeScript typecheck：通过；
- Vite production build：通过；
- Sites worker 测试：4/4；
- 浏览器对抗复测覆盖票据冻结、聊天竞态、Operation 独占、Kill Switch 和数据状态；
- 应用来源控制台错误/警告：0。

## 3. 完整产品进度

这三个数字用于管理，不代表发布或实盘就绪：

| 维度 | 管理估算 | 说明 |
| --- | ---: | --- |
| 需求、架构和 UX 基线 | 100% | 已形成当前版本事实来源 |
| Phase 0 契约与安全基础 | 约 90% | 主契约已在 main；平台补充契约已通过审查但尚未合并 |
| Phase 1 平台与数据 | 约 15% | 契约完成，Go/PostgreSQL/NATS 运行时尚未实现 |
| 网页/桌面客户端 | 约 30% | 高保真模拟壳完成，真实认证、API、数据和桌面能力尚未接入 |
| 完整产品 | 约 18% | 交易核心、只读适配器、Hermes、仿真交易、iOS、学习和自动交易仍未实现 |

严格按阶段退出条件计算，目前还没有新增 Phase 被宣布正式退出。测试通过只
证明对应固定提交满足本轮范围，不自动授权合并、部署、钱包、签名、真实下单
或自动交易。

## 4. 当前安全与发布状态

| 项目 | 状态 |
| --- | --- |
| `main` 集成 | 本轮功能分支尚未合并 |
| 服务器部署 | `NOT_DEPLOYED` |
| 生产启用 | `PRODUCTION_DISABLED` |
| Hyperliquid 连接 | `NOT_CONNECTED` |
| 钱包/签名器 | `NOT_IMPLEMENTED` |
| 真实下单 | `NOT_AUTHORIZED` |
| 自动交易 | `NOT_AUTHORIZED` |

## 5. 下一轮建议

第 2 轮建议只做 Phase 1 Go 平台最小纵切：

1. 建立 `services/trading-core/` Go module、配置和健康检查；
2. 按平台 Schema 建立用户、ownership、device、session、Inbox、Outbox 迁移；
3. 实现一个不触及交易的事务闭环：鉴权 scope → scoped idempotency →
   audit/outbox → exact response replay；
4. 用 PostgreSQL/NATS 测试容器执行崩溃、重投和恢复验证；
5. 固定提交独立审查后，再更新本文件和代码地图。

该轮不接 Hyperliquid 写 API、不接钱包、不签名、不部署、不启用实盘。
