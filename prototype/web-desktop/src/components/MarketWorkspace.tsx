import {
  ArrowRight,
  Camera,
  CaretDown,
  ChartBar,
  ChartBarHorizontal,
  CornersOut,
  ShieldCheck,
  TrendUp,
} from "@phosphor-icons/react";
import {
  CandlestickSeries,
  ColorType,
  createChart,
  CrosshairMode,
  HistogramSeries,
  LineStyle,
} from "lightweight-charts";
import { useEffect, useRef } from "react";
import {
  buildChartData,
  formatPrice,
  type SymbolProfile,
  type Timeframe,
} from "../market-data";

const bottomTabs = [
  "持仓 (1)",
  "当前委托 (3)",
  "成交记录",
  "条件单 (1)",
  "仓位历史",
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
      title: "SIM MARK",
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
      aria-label={`${profile.pair} ${timeframe} 本地模拟K线图，不是实时行情`}
      data-testid="market-chart"
    />
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
    <section className="market-header" aria-label="本地模拟市场摘要">
      <div className="instrument-quote">
        <button
          className="instrument-name"
          type="button"
          onClick={onCycleSymbol}
          aria-label={`当前 ${profile.pair}，切换 BTC、ETH、SOL 模拟品种`}
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
        <span className="simulated-label">模拟行情</span>
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
        <button className="icon-button active" type="button" aria-label="K线">
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
                <button
                  type="button"
                  onClick={() => onAction("已打开模拟减仓面板。")}
                >
                  减仓
                </button>
                <button
                  type="button"
                  onClick={() => onAction("已打开模拟止损调整。")}
                >
                  止损
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  );

  return (
    <section className="bottom-panel" aria-label="模拟持仓和订单">
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
        {selectedTab === bottomTabs[0] ? (
          positionTable
        ) : (
          <div className="panel-empty">
            <ChartBar size={22} aria-hidden="true" />
            <span>{selectedTab} 的本地模拟详情将在后续页面展开。</span>
          </div>
        )}
      </div>
      <dl className="account-summary">
        <div>
          <dt>模拟账户权益 (USDC)</dt>
          <dd>10,000.00</dd>
        </div>
        <div>
          <dt>模拟可用保证金 (USDC)</dt>
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
          onClick={() => onAction("已打开本地模拟风险详情。")}
        >
          详情 <ArrowRight size={14} aria-hidden="true" />
        </button>
      </dl>
    </section>
  );
}

export function MarketWorkspace({
  profile,
  timeframe,
  indicatorEnabled,
  selectedTab,
  onCycleSymbol,
  onTimeframe,
  onToggleIndicator,
  onTab,
  onAction,
}: {
  profile: SymbolProfile;
  timeframe: Timeframe;
  indicatorEnabled: boolean;
  selectedTab: string;
  onCycleSymbol: () => void;
  onTimeframe: (timeframe: Timeframe) => void;
  onToggleIndicator: () => void;
  onTab: (tab: string) => void;
  onAction: (message: string) => void;
}) {
  return (
    <section className="center-workspace">
      <MarketHeader profile={profile} onCycleSymbol={onCycleSymbol} />
      <section className="chart-section" aria-label="图表">
        <ChartToolbar
          timeframe={timeframe}
          onTimeframe={onTimeframe}
          indicatorEnabled={indicatorEnabled}
          onToggleIndicator={onToggleIndicator}
        />
        <MarketChart profile={profile} timeframe={timeframe} />
      </section>
      <BottomPanel
        profile={profile}
        selectedTab={selectedTab}
        onTab={onTab}
        onAction={onAction}
      />
    </section>
  );
}
