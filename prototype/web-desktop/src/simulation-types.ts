export type DataState = "LIVE" | "STALE" | "RECONCILING";

export type OperationState =
  | "AWAITING_CONFIRMATION"
  | "RISK_REVALIDATING"
  | "DISPATCH_PENDING"
  | "DISPATCHED"
  | "ACKNOWLEDGED"
  | "RECONCILING"
  | "UNKNOWN_REQUIRES_RECONCILIATION"
  | "STOP_PLACING"
  | "PROTECTION_PENDING"
  | "PROTECTED"
  | "EXPIRED"
  | "REJECTED";

export type PositionEffect = "OPEN" | "ADD";

const activeOperationStates: readonly OperationState[] = [
  "RISK_REVALIDATING",
  "DISPATCH_PENDING",
  "DISPATCHED",
  "ACKNOWLEDGED",
  "RECONCILING",
  "UNKNOWN_REQUIRES_RECONCILIATION",
  "STOP_PLACING",
  "PROTECTION_PENDING",
];

export const isOperationActive = (state: OperationState) =>
  activeOperationStates.includes(state);

export type ConfirmationTicketSnapshot = Readonly<{
  ticketId: string;
  symbol: "BTC" | "ETH" | "SOL";
  pair: string;
  positionEffect: PositionEffect;
  direction: "BUY_LONG";
  directionLabel: string;
  quantity: string;
  notional: string;
  leverage: "5x";
  marginMode: "逐仓";
  margin: string;
  referencePrice: string;
  worstFillPrice: string;
  stopMarket: string;
  liquidationPrice: string;
  maxLoss: string;
  maxLossPercent: string;
  totalRisk: string;
  totalRiskPercent: string;
  feeBudget: string;
}>;

export type ChatMessage = {
  id: number;
  role: "you" | "hermes";
  content: string;
};
export const operationLabels: Record<OperationState, string> = {
  AWAITING_CONFIRMATION: "等待你的模拟确认",
  RISK_REVALIDATING: "模拟服务端正在重新验证风险",
  DISPATCH_PENDING: "模拟执行命令等待派发",
  DISPATCHED: "模拟命令已进入执行链路",
  ACKNOWLEDGED: "模拟交易所已确认请求",
  RECONCILING: "模拟服务端正在核对 Operation",
  UNKNOWN_REQUIRES_RECONCILIATION: "模拟结果未知，正在核对",
  STOP_PLACING: "正在模拟建立 Stop Market",
  PROTECTION_PENDING: "正在模拟建立 Stop Market",
  PROTECTED: "模拟成交与止损均已核对",
  EXPIRED: "确认已过期",
  REJECTED: "模拟风控已拒绝",
};

export const dataStateCopy: Record<
  DataState,
  { label: string; detail: string }
> = {
  LIVE: {
    label: "LIVE",
    detail: "本地模拟行情与账户快照已核对。",
  },
  STALE: {
    label: "STALE",
    detail: "模拟数据已过期，已禁止开仓、加仓和其他增加风险操作。",
  },
  RECONCILING: {
    label: "RECONCILING",
    detail: "正在模拟核对账户、仓位、订单与成交；一致前禁止增加风险。",
  },
};
