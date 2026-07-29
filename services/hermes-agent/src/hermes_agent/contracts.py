"""FIT-Trade v1 domain contracts — strict Pydantic v2 models.

Every model mirrors contracts/jsonschema/fit-trade-v1.schema.json.
Unknown fields are rejected via model_config(extra='forbid').
All financial values are represented as canonical decimal strings.
"""

from __future__ import annotations

import re
from datetime import datetime
from enum import Enum
from typing import Annotated, Literal

from pydantic import (
    BaseModel,
    Field,
    StringConstraints,
    field_validator,
    model_validator,
)

# ---------------------------------------------------------------------------
# Schema version
# ---------------------------------------------------------------------------

SCHEMA_VERSION = "fit.trade.v1"

# ---------------------------------------------------------------------------
# Enums
# ---------------------------------------------------------------------------


class Network(str, Enum):
    HYPERLIQUID_MAINNET = "HYPERLIQUID_MAINNET"
    HYPERLIQUID_TESTNET = "HYPERLIQUID_TESTNET"


class Symbol(str, Enum):
    BTC_PERP = "BTC-PERP"
    ETH_PERP = "ETH-PERP"
    SOL_PERP = "SOL-PERP"


class Side(str, Enum):
    BUY = "BUY"
    SELL = "SELL"


class PositionEffect(str, Enum):
    OPEN = "OPEN"
    INCREASE = "INCREASE"
    REDUCE = "REDUCE"
    CLOSE = "CLOSE"


class OrderType(str, Enum):
    MARKET = "MARKET"
    LIMIT = "LIMIT"


class TimeInForce(str, Enum):
    IOC = "IOC"
    GTC = "GTC"
    ALO = "ALO"


class MarginMode(str, Enum):
    CROSS = "CROSS"
    ISOLATED = "ISOLATED"


class DataStatus(str, Enum):
    LIVE = "LIVE"
    STALE = "STALE"
    RECONCILING = "RECONCILING"


class OperationState(str, Enum):
    DRAFT = "DRAFT"
    AWAITING_CONFIRMATION = "AWAITING_CONFIRMATION"
    EXPIRED = "EXPIRED"
    CONFIRMED = "CONFIRMED"
    RISK_REVALIDATING = "RISK_REVALIDATING"
    REJECTED = "REJECTED"
    ADMITTED = "ADMITTED"
    DISPATCH_PENDING = "DISPATCH_PENDING"
    DISPATCHED = "DISPATCHED"
    ACKNOWLEDGED = "ACKNOWLEDGED"
    UNKNOWN_REQUIRES_RECONCILIATION = "UNKNOWN_REQUIRES_RECONCILIATION"
    MANUAL_RECONCILIATION = "MANUAL_RECONCILIATION"
    FINAL = "FINAL"


class AttemptState(str, Enum):
    CREATED = "CREATED"
    SIGNED = "SIGNED"
    SUBMITTED = "SUBMITTED"
    ACKNOWLEDGED = "ACKNOWLEDGED"
    UNKNOWN_REQUIRES_RECONCILIATION = "UNKNOWN_REQUIRES_RECONCILIATION"
    REJECTED = "REJECTED"


class OrderState(str, Enum):
    PENDING = "PENDING"
    OPEN = "OPEN"
    PARTIALLY_FILLED = "PARTIALLY_FILLED"
    FILLED = "FILLED"
    CANCELLED = "CANCELLED"
    REJECTED = "REJECTED"
    UNKNOWN_REQUIRES_RECONCILIATION = "UNKNOWN_REQUIRES_RECONCILIATION"


class ProtectionState(str, Enum):
    NO_POSITION = "NO_POSITION"
    ENTRY_PENDING = "ENTRY_PENDING"
    PARTIALLY_FILLED = "PARTIALLY_FILLED"
    FILLED = "FILLED"
    PROTECTION_PENDING = "PROTECTION_PENDING"
    PROTECTED = "PROTECTED"
    ADJUSTING_PROTECTION = "ADJUSTING_PROTECTION"
    PROTECTION_FAILED = "PROTECTION_FAILED"
    EMERGENCY_CLOSING = "EMERGENCY_CLOSING"
    MANUAL_RECONCILIATION = "MANUAL_RECONCILIATION"
    CLOSED = "CLOSED"


# ---------------------------------------------------------------------------
# Shared constrained types
# ---------------------------------------------------------------------------

# RFC 8785 canonical decimal: sign? (1-9[0-9]{0,17} | 0) (.[0-9]{0,17}[1-9])?
# No leading zeros, no trailing zeros, no exponent, no plus sign.
DECIMAL_RE = re.compile(
    r"^-?(?:0|[1-9][0-9]{0,17})(?:\.[0-9]{0,17}[1-9])?$"
)

# Non-negative decimal: same but no leading minus.
NON_NEGATIVE_DECIMAL_RE = re.compile(
    r"^(?:0|[1-9][0-9]{0,17})(?:\.[0-9]{0,17}[1-9])?$"
)

# Positive: non-negative but not exactly "0".
POSITIVE_DECIMAL_RE = re.compile(
    r"^(?:0\.[0-9]{0,17}[1-9]|[1-9][0-9]{0,17}(?:\.[0-9]{0,17}[1-9])?)$"
)

# Fraction: [0, 1] as canonical decimal.
FRACTION_RE = re.compile(
    r"^(?:0(?:\.[0-9]{0,17}[1-9])?|1)$"
)

# Positive fraction: (0, 1] as canonical decimal.
POSITIVE_FRACTION_RE = re.compile(
    r"^(?:0\.[0-9]{0,17}[1-9]|1)$"
)

DecimalStr = Annotated[str, StringConstraints(pattern=DECIMAL_RE.pattern, max_length=38)]
NonNegativeDecimalStr = Annotated[str, StringConstraints(pattern=NON_NEGATIVE_DECIMAL_RE.pattern, max_length=38)]
PositiveDecimalStr = Annotated[str, StringConstraints(pattern=POSITIVE_DECIMAL_RE.pattern, max_length=38)]
FractionStr = Annotated[str, StringConstraints(pattern=FRACTION_RE.pattern, max_length=38)]
PositiveFractionStr = Annotated[str, StringConstraints(pattern=POSITIVE_FRACTION_RE.pattern, max_length=38)]
UUIDStr = Annotated[str, StringConstraints(pattern=r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$", min_length=36, max_length=36)]
TimestampStr = Annotated[str, StringConstraints(pattern=r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$")]
ClientOrderIdStr = Annotated[str, StringConstraints(pattern=r"^[a-f0-9]{32}$")]
VersionStr = Annotated[str, StringConstraints(pattern=r"^[a-z0-9][a-z0-9._-]{0,63}$")]
NonceStr = Annotated[str, StringConstraints(pattern=r"^[A-Za-z0-9_-]{32,128}$")]
HashStr = Annotated[str, StringConstraints(pattern=r"^[a-f0-9]{64}$")]


class BaseDomain(BaseModel):
    """Base for all domain objects — rejects unknown fields."""
    model_config = {"extra": "forbid"}


# ---------------------------------------------------------------------------
# Nested types
# ---------------------------------------------------------------------------

class StopMarket(BaseDomain):
    type: Literal["STOP_MARKET"] = "STOP_MARKET"
    trigger_price: PositiveDecimalStr
    reduce_only: Literal[True] = True


class TakeProfitLeg(BaseDomain):
    trigger_price: PositiveDecimalStr
    quantity_fraction: PositiveFractionStr
    reduce_only: Literal[True] = True


class SymbolLeverage(BaseDomain):
    symbol: Symbol
    maximum_leverage: int = Field(ge=1, le=100)


# ---------------------------------------------------------------------------
# Core domain objects
# ---------------------------------------------------------------------------

class TradeIntent(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    intent_id: UUIDStr
    user_id: UUIDStr
    account_id: UUIDStr
    network: Network
    symbol: Symbol
    side: Side
    position_effect: PositionEffect
    order_type: OrderType
    time_in_force: TimeInForce
    quantity: PositiveDecimalStr
    notional: PositiveDecimalStr
    margin_mode: MarginMode
    leverage: int = Field(ge=1, le=100)
    entry_price_or_bound: PositiveDecimalStr
    worst_acceptable_price: PositiveDecimalStr
    stop: StopMarket | None = None
    take_profit_plan: list[TakeProfitLeg] = Field(default_factory=list, max_length=8)
    risk_policy_version: str = Field(pattern=r"^risk-[a-z0-9][a-z0-9._-]{0,63}$")
    market_snapshot_id: UUIDStr
    strategy_version: str = Field(pattern=r"^strategy-[a-z0-9][a-z0-9._-]{0,63}$")
    model_version: str = Field(pattern=r"^model-[a-z0-9][a-z0-9._-]{0,63}$")
    source: Literal["USER_DIRECTED", "AUTOMATION"]
    created_at: TimestampStr

    @model_validator(mode="after")
    def _enforce_conditional_rules(self) -> "TradeIntent":
        # Stop is mandatory for OPEN / INCREASE
        if self.position_effect in (PositionEffect.OPEN, PositionEffect.INCREASE):
            if self.stop is None:
                raise ValueError(
                    f"position_effect '{self.position_effect.value}' requires stop"
                )
        # MARKET => IOC only
        if self.order_type == OrderType.MARKET and self.time_in_force != TimeInForce.IOC:
            raise ValueError("MARKET orders must use IOC time_in_force")
        # LIMIT => GTC or ALO only
        if self.order_type == OrderType.LIMIT and self.time_in_force not in (TimeInForce.GTC, TimeInForce.ALO):
            raise ValueError("LIMIT orders must use GTC or ALO time_in_force")
        return self


class ConfirmationTicket(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    confirmation_id: UUIDStr
    device_id: UUIDStr
    session_id: UUIDStr
    intent: TradeIntent
    maximum_loss: NonNegativeDecimalStr
    maximum_loss_fraction: FractionStr
    post_trade_total_risk: NonNegativeDecimalStr
    post_trade_total_risk_fraction: FractionStr
    liquidation_price: PositiveDecimalStr
    estimated_fees: NonNegativeDecimalStr
    slippage_budget: NonNegativeDecimalStr
    expires_at: TimestampStr
    confirmation_nonce: NonceStr
    confirmation_hash: HashStr
    created_at: TimestampStr

    @model_validator(mode="after")
    def _intent_must_be_risk_increasing(self) -> "ConfirmationTicket":
        if self.intent.position_effect not in (PositionEffect.OPEN, PositionEffect.INCREASE):
            raise ValueError(
                f"confirmation intent must be OPEN or INCREASE, got {self.intent.position_effect.value}"
            )
        return self


class Operation(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    operation_id: UUIDStr
    intent_id: UUIDStr
    confirmation_id: UUIDStr | None = None
    state: OperationState
    state_version: int = Field(ge=0)
    rejection_code: str | None = Field(default=None, max_length=96)
    created_at: TimestampStr
    updated_at: TimestampStr


class ExecutionAttempt(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    attempt_id: UUIDStr
    operation_id: UUIDStr
    client_order_id: ClientOrderIdStr
    attempt_number: int = Field(ge=1, le=32)
    state: AttemptState
    created_at: TimestampStr


class Order(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    order_id: UUIDStr
    operation_id: UUIDStr
    client_order_id: ClientOrderIdStr
    exchange_order_id: str | None = Field(default=None, max_length=128)
    symbol: Symbol
    side: Side
    quantity: PositiveDecimalStr
    filled_quantity: NonNegativeDecimalStr
    limit_price: PositiveDecimalStr | None = None
    reduce_only: bool
    state: OrderState
    updated_at: TimestampStr


class Fill(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    fill_id: UUIDStr
    order_id: UUIDStr
    symbol: Symbol
    side: Side
    quantity: PositiveDecimalStr
    price: PositiveDecimalStr
    fee: NonNegativeDecimalStr
    occurred_at: TimestampStr


class PositionSnapshot(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    position_id: UUIDStr
    account_id: UUIDStr
    symbol: Symbol
    signed_quantity: str  # DecimalStr — allows negative
    entry_price: PositiveDecimalStr
    mark_price: PositiveDecimalStr
    protection_status_id: UUIDStr
    protection_state: ProtectionState
    data_status: DataStatus
    snapshot_at: TimestampStr

    @field_validator("signed_quantity")
    @classmethod
    def _validate_signed_decimal(cls, v: str) -> str:
        if not DECIMAL_RE.match(v):
            raise ValueError(f"'{v}' is not a canonical decimal")
        return v


class RiskPolicy(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    risk_policy_version: str = Field(pattern=r"^risk-[a-z0-9][a-z0-9._-]{0,63}$")
    allowed_symbols: list[Symbol] = Field(min_length=1)
    maximum_trade_risk_fraction: FractionStr
    maximum_total_risk_fraction: FractionStr
    maximum_leverage_by_symbol: list[SymbolLeverage] = Field(min_length=1)
    maximum_slippage_fraction: FractionStr
    created_at: TimestampStr


class AutomationGrant(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    grant_id: UUIDStr
    user_id: UUIDStr
    account_id: UUIDStr
    state: Literal["ACTIVE", "DISABLED", "PAUSED"]
    allowed_symbols: list[Symbol] = Field(min_length=1)
    risk_policy_version: str = Field(pattern=r"^risk-[a-z0-9][a-z0-9._-]{0,63}$")
    strategy_version: str = Field(pattern=r"^strategy-[a-z0-9][a-z0-9._-]{0,63}$")
    model_version: str = Field(pattern=r"^model-[a-z0-9][a-z0-9._-]{0,63}$")
    review_model_version: str = Field(pattern=r"^model-[a-z0-9][a-z0-9._-]{0,63}$")
    maximum_notional: PositiveDecimalStr
    authorization_hash: str = Field(pattern=r"^[a-f0-9]{64}$")


class ModelProposal(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    proposal_id: UUIDStr
    model_version: str = Field(pattern=r"^model-[a-z0-9][a-z0-9._-]{0,63}$")
    decision: Literal["NO_TRADE", "PROPOSE_TRADE"]
    intent: TradeIntent | None = None
    created_at: TimestampStr

    @model_validator(mode="after")
    def _enforce_decision_rules(self) -> "ModelProposal":
        if self.decision == "PROPOSE_TRADE" and self.intent is None:
            raise ValueError("PROPOSE_TRADE requires intent")
        if self.decision == "NO_TRADE" and self.intent is not None:
            raise ValueError("NO_TRADE must not carry an intent")
        return self


class ModelReview(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    review_id: UUIDStr
    proposal_id: UUIDStr
    review_model_version: str = Field(pattern=r"^model-[a-z0-9][a-z0-9._-]{0,63}$")
    decision: Literal["NO_TRADE", "APPROVE", "REJECT"]
    reason_codes: list[str] = Field(default_factory=list)
    created_at: TimestampStr


class RiskDecision(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    decision_id: UUIDStr
    intent_id: UUIDStr
    risk_policy_version: str = Field(pattern=r"^risk-[a-z0-9][a-z0-9._-]{0,63}$")
    result: Literal["ADMITTED", "REJECTED"]
    reason_codes: list[str] = Field(default_factory=list)
    admitted_quantity: PositiveDecimalStr | None = None
    maximum_loss: NonNegativeDecimalStr | None = None
    maximum_loss_fraction: FractionStr | None = None
    post_trade_total_risk: NonNegativeDecimalStr | None = None
    post_trade_total_risk_fraction: FractionStr | None = None
    market_snapshot_id: UUIDStr | None = None
    decided_at: TimestampStr


class ExecutionCommand(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    command_id: UUIDStr
    operation_id: UUIDStr
    attempt_id: UUIDStr
    authorization_type: Literal["USER_CONFIRMATION", "AUTOMATION"]
    authorization_id: UUIDStr
    action: Literal["PLACE_ORDER", "CANCEL_ORDER", "MODIFY_ORDER", "TIGHTEN_STOP", "EMERGENCY_CLOSE"]
    client_order_id: ClientOrderIdStr | None = None
    symbol: Symbol
    side: Side | None = None
    order_type: OrderType | None = None
    time_in_force: TimeInForce | None = None
    quantity: PositiveDecimalStr | None = None
    worst_acceptable_price: PositiveDecimalStr | None = None
    reduce_only: bool = False
    created_at: TimestampStr


class ExecutionResult(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    result_id: UUIDStr
    command_id: UUIDStr
    attempt_id: UUIDStr
    status: Literal["ACKNOWLEDGED", "REJECTED", "UNKNOWN_REQUIRES_RECONCILIATION"]
    exchange_order_id: str | None = Field(default=None, max_length=128)
    observed_at: TimestampStr


class ProtectionStatus(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    protection_status_id: UUIDStr
    position_id: UUIDStr
    state: ProtectionState
    absolute_live_position_quantity: NonNegativeDecimalStr
    active_stop_order_ids: list[str] = Field(min_length=1)
    coverage_evidence_hash: str = Field(pattern=r"^[a-f0-9]{64}$")
    data_status: DataStatus
    observed_at: TimestampStr


class ReconciliationStatus(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    reconciliation_id: UUIDStr
    operation_id: UUIDStr
    attempt_id: UUIDStr
    client_order_id: ClientOrderIdStr
    state: Literal["PENDING", "RECONCILED", "MANUAL_REQUIRED"]
    evidence_sources: list[Literal["CLOID", "OID", "FILL", "ORDERBOOK", "EXCHANGE_EVENT"]] = Field(min_length=1)
    checked_at: TimestampStr


class AgentFeedback(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    feedback_id: UUIDStr
    operation_id: UUIDStr
    user_id: UUIDStr
    account_id: UUIDStr
    rating: int = Field(ge=-2, le=2)
    comment: str = Field(min_length=1, max_length=2000)
    include_in_learning: bool
    created_at: TimestampStr


class AutomationAuthorization(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    authorization_id: UUIDStr
    grant_id: UUIDStr
    user_id: UUIDStr
    account_id: UUIDStr
    device_id: UUIDStr
    session_id: UUIDStr
    risk_policy_version: str = Field(pattern=r"^risk-[a-z0-9][a-z0-9._-]{0,63}$")
    strategy_version: str = Field(pattern=r"^strategy-[a-z0-9][a-z0-9._-]{0,63}$")
    model_version: str = Field(pattern=r"^model-[a-z0-9][a-z0-9._-]{0,63}$")
    review_model_version: str = Field(pattern=r"^model-[a-z0-9][a-z0-9._-]{0,63}$")
    allowed_symbols: list[Symbol] = Field(min_length=1)
    maximum_notional: PositiveDecimalStr
    authorization_hash: str = Field(pattern=r"^[a-f0-9]{64}$")
    authorized_at: TimestampStr


class AuditEvent(BaseDomain):
    schema_version: Literal["fit.trade.v1"] = "fit.trade.v1"
    event_id: UUIDStr
    event_type: str = Field(min_length=1, max_length=64)
    actor_type: Literal["USER", "SYSTEM"]
    subject_type: str = Field(min_length=1, max_length=32)
    subject_id: UUIDStr
    occurred_at: TimestampStr
    payload_hash: str = Field(pattern=r"^[a-f0-9]{64}$")


# ---------------------------------------------------------------------------
# Top-level discriminator map — matches JSON Schema oneOf
# ---------------------------------------------------------------------------

DOMAIN_TYPES: dict[str, type[BaseDomain]] = {
    "TradeIntent": TradeIntent,
    "ConfirmationTicket": ConfirmationTicket,
    "Operation": Operation,
    "ExecutionAttempt": ExecutionAttempt,
    "Order": Order,
    "Fill": Fill,
    "PositionSnapshot": PositionSnapshot,
    "RiskPolicy": RiskPolicy,
    "AutomationGrant": AutomationGrant,
    "ModelProposal": ModelProposal,
    "ModelReview": ModelReview,
    "RiskDecision": RiskDecision,
    "ExecutionCommand": ExecutionCommand,
    "ExecutionResult": ExecutionResult,
    "ProtectionStatus": ProtectionStatus,
    "ReconciliationStatus": ReconciliationStatus,
    "AgentFeedback": AgentFeedback,
    "AutomationAuthorization": AutomationAuthorization,
    "AuditEvent": AuditEvent,
}
