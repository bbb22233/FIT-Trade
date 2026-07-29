import {
  ArrowsClockwise,
  ArrowRight,
  Briefcase,
  Camera,
  CaretDown,
  ChartBar,
  ChartBarHorizontal,
  ChartLineUp,
  CheckCircle,
  CornersOut,
  Database,
  DotsThreeVertical,
  FileText,
  GearSix,
  HandPalm,
  LockSimple,
  PaperPlaneTilt,
  Pulse,
  PushPin,
  ShieldCheck,
  ShieldWarning,
  SquaresFour,
  TrendUp,
  Warning,
  X,
} from "@phosphor-icons/react";
import {
  CandlestickSeries,
  ColorType,
  createChart,
  CrosshairMode,
  HistogramSeries,
  LineStyle,
} from "lightweight-charts";
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type FormEvent,
  type RefObject,
} from "react";
import {
  buildChartData,
  formatPrice,
  symbolProfiles,
  type SymbolKey,
  type SymbolProfile,
  type Timeframe,
} from "./market-data";

type OperationState =
  | "AWAITING_CONFIRMATION"
  | "RISK_REVALIDATING"
  | "DISPATCHED"
  | "RECONCILING"
  | "PROTECTION_PENDING"
  | "PROTECTED"
  | "EXPIRED";

type ChatMessage = {
  id: number;
  role: "you" | "hermes";
  content: string;
};

const operationLabels: Record<OperationState, string> = {
  AWAITING_CONFIRMATION: "等待你的确认",
  RISK_REVALIDATING: "服务器重新验证风险",
  DISPATCHED: "已派发至执行链路",
  RECONCILING: "正在核对成交与仓位",
  PROTECTION_PENDING: "正在建立 Stop Market",
  PROTECTED: "成交与止损均已核对",
  EXPIRED: "确认已过期",
};

const defaultMessages: ChatMessage[] = [
  {
    id: 1,
    role: "hermes",
    content:
      "已读取 BTC 的 1h / 4h 已收盘数据、账户风险和现有持仓。你可以继续讨论，或发出明确的实盘交易指令。",
  },
  {
    id: 2,
    role: "you",
    content: "BTC 5倍做多，止损放在1小时前低，仓位你计算。",
  },
  {
    id: 3,
    role: "hermes",
    content:
      "已识别为增加风险的实盘指令。Go 风控根据止损距离计算为 0.025 BTC；必须由你核对结构化确认票据。",
  },
];

const navItems = [
  { label: "概览", icon: SquaresFour },
  { label: "市场", icon: ChartLineUp },
  { label: "仓位", icon: Briefcase },
  { label: "订单", icon: FileText },
  { label: "风险", icon: ShieldCheck },
  { label: "策略", icon: Pulse },
  { label: "数据", icon: Database },
  { label: "设置", icon: GearSix },
];

function MarketChart({
  profile,
  timeframe,
}: {
  profile: SymbolProfile;
  timeframe: Timeframe;
}) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const chart = createChart(container, {
      width: container.clientWidth,
      height: container.clientHeight,
      layout: {
        attributionLogo: true,
        background: { type: ColorType.Solid, color: "#0d151d" },
        textColor: "#7d8c9b",
        fontFamily: "Inter, system-ui, sans-serif",
        fontSize: 11,
      },
      grid: {
        vertLines: { color: "#17232e" },
        horzLines: { color: "#17232e" },
      },
      crosshair: {
        mode: CrosshairMode.Normal,
        vertLine: { color: "#526270", labelBackgroundColor: "#253441" },
        horzLine: { color: "#526270", labelBackgroundColor: "#253441" },
      },
      rightPriceScale: {
        borderColor: "#25303c",
        scaleMargins: { top: 0.08, bottom: 0.22 },
      },
      timeScale: {
        borderColor: "#25303c",
        timeVisible: true,
        secondsVisible: false,
        rightOffset: 3,
        barSpacing: timeframe === "1h" ? 8 : 10,
      },
      localization: {
        priceFormatter: (price: number) => formatPrice(profile, price),
      },
    });

    const { candles, volume } = buildChartData(profile, timeframe);
    const candleSeries = chart.addSeries(CandlestickSeries, {
      upColor: "#2bc77a",
      downColor: "#f05d64",
      wickUpColor: "#2bc77a",
      wickDownColor: "#f05d64",
      borderVisible: false,
      priceLineVisible: false,
      lastValueVisible: true,
    });
    candleSeries.setData(candles);
    candleSeries.createPriceLine({
      price: profile.price,
      color: "#2bc77a",
      lineWidth: 1,
      lineStyle: LineStyle.Dashed,
      axisLabelVisible: true,
      title: "MARK",
    });

    const volumeSeries = chart.addSeries(HistogramSeries, {
      priceFormat: { type: "volume" },
      priceScaleId: "",
      lastValueVisible: false,
      priceLineVisible: false,
    });
    volumeSeries.priceScale().applyOptions({
      scaleMargins: { top: 0.82, bottom: 0 },
    });
    volumeSeries.setData(volume);
    chart.timeScale().fitContent();

    const observer = new ResizeObserver(([entry]) => {
      const { width, height } = entry.contentRect;
      chart.applyOptions({
        width: Math.max(1, Math.floor(width)),
        height: Math.max(1, Math.floor(height)),
      });
    });
    observer.observe(container);

    return () => {
      observer.disconnect();
      chart.remove();
    };
  }, [profile, timeframe]);

  return (
    <div
      className="chart-canvas"
      ref={containerRef}
      role="img"
      aria-label={`${profile.pair} ${timeframe} 示例K线图，数据不是实时行情`}
    />
  );
}

function TopBar({
  safetyPaused,
  operationState,
  onKillSwitch,
}: {
  safetyPaused: boolean;
  operationState: OperationState;
  onKillSwitch: () => void;
}) {
  const protectionText =
    operationState === "PROTECTION_PENDING"
      ? "PROTECTION PENDING"
      : operationState === "PROTECTED"
        ? "PROTECTED"
        : "PROTECTED";

  return (
    <header className="top-bar">
      <div className="brand-block">Hermes Command</div>
      <div className="system-track" aria-label="系统状态">
        <div className="system-item mainnet">
          <span className="status-dot" aria-hidden="true" />
          MAINNET
        </div>
        <div className="system-item live">
          <span className="status-dot" aria-hidden="true" />
          LIVE
        </div>
        <div className="system-item protected">
          <ShieldCheck size={17} weight="regular" aria-hidden="true" />
          {protectionText}
        </div>
        <div className="system-item automation">
          <span className="status-dot" aria-hidden="true" />
          自动交易：{safetyPaused ? "安全暂停" : "关闭"}
        </div>
      </div>
      <button
        className={`kill-button ${safetyPaused ? "is-active" : ""}`}
        type="button"
        onClick={onKillSwitch}
        disabled={safetyPaused}
      >
        <HandPalm size={18} weight="bold" aria-hidden="true" />
        {safetyPaused ? "已停止增加风险" : "停止开仓和增加风险"}
      </button>
      <div className="clock">2026-07-28&nbsp;&nbsp;14:33:18&nbsp;&nbsp;UTC+8</div>
    </header>
  );
}

function LeftNavigation({
  active,
  onSelect,
}: {
  active: string;
  onSelect: (label: string) => void;
}) {
  return (
    <nav className="left-nav" aria-label="主要功能">
      <div className="nav-items">
        {navItems.map(({ label, icon: Icon }) => (
          <button
            className={`nav-item ${active === label ? "active" : ""}`}
            type="button"
            key={label}
            onClick={() => onSelect(label)}
            aria-current={active === label ? "page" : undefined}
          >
            <Icon size={22} weight="regular" aria-hidden="true" />
            <span>{label}</span>
          </button>
        ))}
      </div>
      <button
        className="nav-collapse"
        type="button"
        aria-label="收起导航"
      >
        ‹‹
      </button>
    </nav>
  );
}

function MarketHeader({
  profile,
  onCycleSymbol,
}: {
  profile: SymbolProfile;
  onCycleSymbol: () => void;
}) {
  return (
    <section className="market-header" aria-label="市场摘要">
      <div className="instrument-quote">
        <button
          className="instrument-name"
          type="button"
          onClick={onCycleSymbol}
          aria-label={`当前 ${profile.pair}，切换交易品种`}
        >
          {profile.pair}
          <CaretDown size={14} aria-hidden="true" />
        </button>
        <span className="instrument-price">
          {formatPrice(profile, profile.price)}
        </span>
        <span className="quote-up">
          +{formatPrice(profile, profile.change)} &nbsp;+
          {profile.changePercent.toFixed(2)}%
        </span>
      </div>
      <dl className="market-stats">
        <div>
          <dt>24h 高</dt>
          <dd>{formatPrice(profile, profile.high24h)}</dd>
        </div>
        <div>
          <dt>24h 低</dt>
          <dd>{formatPrice(profile, profile.low24h)}</dd>
        </div>
        <div>
          <dt>24h 量 (USD)</dt>
          <dd>{profile.volume24h}</dd>
        </div>
        <div>
          <dt>资金费率 / 8h</dt>
          <dd className="quote-up">{profile.funding}</dd>
        </div>
        <div>
          <dt>持仓量 (USD)</dt>
          <dd>{profile.openInterest}</dd>
        </div>
      </dl>
    </section>
  );
}

function ChartToolbar({
  timeframe,
  onTimeframe,
  indicatorEnabled,
  onToggleIndicator,
}: {
  timeframe: Timeframe;
  onTimeframe: (timeframe: Timeframe) => void;
  indicatorEnabled: boolean;
  onToggleIndicator: () => void;
}) {
  return (
    <div className="chart-toolbar">
      <div className="toolbar-left">
        {(["1h", "4h"] as Timeframe[]).map((item) => (
          <button
            type="button"
            className={`timeframe-button ${timeframe === item ? "active" : ""}`}
            onClick={() => onTimeframe(item)}
            key={item}
          >
            {item}
          </button>
        ))}
        <span className="toolbar-divider" aria-hidden="true" />
        <button
          type="button"
          className={`toolbar-button ${indicatorEnabled ? "active" : ""}`}
          onClick={onToggleIndicator}
        >
          指标
          <CaretDown size={13} aria-hidden="true" />
        </button>
      </div>
      <div className="toolbar-right">
        <button className="toolbar-button" type="button">
          显示
          <CaretDown size={13} aria-hidden="true" />
        </button>
        <button className="icon-button" type="button" aria-label="趋势线">
          <TrendUp size={18} aria-hidden="true" />
        </button>
        <button
          className="icon-button active"
          type="button"
          aria-label="K线"
        >
          <ChartBarHorizontal size={18} aria-hidden="true" />
        </button>
        <button className="icon-button" type="button" aria-label="截图">
          <Camera size={18} aria-hidden="true" />
        </button>
        <button className="icon-button" type="button" aria-label="全屏">
          <CornersOut size={18} aria-hidden="true" />
        </button>
      </div>
    </div>
  );
}

const bottomTabs = ["持仓 (1)", "当前委托 (3)", "成交记录", "条件单 (1)", "仓位历史"];

function BottomPanel({
  profile,
  selectedTab,
  onTab,
  onAction,
}: {
  profile: SymbolProfile;
  selectedTab: string;
  onTab: (tab: string) => void;
  onAction: (message: string) => void;
}) {
  const positionTable = (
    <div className="table-scroll">
      <table className="positions-table">
        <thead>
          <tr>
            <th>市场</th>
            <th>方向</th>
            <th>仓位</th>
            <th>杠杆</th>
            <th>开仓均价</th>
            <th>标记价格</th>
            <th>未实现盈亏</th>
            <th>保证金</th>
            <th>保护</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>{profile.pair}</td>
            <td className="quote-up">多</td>
            <td>{profile.quantity}</td>
            <td>5x 逐仓</td>
            <td>{formatPrice(profile, profile.entry * 0.985)}</td>
            <td>{formatPrice(profile, profile.price)}</td>
            <td className="quote-up">+27.55 USDC</td>
            <td>{profile.margin}</td>
            <td>
              <span className="protection-badge">
                <ShieldCheck size={14} aria-hidden="true" />
                PROTECTED
              </span>
            </td>
            <td>
              <div className="table-actions">
                <button type="button" onClick={() => onAction("已打开减仓面板")}>
                  减仓
                </button>
                <button type="button" onClick={() => onAction("已打开止损调整")}>
                  止损
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  );

  const emptyState = (
    <div className="panel-empty">
      <ChartBar size={22} aria-hidden="true" />
      <span>{selectedTab} 的示例详情将在后续页面展开。</span>
    </div>
  );

  return (
    <section className="bottom-panel" aria-label="持仓和订单">
      <div className="bottom-tabs">
        {bottomTabs.map((tab) => (
          <button
            type="button"
            className={selectedTab === tab ? "active" : ""}
            onClick={() => onTab(tab)}
            key={tab}
          >
            {tab}
          </button>
        ))}
      </div>
      <div className="bottom-content">
        {selectedTab === bottomTabs[0] ? positionTable : emptyState}
      </div>
      <dl className="account-summary">
        <div>
          <dt>账户权益 (USDC)</dt>
          <dd>10,000.00</dd>
        </div>
        <div>
          <dt>可用保证金 (USDC)</dt>
          <dd>8,924.40</dd>
        </div>
        <div>
          <dt>已用风险</dt>
          <dd>0.55%</dd>
        </div>
        <div>
          <dt>交易后总风险</dt>
          <dd className="risk-value">{profile.totalRiskPercent}</dd>
        </div>
        <button
          type="button"
          className="summary-link"
          onClick={() => onAction("已打开风险详情")}
        >
          详情 <ArrowRight size={14} aria-hidden="true" />
        </button>
      </dl>
    </section>
  );
}

function ConfirmationTicket({
  profile,
  operationState,
  expiresIn,
  safetyPaused,
  onConfirm,
  onEdit,
}: {
  profile: SymbolProfile;
  operationState: OperationState;
  expiresIn: number;
  safetyPaused: boolean;
  onConfirm: () => void;
  onEdit: () => void;
}) {
  const isProcessing = [
    "RISK_REVALIDATING",
    "DISPATCHED",
    "RECONCILING",
    "PROTECTION_PENDING",
  ].includes(operationState);
  const isFinal = operationState === "PROTECTED";
  const isExpired = operationState === "EXPIRED";
  const minutes = String(Math.floor(expiresIn / 60)).padStart(2, "0");
  const seconds = String(expiresIn % 60).padStart(2, "0");

  return (
    <section className="confirmation-ticket" aria-labelledby="confirmation-title">
      <div className="ticket-header">
        <div>
          <span className="ticket-kicker">MAINNET · 增加风险 · 不可修改</span>
          <h2 id="confirmation-title">{profile.symbol} 开仓确认</h2>
        </div>
        <span className="side-label">买入开多</span>
      </div>

      <div className="ticket-warning">
        <Warning size={17} weight="regular" aria-hidden="true" />
        <span>{operationLabels[operationState]}</span>
        <span className="expiry">
          {isExpired ? "已过期" : `${minutes}:${seconds}`}
        </span>
      </div>

      <div className="risk-ledger">
        <div className="ledger-row critical">
          <span>最大亏损</span>
          <strong>
            {profile.maxLoss}
            <small>{profile.maxLossPercent}</small>
          </strong>
        </div>
        <div className="ledger-row critical">
          <span>交易后总风险</span>
          <strong>
            {profile.totalRisk}
            <small>{profile.totalRiskPercent}</small>
          </strong>
        </div>
        <div className="ledger-row">
          <span>Stop Market</span>
          <strong>{formatPrice(profile, profile.stop)}</strong>
        </div>
        <div className="ledger-row">
          <span>强平价</span>
          <strong>{formatPrice(profile, profile.liquidation)}</strong>
        </div>
      </div>

      <div className="order-ledger">
        <div>
          <span>数量 / 名义价值</span>
          <strong>
            {profile.quantity} / {profile.notional}
          </strong>
        </div>
        <div>
          <span>杠杆 / 保证金</span>
          <strong>5x 逐仓 / {profile.margin}</strong>
        </div>
        <div>
          <span>参考 / 最差成交</span>
          <strong>
            {formatPrice(profile, profile.entry)} /{" "}
            {formatPrice(profile, profile.worstFill)}
          </strong>
        </div>
        <div>
          <span>费用与滑点预算</span>
          <strong>{profile.feeBudget}</strong>
        </div>
      </div>

      <p className="ticket-footnote">
        以上由服务器按当前市场、账户与 RISK-v1 计算。任一风险字段变化都会使本确认失效。
      </p>

      {isProcessing || isFinal ? (
        <div className={`operation-state ${isFinal ? "complete" : ""}`}>
          {isFinal ? (
            <CheckCircle size={18} weight="fill" aria-hidden="true" />
          ) : (
            <ArrowsClockwise size={18} aria-hidden="true" />
          )}
          <div>
            <strong>{operationLabels[operationState]}</strong>
            <span>
              {isFinal
                ? "服务端已核对成交、仓位和 Stop Market。"
                : "请勿重复操作，系统会先查询真实结果。"}
            </span>
          </div>
        </div>
      ) : null}

      <div className="ticket-actions">
        <button
          type="button"
          className="ticket-secondary"
          onClick={onEdit}
          disabled={isProcessing}
        >
          返回修改
        </button>
        <button
          type="button"
          className="confirm-button"
          onClick={onConfirm}
          disabled={
            safetyPaused ||
            isProcessing ||
            isFinal ||
            isExpired
          }
        >
          {safetyPaused ? (
            <>
              <ShieldWarning size={17} aria-hidden="true" />
              安全暂停：禁止增加风险
            </>
          ) : isFinal ? (
            <>
              <ShieldCheck size={17} aria-hidden="true" />
              已成交并确认保护
            </>
          ) : isProcessing ? (
            <>
              <ArrowsClockwise size={17} aria-hidden="true" />
              处理中，请勿重复操作
            </>
          ) : isExpired ? (
            "确认已过期，请重新生成"
          ) : (
            <>
              <LockSimple size={17} aria-hidden="true" />
              确认主网实盘开仓
            </>
          )}
        </button>
      </div>
    </section>
  );
}

function HermesRail({
  profile,
  messages,
  thinking,
  confirmationVisible,
  operationState,
  expiresIn,
  safetyPaused,
  inputRef,
  onSubmit,
  onConfirm,
  onEdit,
}: {
  profile: SymbolProfile;
  messages: ChatMessage[];
  thinking: boolean;
  confirmationVisible: boolean;
  operationState: OperationState;
  expiresIn: number;
  safetyPaused: boolean;
  inputRef: RefObject<HTMLInputElement | null>;
  onSubmit: (value: string) => void;
  onConfirm: () => void;
  onEdit: () => void;
}) {
  const [input, setInput] = useState("");

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    const trimmed = input.trim();
    if (!trimmed) return;
    onSubmit(trimmed);
    setInput("");
  };

  return (
    <aside className="hermes-rail" aria-label="Hermes 交易助手">
      <div className="hermes-header">
        <div>
          <Pulse size={20} weight="duotone" aria-hidden="true" />
          <span>Hermes</span>
        </div>
        <button className="icon-button" type="button" aria-label="固定Hermes面板">
          <PushPin size={17} aria-hidden="true" />
        </button>
      </div>
      <div className="conversation">
        {messages.map((message) => (
          <article className={`chat-message ${message.role}`} key={message.id}>
            <div className="message-meta">
              <strong>{message.role === "you" ? "你" : "Hermes"}</strong>
              <span>14:{message.id === 1 ? "30" : "31"}</span>
            </div>
            <p>{message.content}</p>
          </article>
        ))}
        {thinking ? (
          <div className="thinking-row">
            <ArrowsClockwise size={16} aria-hidden="true" />
            Hermes 正在生成结构化意图…
          </div>
        ) : null}
      </div>

      {confirmationVisible ? (
        <ConfirmationTicket
          profile={profile}
          operationState={operationState}
          expiresIn={expiresIn}
          safetyPaused={safetyPaused}
          onConfirm={onConfirm}
          onEdit={onEdit}
        />
      ) : (
        <div className="no-confirmation">
          <ShieldCheck size={20} aria-hidden="true" />
          <strong>当前没有待确认的增加风险操作</strong>
          <span>模型文本不能直接触发签名或下单。</span>
        </div>
      )}

      <form className="chat-composer" onSubmit={handleSubmit}>
        <label htmlFor="hermes-command" className="sr-only">
          向 Hermes 发送消息
        </label>
        <input
          id="hermes-command"
          ref={inputRef}
          value={input}
          onChange={(event) => setInput(event.target.value)}
          placeholder="向 Hermes 发送指令或问题…"
          disabled={thinking}
        />
        <button type="submit" aria-label="发送" disabled={!input.trim() || thinking}>
          <PaperPlaneTilt size={18} weight="fill" aria-hidden="true" />
        </button>
      </form>
    </aside>
  );
}

function StatusFooter({ safetyPaused }: { safetyPaused: boolean }) {
  return (
    <footer className="status-footer">
      <div>
        <span>连接状态</span>
        <strong className="quote-up">
          <span className="status-dot" aria-hidden="true" /> 已连接
        </strong>
      </div>
      <div>
        <span>延迟</span>
        <strong className="quote-up">24ms</strong>
      </div>
      <div>
        <span>区块高度</span>
        <strong>31,542,871</strong>
      </div>
      <div>
        <span>资金费率下次结算</span>
        <strong>03:27:42</strong>
      </div>
      <div className="footer-right">
        <span className="status-dot" aria-hidden="true" />
        <strong>{safetyPaused ? "安全暂停 / 保护继续" : "线路 A / 稳定"}</strong>
      </div>
    </footer>
  );
}

export function App() {
  const [symbol, setSymbol] = useState<SymbolKey>("BTC");
  const [timeframe, setTimeframe] = useState<Timeframe>("1h");
  const [indicatorEnabled, setIndicatorEnabled] = useState(true);
  const [activeNav, setActiveNav] = useState("仓位");
  const [bottomTab, setBottomTab] = useState(bottomTabs[0]);
  const [messages, setMessages] = useState(defaultMessages);
  const [thinking, setThinking] = useState(false);
  const [confirmationVisible, setConfirmationVisible] = useState(true);
  const [operationState, setOperationState] =
    useState<OperationState>("AWAITING_CONFIRMATION");
  const [expiresIn, setExpiresIn] = useState(600);
  const [killDialogOpen, setKillDialogOpen] = useState(false);
  const [safetyPaused, setSafetyPaused] = useState(false);
  const [toast, setToast] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const timersRef = useRef<number[]>([]);
  const profile = useMemo(() => symbolProfiles[symbol], [symbol]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      setExpiresIn((current) => {
        if (
          current <= 1 &&
          operationState === "AWAITING_CONFIRMATION"
        ) {
          setOperationState("EXPIRED");
          return 0;
        }
        if (operationState !== "AWAITING_CONFIRMATION") return current;
        return Math.max(0, current - 1);
      });
    }, 1_000);
    return () => window.clearInterval(timer);
  }, [operationState]);

  useEffect(
    () => () => {
      timersRef.current.forEach((timer) => window.clearTimeout(timer));
    },
    [],
  );

  useEffect(() => {
    if (!toast) return;
    const timer = window.setTimeout(() => setToast(""), 2_400);
    return () => window.clearTimeout(timer);
  }, [toast]);

  const resetConfirmation = () => {
    timersRef.current.forEach((timer) => window.clearTimeout(timer));
    timersRef.current = [];
    setOperationState("AWAITING_CONFIRMATION");
    setExpiresIn(600);
    setConfirmationVisible(true);
  };

  const handleSymbol = (next: SymbolKey) => {
    setSymbol(next);
    resetConfirmation();
  };

  const handleConfirm = () => {
    if (
      operationState !== "AWAITING_CONFIRMATION" ||
      safetyPaused
    ) {
      return;
    }
    setOperationState("RISK_REVALIDATING");
    const sequence: Array<[number, OperationState]> = [
      [800, "DISPATCHED"],
      [1_600, "RECONCILING"],
      [2_500, "PROTECTION_PENDING"],
      [3_500, "PROTECTED"],
    ];
    timersRef.current = sequence.map(([delay, state]) =>
      window.setTimeout(() => setOperationState(state), delay),
    );
  };

  const handleEdit = () => {
    if (
      operationState !== "AWAITING_CONFIRMATION" &&
      operationState !== "EXPIRED"
    ) {
      return;
    }
    setConfirmationVisible(false);
    setToast("确认票据已关闭；修改指令后将重新计算全部风险字段。");
    window.setTimeout(() => inputRef.current?.focus(), 0);
  };

  const handleMessage = (content: string) => {
    const nextId = messages.length + 1;
    setMessages((current) => [
      ...current,
      { id: nextId, role: "you", content },
    ]);
    setThinking(true);
    window.setTimeout(() => {
      setMessages((current) => [
        ...current,
        {
          id: current.length + 1,
          role: "hermes",
          content: `已按 ${profile.pair} 当前上下文重新生成结构化意图。Go 风控会重新计算仓位、止损与最大亏损。`,
        },
      ]);
      setThinking(false);
      resetConfirmation();
    }, 700);
  };

  const activateKillSwitch = () => {
    setSafetyPaused(true);
    setKillDialogOpen(false);
    setToast("已停止开仓和增加风险；已有保护、止盈、减仓和平仓继续运行。");
  };

  return (
    <div className="app-shell">
      <TopBar
        safetyPaused={safetyPaused}
        operationState={operationState}
        onKillSwitch={() => setKillDialogOpen(true)}
      />

      {safetyPaused ? (
        <div className="safety-banner" role="alert">
          <ShieldWarning size={17} weight="fill" aria-hidden="true" />
          <strong>安全暂停：</strong>
          已禁止开仓、加仓和其他增加风险操作；已有仓位保护与退出能力继续运行。
        </div>
      ) : null}

      <main className="main-shell">
        <LeftNavigation
          active={activeNav}
          onSelect={(label) => {
            setActiveNav(label);
            if (label !== "仓位") {
              setToast(`${label}页面将在后续原型中展开。`);
            }
          }}
        />

        <section className="center-workspace">
          <MarketHeader
            profile={profile}
            onCycleSymbol={() => {
              const symbols = Object.keys(symbolProfiles) as SymbolKey[];
              const nextIndex = (symbols.indexOf(symbol) + 1) % symbols.length;
              handleSymbol(symbols[nextIndex]);
            }}
          />
          <section className="chart-section" aria-label="图表">
            <ChartToolbar
              timeframe={timeframe}
              onTimeframe={setTimeframe}
              indicatorEnabled={indicatorEnabled}
              onToggleIndicator={() =>
                setIndicatorEnabled((current) => !current)
              }
            />
            <MarketChart profile={profile} timeframe={timeframe} />
          </section>
          <BottomPanel
            profile={profile}
            selectedTab={bottomTab}
            onTab={setBottomTab}
            onAction={setToast}
          />
        </section>

        <HermesRail
          profile={profile}
          messages={messages}
          thinking={thinking}
          confirmationVisible={confirmationVisible}
          operationState={operationState}
          expiresIn={expiresIn}
          safetyPaused={safetyPaused}
          inputRef={inputRef}
          onSubmit={handleMessage}
          onConfirm={handleConfirm}
          onEdit={handleEdit}
        />
      </main>

      <StatusFooter safetyPaused={safetyPaused} />

      {killDialogOpen ? (
        <div className="dialog-backdrop" role="presentation">
          <section
            className="safety-dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby="kill-title"
          >
            <div className="dialog-title">
              <ShieldWarning size={22} weight="fill" aria-hidden="true" />
              <h2 id="kill-title">停止开仓和增加风险</h2>
              <button
                type="button"
                className="icon-button"
                onClick={() => setKillDialogOpen(false)}
                aria-label="关闭"
              >
                <X size={18} aria-hidden="true" />
              </button>
            </div>
            <p>
              立即禁止开仓、加仓、提高杠杆和放宽止损。这个操作不会自动平仓。
            </p>
            <ul>
              <li>
                <CheckCircle size={16} aria-hidden="true" />
                已有 Stop Market、止盈和保护监控继续运行
              </li>
              <li>
                <CheckCircle size={16} aria-hidden="true" />
                reduce-only 减仓和平仓仍然可用
              </li>
            </ul>
            <div className="dialog-actions">
              <button type="button" onClick={() => setKillDialogOpen(false)}>
                取消
              </button>
              <button
                type="button"
                className="danger-action"
                onClick={activateKillSwitch}
              >
                <HandPalm size={17} aria-hidden="true" />
                停止开仓和增加风险
              </button>
            </div>
          </section>
        </div>
      ) : null}

      {toast ? (
        <div className="toast" role="status">
          {toast}
        </div>
      ) : null}
    </div>
  );
}
