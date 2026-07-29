# 开源依赖准入基线（OSS-001）

## 1. 结论与边界

- 任务：`OSS-001`
- 核验日期：`2026-07-29`
- 准入结果：`BOUNDARIES_FROZEN`
- 证据范围：只使用候选项目自己的 GitHub 仓库、固定 tag、发布页、许可证和固定 tag 下的源码。
- 当前阶段：仅开发和模拟。本文不授权连接钱包、签名器、交易所写 API、真实下单、自动交易、部署或生产启用。
- 机器可读清单：`coordination/evidence/OSS-001/dependency-intake.json`

依赖必须以本文固定的 tag 和解析后的 commit 为准；不能使用 `latest`、
分支头、浮动 semver 范围或未经审查的安装脚本。许可证准入不等于代码安全、
功能验收、合并或上线授权。

## 2. 统一准入规则

1. 运行时依赖必须由包管理器锁定精确版本和校验值；构建工具必须锁定发布
   资产及官方 GitHub Release 提供的摘要。
2. 升级必须单独提交：核对 tag/commit、许可证和 NOTICE 变化，阅读发布说明
   与安全公告，检查依赖树差异，然后运行合同、单元、集成、故障路径和秘密扫描。
3. Go 服务当前基线是 `Go 1.23`。准入版本必须能在该基线构建；提高 Go
   基线属于独立工具链变更，不能被依赖升级顺带完成。
4. 任何第三方库都不能成为订单、成交、仓位或保护状态的权威来源。服务端
   状态、幂等、未知结果关闭和交易所核对规则保持不变。
5. 任何依赖都不能获得钱包、签名密钥、恢复材料或越过受控接口的交易所写
   权限。Hermes 文本永远不能直接签名、下单或授权订单。
6. 开发 mock 必须显著标为模拟数据。测试通过不授权实盘、部署或生产。

## 3. 固定版本总表

| 候选 | 固定 ref | 解析 commit | 许可证 | 使用方式 | 准入状态 |
|---|---|---|---|---|---|
| hyperliquid-python-sdk | `0.24.0` | `2fdb18f9517675ea03695a0962bd19eece9c83f0` | MIT | 当前阶段只读/模拟 SDK | `APPROVED_CURRENT_PHASE_READ_ONLY` |
| Lightweight Charts | `v5.2.0` | `868cae27bd1acafa0128d8d868ea740a59ae42ce` | Apache-2.0 | Web 图表运行时 | `APPROVED_WITH_ATTRIBUTION` |
| NousResearch/hermes-agent | `v2026.7.20` | `3ef6bbd201263d354fd83ec55b3c306ded2eb72a` | MIT | OS 隔离的 Agent 运行时 | `APPROVED_ISOLATED_RUNTIME` |
| pgx v5 | `v5.7.6` | `a2fca037434a0a7096b095d4ed87cdffb03b626e` | MIT | Go PostgreSQL 驱动/连接池 | `APPROVED_GO_1_23` |
| sqlc | `v1.31.1` | `a95e91d70ad9e1181253c333a1cfdd75ae4b95a5` | MIT | 仅构建时生成器 | `APPROVED_BUILD_ONLY` |
| nats.go | `v1.48.0` | `a0e7b702c6b8ef9f86d09008d8cfcb4623fdd608` | Apache-2.0 | Go 内部消息客户端 | `APPROVED_GO_1_23` |
| chi | `v5.3.1` | `8b258c7bb28f97a5f2a856ff7ef962578fec9215` | MIT | Go HTTP 路由 | `APPROVED_GO_1_23` |
| opentelemetry-go | `v1.38.0` | `84e3f3ac8b25204f3a0f77a805437a5e08573b35` | Apache-2.0 | Go 遥测 API/SDK | `APPROVED_GO_1_23` |
| testcontainers-go | `v0.38.0` | `41bd60184bf156af3dbb7f5b7fbb58da3603ada4` | MIT | 仅开发/集成测试 | `APPROVED_TEST_ONLY_GO_1_23` |
| Tauri | `tauri-v2.11.5` | `7cd71369c00978a3783b6ae3e9972358abbe4ae6` | Apache-2.0 OR MIT | 桌面壳 | `APPROVED_DESKTOP_WITH_CAPABILITY_REVIEW` |
| Hummingbot | `v2.15.0` | `816b8ab539360557cee7d9248c2f24473b10b16f` | Apache-2.0 | 仅参考 | `REFERENCE_ONLY` |
| CCXT | `v4.5.70` | `48410ae2a126d6bc09845432705b2ef41010da34` | MIT | 仅参考 | `REFERENCE_ONLY` |
| Freqtrade | `2026.6` | `b604e2fd70539f7f73d3c62c16ce0b155bbab319` | GPL-3.0 | 禁止复制或引入 | `PROHIBITED` |

### Go 1.23 兼容性选择

以下不是“旧版遗漏”，而是按仓库当前 Go 基线主动冻结的最高兼容版本：

| 依赖 | 本文固定版本 | 固定版本的 `go` 指令 | 核验时上游最新版 | 上游最新版门槛 |
|---|---:|---:|---:|---:|
| pgx v5 | `v5.7.6` | `1.23.0` | `v5.9.2` | `1.25.0` |
| nats.go | `v1.48.0` | `1.23.0` | `v1.52.0` | `1.25.0` |
| opentelemetry-go | `v1.38.0` | `1.23.0` | `v1.44.0` | `1.25.0` |
| testcontainers-go | `v0.38.0` | `1.23.0` | `v0.43.0` | `1.25.0` |

`chi v5.3.1` 自身声明 `go 1.23`，可直接进入当前基线。`sqlc v1.31.1`
是构建工具，其源码构建要求更高工具链；当前基线只能使用官方 Release 的
预编译资产并核对 GitHub Release 摘要，不能用当前 Go 工具链临时从源码安装。

## 4. 运行时和构建依赖

### 4.1 hyperliquid-python-sdk

- 官方来源：[仓库](https://github.com/hyperliquid-dex/hyperliquid-python-sdk)、
  [0.24.0 发布](https://github.com/hyperliquid-dex/hyperliquid-python-sdk/releases/tag/0.24.0)、
  [固定版本许可证](https://github.com/hyperliquid-dex/hyperliquid-python-sdk/blob/0.24.0/LICENSE.md)。
- 允许：`Info` 查询、公开/账户只读数据结构、订阅消息类型、精度与官方请求
  形状；当前阶段只允许 Fake Exchange 和显著标识的模拟执行。
- 禁止：Hermes 或客户端导入 `Exchange` 后直接下单；加载钱包或签名材料；
  复制示例配置中的密钥流程；把 SDK WebSocket 回调当成交易权威状态；在当前
  阶段调用任何交易所写端点。
- 安全边界：SDK 只能位于 Hyperliquid 适配层；Hermes 不能访问该层的写对象，
  签名执行器也不能因本准入记录而启用。

固定版本的
[`WebsocketManager`](https://github.com/hyperliquid-dex/hyperliquid-python-sdk/blob/0.24.0/hyperliquid/websocket_manager.py#L77-L162)
只构造一次 `WebSocketApp`、调用一次 `run_forever()`，且没有
`on_close`/`on_error` 恢复、订阅重放或 REST 核对流程。因此官方 WebSocket
客户端不能单独满足本系统可靠性要求。我们的适配器必须补：

1. 带抖动的有界指数退避、连接代际和显式 `STALE`/`RECONCILING` 状态；
2. 心跳超时、断线检测、重新订阅和重复消息幂等处理；
3. 重连后以 REST 查询订单、成交、仓位和保护状态，核对成功前禁止增加风险；
4. 发现未知执行结果时关闭失败并进入人工/确定性核对，不从回调推断成功。

升级方式：只接受上游正式 release；先比较 `Info`/`Exchange`/WebSocket、
精度和签名相关差异，再跑只读集成与断线/乱序/重复/缺消息故障测试。升级不
自动授权写 API。

### 4.2 Lightweight Charts

- 官方来源：[仓库](https://github.com/tradingview/lightweight-charts)、
  [v5.2.0 发布](https://github.com/tradingview/lightweight-charts/releases/tag/v5.2.0)、
  [固定版本 LICENSE](https://github.com/tradingview/lightweight-charts/blob/v5.2.0/LICENSE)、
  [固定版本 NOTICE](https://github.com/tradingview/lightweight-charts/blob/v5.2.0/NOTICE)。
- 允许：K 线、成交量、订单/保护标记和经过审查的图表 primitive/plugin API。
- 禁止：从图表状态生成服务端订单状态；引入 TradingView 的其他专有库；
  隐藏模拟数据标识；把浏览器数字精度用于交易金额计算。
- NOTICE/署名：发布的网页和 Tauri 应用必须保留 Apache-2.0 许可证要求的
  NOTICE，用户可见页面必须显示 NOTICE 中的 TradingView 归属，并链接
  [TradingView](https://www.tradingview.com/)。上游 README 说明可使用
  `attributionLogo` 满足链接要求；同时必须保留其声明的 tslib BSD Zero
  Clause 归属。不能把署名留到上线后再补。

升级方式：锁定精确 npm 版本和 lockfile 完整性值；核对 LICENSE、NOTICE、
tslib 归属、渲染 API 和插件 API 变化，并做浏览器与 Tauri 截图/交互回归。

### 4.3 NousResearch/hermes-agent

- 官方来源：[仓库](https://github.com/NousResearch/hermes-agent)、
  [v2026.7.20 发布](https://github.com/NousResearch/hermes-agent/releases/tag/v2026.7.20)、
  [固定版本 README](https://github.com/NousResearch/hermes-agent/blob/v2026.7.20/README.md)、
  [固定版本许可证](https://github.com/NousResearch/hermes-agent/blob/v2026.7.20/LICENSE)。
- 允许：模型路由、受控 Agent loop、提示词/意图翻译、经过审查的 MCP
  工具适配和离线评估模式。
- 禁止：上游通用终端、Shell、SSH、Docker socket、浏览器自动化、任意 MCP、
  自动安装技能、无人值守 cron、消息平台网关、自由网络访问、读取主机秘密，
  以及任何签名、下单、转账、提现、SQL 或风险配置修改能力。
- OS 级隔离是强制项：独立非特权 UID 和容器/沙箱；只读根文件系统；按任务
  挂载最小目录；默认拒绝网络，只放行批准的模型端点和受控 Go MCP 网关；
  无宿主 Shell/SSH/Docker socket；CPU、内存、进程数和执行时间受限。
- Hermes 只能提出结构化意图。身份由服务端注入；Go 核心执行 schema、确认
  哈希、风险和状态机校验。Hermes 自由文本不能形成“已确认”“已下单”或
  “已保护”状态，不能直接接触 SDK 的签名/订单对象。

上游 README 明确包含本地、Docker、SSH 等终端后端、定时任务、消息网关和
自动技能能力，所以应用内工具白名单不能替代 OS 隔离。

升级方式：固定 tag/commit 和 Python lockfile；逐项比较新增工具、默认开启
能力、网络端点、迁移与持久化行为。任何新增工具默认为拒绝，MCP inventory、
隔离逃逸和提示注入测试通过后才能升级。

### 4.4 pgx v5

- 官方来源：[仓库](https://github.com/jackc/pgx)、
  [固定 tag](https://github.com/jackc/pgx/tree/v5.7.6)、
  [许可证](https://github.com/jackc/pgx/blob/v5.7.6/LICENSE)。
- 允许：`pgxpool`、参数化查询、事务和 PostgreSQL 类型。
- 禁止：Hermes/客户端直连数据库、超级用户 DSN、字符串拼接 SQL、把数据库
  连接信息写入日志、绕过服务端租户和账户范围。
- 升级：保持 `/v5` 主版本；先提高独立 Go 基线并验证迁移、并发事务、连接
  中断和恢复，再考虑 `v5.8+`。

### 4.5 sqlc

- 官方来源：[仓库](https://github.com/sqlc-dev/sqlc)、
  [v1.31.1 发布](https://github.com/sqlc-dev/sqlc/releases/tag/v1.31.1)、
  [许可证](https://github.com/sqlc-dev/sqlc/blob/v1.31.1/LICENSE)。
- 允许：仅在开发/CI 中从经过审查的 SQL 生成 Go 查询代码。
- 禁止：生产运行时依赖、在线下载插件、执行模型生成的 SQL、允许 Hermes
  改查询文件、未经审查直接接受生成代码差异。
- 升级：使用官方 Release 资产并核对 GitHub Release 的 SHA-256 摘要；固定
  `sqlc` 配置版本，对生成结果做干净树/确定性差异检查，再跑数据库集成测试。

### 4.6 nats.go

- 官方来源：[仓库](https://github.com/nats-io/nats.go)、
  [v1.48.0 发布](https://github.com/nats-io/nats.go/releases/tag/v1.48.0)、
  [许可证](https://github.com/nats-io/nats.go/blob/v1.48.0/LICENSE)。
- 允许：内部 request/reply、发布订阅、经过设计评审的 JetStream durable
  consumer，以及连接/断线状态回调。
- 禁止：公网暴露 NATS、把消息流当成订单权威库、在消息中携带签名材料、无
  上限重试、没有幂等键的重复消费、重连期间继续增加风险。
- 升级：核对重连、drain、JetStream ack/redelivery 语义，跑断线、重复、乱序
  和消费者重启测试。`v1.49+` 必须先独立提高 Go 基线。

### 4.7 chi

- 官方来源：[仓库](https://github.com/go-chi/chi)、
  [v5.3.1 发布](https://github.com/go-chi/chi/releases/tag/v5.3.1)、
  [许可证](https://github.com/go-chi/chi/blob/v5.3.1/LICENSE)。
- 允许：路由、middleware 组合和 URL 参数解析。
- 禁止：把认证/授权交给路由匹配、公开 `pprof` 或调试路由、记录认证头和
  请求体、允许未限制的请求体或超时。
- 升级：保持 `/v5`，复核中间件顺序、路径匹配、安全头、请求大小和超时测试。

### 4.8 opentelemetry-go

- 官方来源：[仓库](https://github.com/open-telemetry/opentelemetry-go)、
  [v1.38.0 发布](https://github.com/open-telemetry/opentelemetry-go/releases/tag/v1.38.0)、
  [许可证](https://github.com/open-telemetry/opentelemetry-go/blob/v1.38.0/LICENSE)。
- 允许：经过最小化配置的 trace/metric API、SDK 和内部 exporter。
- 禁止：采集密钥、认证头、完整请求/响应体、Hermes 提示词、个人交易理由或
  高基数字段；自动向公网 exporter 发数据；遥测失败影响交易状态机。
- 升级：先审计属性白名单和 exporter 端点，再验证采样、背压、关闭和脱敏。
  `v1.39+` 必须先独立提高 Go 基线。

### 4.9 testcontainers-go

- 官方来源：[仓库](https://github.com/testcontainers/testcontainers-go)、
  [v0.38.0 发布](https://github.com/testcontainers/testcontainers-go/releases/tag/v0.38.0)、
  [许可证](https://github.com/testcontainers/testcontainers-go/blob/v0.38.0/LICENSE)。
- 允许：仅在开发/CI 启动固定镜像摘要的 PostgreSQL、NATS 和故障注入容器。
- 禁止：生产编译/运行时依赖、Hermes 访问 Docker socket、浮动镜像 tag、
  使用真实凭证/账户、把容器集成测试通过解释为实盘授权。
- 升级：审核 Docker API 和清理行为，锁定测试镜像 digest，验证失败清理和
  并行隔离。`v0.39+` 必须先独立提高 Go 基线。

### 4.10 Tauri

- 官方来源：[仓库](https://github.com/tauri-apps/tauri)、
  [tauri-v2.11.5 发布](https://github.com/tauri-apps/tauri/releases/tag/tauri-v2.11.5)、
  [workspace 许可证声明](https://github.com/tauri-apps/tauri/blob/tauri-v2.11.5/Cargo.toml#L39-L44)、
  [Apache-2.0](https://github.com/tauri-apps/tauri/blob/tauri-v2.11.5/LICENSE_APACHE-2.0)、
  [MIT](https://github.com/tauri-apps/tauri/blob/tauri-v2.11.5/LICENSE_MIT)。
- 允许：桌面窗口、受控 IPC、系统设备认证桥接和明确列出的最小 capability。
- 禁止：默认开启 shell、process、filesystem、global HTTP 或 updater 权限；
  WebView 读取密钥；把签名器做成 sidecar；远程内容获得本地 command 权限；
  未固定和未签名的更新。
- 升级：Rust crate 与 JavaScript 包保持同一固定 release；逐项 diff capability
  和 CSP，做命令参数校验、路径穿越、远程内容、更新签名和桌面 UI 回归。

## 5. 仅参考与禁止项

### 5.1 Hummingbot：仅参考

- 官方来源：[仓库](https://github.com/hummingbot/hummingbot)、
  [v2.15.0 发布](https://github.com/hummingbot/hummingbot/releases/tag/v2.15.0)、
  [许可证](https://github.com/hummingbot/hummingbot/blob/v2.15.0/LICENSE)。
- 可参考：连接器职责划分、WebSocket/REST 双路径测试场景、订单生命周期边界
  情况清单。
- 不可使用：不能作为运行时依赖，不能复制认证、签名、下单、费用、builder、
  重连或策略实现；不能用其状态替代本系统服务端权威状态。

特别禁止复制默认 builder fee。固定版本的
[`hyperliquid_perpetual_constants.py`](https://github.com/hummingbot/hummingbot/blob/v2.15.0/hummingbot/connector/derivative/hyperliquid_perpetual/hyperliquid_perpetual_constants.py#L9-L23)
默认启用 Foundation builder 支持，并设置每单 `10` 个十分之一基点（即
`1 bp`）；其
[`_place_order`](https://github.com/hummingbot/hummingbot/blob/v2.15.0/hummingbot/connector/derivative/hyperliquid_perpetual/hyperliquid_perpetual_derivative.py#L630-L691)
会在满足条件时加入 `builder` 字段。本项目不能复制 builder 地址、默认费用、
注入逻辑或用户批准流程。未来如要支持 builder，必须另立产品、法律、安全、
用户明确同意和零默认费用评审；本任务不授权。

### 5.2 CCXT：仅参考

- 官方来源：[仓库](https://github.com/ccxt/ccxt)、
  [v4.5.70 发布](https://github.com/ccxt/ccxt/releases/tag/v4.5.70)、
  [许可证](https://github.com/ccxt/ccxt/blob/v4.5.70/LICENSE.txt)。
- 可参考：交易所错误分类、市场/订单字段命名和跨交易所边界情况清单。
- 不可使用：不能成为运行时依赖或 Hyperliquid 行为权威，不能复制签名/请求
  代码，不能因此扩展到多交易所，也不能动态加载其打包后的交易所实现。

### 5.3 Freqtrade：禁止复制或引入

- 官方来源：[仓库](https://github.com/freqtrade/freqtrade)、
  [2026.6 发布](https://github.com/freqtrade/freqtrade/releases/tag/2026.6)、
  [GPL-3.0 许可证](https://github.com/freqtrade/freqtrade/blob/2026.6/LICENSE)。
- `PROHIBITED`：禁止加入依赖、vendor、submodule、容器镜像；禁止复制代码
  片段、策略、测试、配置、文档或资产；禁止改写后引入。
- 原因：本项目决定避免 GPL 代码耦合及与自身交易安全模型混合。该决定不是
  对 GPL 或项目质量的判断。通用概念必须从协议/官方 API 等独立一手来源
  重新设计，并保留清洁实现证据。

## 6. 后续集成门

1. 使用方在自己的有界任务中加入精确 lock、许可证/NOTICE 归档和最小测试。
2. Hyperliquid 只读适配器先证明断线、重连、重复、乱序、缺失消息和 REST
   核对；在此之前不得进入写路径。
3. Hermes 先证明 OS 隔离和固定十工具 MCP inventory；不能因采用上游运行时
   而扩大工具面。
4. Lightweight Charts 的 NOTICE、TradingView 可见归属和链接必须随首个
   可分发构建进入 UI 验收。
5. 每个集成候选 commit 必须接受独立固定 commit 评审。评审通过仍不授权
   合并、部署、生产、钱包、签名或真实交易。
