import type {
  CandlestickData,
  HistogramData,
  UTCTimestamp,
} from "lightweight-charts";

export type SymbolKey = "BTC" | "ETH" | "SOL";
export type Timeframe = "1h" | "4h";

export type SymbolProfile = {
  symbol: SymbolKey;
  pair: string;
  price: number;
  change: number;
  changePercent: number;
  high24h: number;
  low24h: number;
  volume24h: string;
  funding: string;
  openInterest: string;
  decimals: number;
  quantity: string;
  notional: string;
  margin: string;
  entry: number;
  worstFill: number;
  stop: number;
  liquidation: number;
  maxLoss: string;
  maxLossPercent: string;
  totalRisk: string;
  totalRiskPercent: string;
  feeBudget: string;
};

export const symbolProfiles: Record<SymbolKey, SymbolProfile> = {
  BTC: {
    symbol: "BTC",
    pair: "BTC-PERP",
    price: 118_452,
    change: 1_842.5,
    changePercent: 1.58,
    high24h: 119_240,
    low24h: 116_210,
    volume24h: "3.87B",
    funding: "0.0042%",
    openInterest: "7.62B",
    decimals: 1,
    quantity: "0.025 BTC",
    notional: "2,961.30 USDC",
    margin: "592.26 USDC",
    entry: 118_452,
    worstFill: 118_689,
    stop: 115_200,
    liquidation: 95_420,
    maxLoss: "86.80 USDC",
    maxLossPercent: "0.87%",
    totalRisk: "141.80 USDC",
    totalRiskPercent: "1.42%",
    feeBudget: "5.50 USDC",
  },
  ETH: {
    symbol: "ETH",
    pair: "ETH-PERP",
    price: 3_821.2,
    change: 51.8,
    changePercent: 1.38,
    high24h: 3_864.4,
    low24h: 3_748.6,
    volume24h: "1.42B",
    funding: "0.0038%",
    openInterest: "2.18B",
    decimals: 1,
    quantity: "0.65 ETH",
    notional: "2,483.78 USDC",
    margin: "496.76 USDC",
    entry: 3_821.2,
    worstFill: 3_828.8,
    stop: 3_708,
    liquidation: 3_080,
    maxLoss: "78.10 USDC",
    maxLossPercent: "0.78%",
    totalRisk: "133.10 USDC",
    totalRiskPercent: "1.33%",
    feeBudget: "4.75 USDC",
  },
  SOL: {
    symbol: "SOL",
    pair: "SOL-PERP",
    price: 186.42,
    change: 5.53,
    changePercent: 3.06,
    high24h: 189.2,
    low24h: 178.65,
    volume24h: "884.2M",
    funding: "0.0061%",
    openInterest: "1.08B",
    decimals: 2,
    quantity: "13 SOL",
    notional: "2,423.46 USDC",
    margin: "484.69 USDC",
    entry: 186.42,
    worstFill: 186.79,
    stop: 179.6,
    liquidation: 149.8,
    maxLoss: "92.15 USDC",
    maxLossPercent: "0.92%",
    totalRisk: "147.15 USDC",
    totalRiskPercent: "1.47%",
    feeBudget: "4.60 USDC",
  },
};

type ChartData = {
  candles: CandlestickData<UTCTimestamp>[];
  volume: HistogramData<UTCTimestamp>[];
};

const round = (value: number, decimals: number) =>
  Number(value.toFixed(decimals));

export function buildChartData(
  profile: SymbolProfile,
  timeframe: Timeframe,
): ChartData {
  const points = timeframe === "1h" ? 96 : 72;
  const stepSeconds = timeframe === "1h" ? 3_600 : 14_400;
  const end =
    Math.floor(Date.UTC(2026, 6, 28, 18, 0, 0) / 1000 / stepSeconds) *
    stepSeconds;
  const start = end - points * stepSeconds;
  const raw: Array<{
    time: UTCTimestamp;
    open: number;
    high: number;
    low: number;
    close: number;
    volume: number;
  }> = [];

  const anchors: Array<[number, number]> = [
    [0, 0.91],
    [0.08, 0.925],
    [0.14, 0.9],
    [0.22, 0.94],
    [0.3, 0.964],
    [0.35, 0.947],
    [0.46, 0.936],
    [0.52, 0.976],
    [0.58, 0.961],
    [0.64, 0.922],
    [0.7, 0.94],
    [0.76, 0.966],
    [0.84, 0.972],
    [0.89, 1.004],
    [0.94, 1.012],
    [1, 1],
  ];

  const multiplierAt = (progress: number) => {
    const rightIndex = anchors.findIndex(([position]) => position >= progress);
    if (rightIndex <= 0) return anchors[0][1];
    const [leftPosition, leftValue] = anchors[rightIndex - 1];
    const [rightPosition, rightValue] = anchors[rightIndex];
    const localProgress =
      (progress - leftPosition) / (rightPosition - leftPosition);
    return leftValue + (rightValue - leftValue) * localProgress;
  };

  let cursor = profile.price * anchors[0][1];
  for (let index = 0; index < points; index += 1) {
    const progress = index / (points - 1);
    const open = cursor;
    const close =
      profile.price *
      (multiplierAt(progress) +
        Math.sin(index * 1.73) * 0.0016 +
        Math.cos(index * 0.61) * 0.0008);
    const range =
      profile.price *
      (0.0019 + Math.abs(Math.sin(index * 0.47)) * 0.0024);
    const high = Math.max(open, close) + range;
    const low = Math.min(open, close) - range * 0.88;
    raw.push({
      time: (start + index * stepSeconds) as UTCTimestamp,
      open,
      high,
      low,
      close,
      volume:
        4_500 +
        Math.abs(Math.sin(index * 0.37)) * 13_000 +
        (index % 17 === 0 ? 16_000 : 0) +
        (Math.abs(close - open) / profile.price) * 580_000,
    });
    cursor = close;
  }

  const scale = profile.price / raw[raw.length - 1].close;
  const candles = raw.map((bar) => ({
    time: bar.time,
    open: round(bar.open * scale, profile.decimals),
    high: round(bar.high * scale, profile.decimals),
    low: round(bar.low * scale, profile.decimals),
    close: round(bar.close * scale, profile.decimals),
  }));
  const volume = raw.map((bar) => ({
    time: bar.time,
    value: Math.round(bar.volume),
    color:
      bar.close >= bar.open
        ? "rgba(43, 199, 122, 0.46)"
        : "rgba(240, 93, 100, 0.46)",
  }));

  return { candles, volume };
}

export const formatPrice = (profile: SymbolProfile, value: number) =>
  value.toLocaleString("en-US", {
    minimumFractionDigits: profile.decimals,
    maximumFractionDigits: profile.decimals,
  });
