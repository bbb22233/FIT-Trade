"""FIT-Trade v1 domain contracts — strict Pydantic v2 models.

Every model mirrors contracts/jsonschema/fit-trade-v1.schema.json exactly.
Unknown fields are rejected via model_config(extra='forbid').
All financial values are represented as canonical decimal strings.

The constants and enums are derived solely from the frozen schema;
no independent enums, tool-name lists, or effect maps exist.

P0-003 hardening:
  - integer fields use strict=True to block string/number coercion
  - boolean fields use strict=True to block string/number coercion
  - const-true fields have strict bool + true-only validation
  - explicit None is rejected on every non-nullable field
  - schema-required fields carry no Pydantic default so omission fails
"""

from __future__ import annotations

import datetime as _datetime
import re
from enum import Enum
from typing import Annotated, Literal, Self

from pydantic import (
    BaseModel,
    Field,
    StringConstraints,
    field_validator,
    functional_validators,
    model_validator,
)

# ---------------------------------------------------------------------------
# Schema version
# ---------------------------------------------------------------------------

SCHEMA_VERSION = "fit.trade.v1"

# ---------------------------------------------------------------------------
# Enums — exact mirror of the JSON Schema $defs
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


class ReconciliationState(str, Enum):
    PENDING = "PENDING"
    ACKNOWLEDGED = "ACKNOWLEDGED"
    REJECTED = "REJECTED"
    NOT_FOUND = "NOT_FOUND"
    MANUAL_RECONCILIATION = "MANUAL_RECONCILIATION"


class EvidenceSource(str, Enum):
    CLOID = "CLOID"
    OID = "OID"
    OPEN_ORDERS = "OPEN_ORDERS"
    HISTORICAL_ORDERS = "HISTORICAL_ORDERS"
    FILLS = "FILLS"


class ExecutionAction(str, Enum):
    PLACE_ORDER = "PLACE_ORDER"
    CANCEL_ENTRY = "CANCEL_ENTRY"
    REPLACE_STOP = "REPLACE_STOP"
    RECONCILE = "RECONCILE"


class AuthorizationType(str, Enum):
    USER_CONFIRMATION = "USER_CONFIRMATION"
    AUTOMATION_GRANT = "AUTOMATION_GRANT"
    RISK_REDUCTION = "RISK_REDUCTION"


class ExecutionResultStatus(str, Enum):
    ACKNOWLEDGED = "ACKNOWLEDGED"
    REJECTED = "REJECTED"
    NOT_DISPATCHED = "NOT_DISPATCHED"
    UNKNOWN_REQUIRES_RECONCILIATION = "UNKNOWN_REQUIRES_RECONCILIATION"


class GrantState(str, Enum):
    DISABLED = "DISABLED"
    ENABLED = "ENABLED"
    SAFETY_PAUSED = "SAFETY_PAUSED"


class AuditActorType(str, Enum):
    USER = "USER"
    DEVICE = "DEVICE"
    HERMES = "HERMES"
    SYSTEM = "SYSTEM"


class AuditSubjectType(str, Enum):
    INTENT = "INTENT"
    CONFIRMATION = "CONFIRMATION"
    OPERATION = "OPERATION"
    ORDER = "ORDER"
    POSITION = "POSITION"
    AUTOMATION_GRANT = "AUTOMATION_GRANT"


# ---------------------------------------------------------------------------
# Shared constrained types
# ---------------------------------------------------------------------------

# Decimal: matches JSON Schema "Decimal" $defs
DECIMAL_RE = re.compile(
    r"^-?(?:0|[1-9][0-9]{0,17})(?:\.[0-9]{0,17}[1-9])?$"
)

# Non-negative decimal
NON_NEGATIVE_DECIMAL_RE = re.compile(
    r"^(?:0|[1-9][0-9]{0,17})(?:\.[0-9]{0,17}[1-9])?$"
)

# Positive: non-negative but not exactly "0"
POSITIVE_DECIMAL_RE = re.compile(
    r"^(?:0\.[0-9]{0,17}[1-9]|[1-9][0-9]{0,17}(?:\.[0-9]{0,17}[1-9])?)$"
)

# Fraction: [0, 1] as canonical decimal
FRACTION_RE = re.compile(
    r"^(?:0(?:\.[0-9]{0,17}[1-9])?|1)$"
)

# Positive fraction: (0, 1] as canonical decimal
POSITIVE_FRACTION_RE = re.compile(
    r"^(?:0\.[0-9]{0,17}[1-9]|1)$"
)

DecimalStr = Annotated[str, StringConstraints(pattern=DECIMAL_RE.pattern, max_length=38)]
NonNegativeDecimalStr = Annotated[
    str, StringConstraints(pattern=NON_NEGATIVE_DECIMAL_RE.pattern, max_length=38)
]
PositiveDecimalStr = Annotated[
    str, StringConstraints(pattern=POSITIVE_DECIMAL_RE.pattern, max_length=38)
]
FractionStr = Annotated[
    str, StringConstraints(pattern=FRACTION_RE.pattern, max_length=38)
]
PositiveFractionStr = Annotated[
    str, StringConstraints(pattern=POSITIVE_FRACTION_RE.pattern, max_length=38)
]

UUIDStr = Annotated[
    str,
    StringConstraints(
        pattern=r"^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$",
        min_length=36,
        max_length=36,
    ),
]
# Timestamp: UTC only (Z/z terminal), accepts T/t/space separator,
# fractional seconds, and valid 23:59:60 leap second.
# Rejects no-zone and numeric-offset (even +00:00) forms.
_TIMESTAMP_PATTERN = (
    r"^\d{4}-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12]\d|3[01])"
    r"[Tt ](?:[01]\d|2[0-3]):[0-5]\d:(?:[0-5]\d|60)"
    r"(?:\.\d+)?[Zz]$"
)

_TIMESTAMP_RE = re.compile(_TIMESTAMP_PATTERN)


def _validate_timestamp_strict(v: str) -> str:
    """Reject no-zone or numeric-offset timestamps; accept only UTC Z/z."""
    m = _TIMESTAMP_RE.match(v)
    if not m:
        raise ValueError(f"timestamp must use terminal Z/z (UTC only): {v!r}")
    # Parse date/time parts for leap-second and calendar checks.
    try:
        # Extract date part: YYYY-MM-DD
        date_part, time_part = v[:10], v[11:]  # separator is at position 10
        _datetime.date.fromisoformat(date_part)
    except ValueError as exc:
        raise ValueError(f"timestamp date invalid: {v!r}") from exc
    # Leap second 60 is only valid at 23:59:60
    second_str = time_part[6:8]  # seconds portion after HH:MM:
    if second_str == "60":
        if time_part[:5] != "23:59":
            raise ValueError(
                f"leap second 60 only valid at 23:59:60, got: {v!r}"
            )
    return v


TimestampStr = Annotated[
    str,
    StringConstraints(pattern=_TIMESTAMP_PATTERN),
    functional_validators.AfterValidator(_validate_timestamp_strict),
]
ClientOrderIdStr = Annotated[str, StringConstraints(pattern=r"^[a-f0-9]{32}$")]
NonceStr = Annotated[str, StringConstraints(pattern=r"^[A-Za-z0-9_-]{32,128}$")]
HashStr = Annotated[str, StringConstraints(pattern=r"^[a-f0-9]{64}$")]

# Version patterns (for fields where schema uses maxLength:64, not the strict pattern)
VersionStr = Annotated[str, StringConstraints(max_length=64)]

# Risk policy version (specific pattern)
RiskPolicyVersionStr = Annotated[
    str, StringConstraints(pattern=r"^risk-[a-z0-9][a-z0-9._-]{0,63}$", max_length=64)
]

# Strategy version (specific pattern where used)
StrategyVersionStr = Annotated[
    str, StringConstraints(pattern=r"^strategy-[a-z0-9][a-z0-9._-]{0,63}$", max_length=64)
]

# Model version (specific pattern where used)
ModelVersionStr = Annotated[
    str, StringConstraints(pattern=r"^model-[a-z0-9][a-z0-9._-]{0,63}$", max_length=64)
]

# Execution error code pattern
ErrorCodeStr = Annotated[str, StringConstraints(pattern=r"^[A-Z][A-Z0-9_]{2,95}$")]

# Reason codes pattern
ReasonCodeStr = Annotated[str, StringConstraints(pattern=r"^[A-Z][A-Z0-9_]{2,63}$")]

# Stop order IDs in active_stop_order_ids
StopOrderIdStr = Annotated[
    str, StringConstraints(min_length=1, max_length=128, pattern=r"\S")
]

# Event type pattern for AuditEvent
EventTypeStr = Annotated[str, StringConstraints(pattern=r"^[A-Z][A-Z0-9_]{2,95}$")]


# ---------------------------------------------------------------------------
# Helper: validate const-true fields (strict bool + true-only)
# ---------------------------------------------------------------------------


def _validate_const_true(v: object) -> bool:
    """Reject anything that is not a Python bool True.

    Pydantic v2 Literal[True] accepts truthy values (1, "true", etc.)
    in non-strict mode.  This validator is called from field_validator(mode='before')
    so it runs before any coercion and rejects strings, ints, and False.
    """
    if v is True:
        return v
    raise ValueError(f"must be true (strict boolean), got {type(v).__name__}: {v!r}")


# ---------------------------------------------------------------------------
# Base domain model
# ---------------------------------------------------------------------------


class BaseDomain(BaseModel):
    """Base for all domain objects — forbids unknown fields and silences model_ namespace warning.

    Post-validation rule: reject any field that was explicitly set to None.
    The frozen JSON Schema has no nullable properties, so None is never a
    valid value.  Fields that are optional in the schema must simply be omitted
    (not provided at all), not set to null.
    """

    model_config = {
        "extra": "forbid",
        "protected_namespaces": (),
    }

    @model_validator(mode="after")
    def _reject_explicit_none(self) -> Self:
        """Reject any field that was explicitly present in the input and set to None."""
        for field_name, field_info in self.model_fields.items():
            if field_name in self.__pydantic_fields_set__:
                if getattr(self, field_name) is None:
                    raise ValueError(
                        f"'{field_name}' must not be explicitly null "
                        f"(schema has no nullable properties; omit the field instead)"
                    )
        return self


# ---------------------------------------------------------------------------
# Nested types
# ---------------------------------------------------------------------------


class StopMarket(BaseDomain):
    type: Literal["STOP_MARKET"]  # schema-required, no default
    trigger_price: PositiveDecimalStr
    reduce_only: Literal[True]  # schema-required, no default; const-true validated below

    @field_validator("reduce_only", mode="before")
    @classmethod
    def _validate_reduce_only_strict_true(cls, v: object) -> bool:
        return _validate_const_true(v)


class TakeProfitLeg(BaseDomain):
    trigger_price: PositiveDecimalStr
    quantity_fraction: PositiveFractionStr
    reduce_only: Literal[True]  # schema-required, no default; const-true validated below

    @field_validator("reduce_only", mode="before")
    @classmethod
    def _validate_reduce_only_strict_true(cls, v: object) -> bool:
        return _validate_const_true(v)


class SymbolLeverage(BaseDomain):
    symbol: Symbol
    maximum_leverage: int = Field(ge=1, le=100, strict=True)


# ---------------------------------------------------------------------------
# Core domain objects — exact mirror of fit-trade-v1.schema.json $defs
# ---------------------------------------------------------------------------


class TradeIntent(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default — omission must fail
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
    leverage: int = Field(ge=1, le=100, strict=True)
    entry_price_or_bound: PositiveDecimalStr
    worst_acceptable_price: PositiveDecimalStr
    stop: StopMarket | None = None
    take_profit_plan: list[TakeProfitLeg] = Field(max_length=8)  # no default — omission must fail
    risk_policy_version: RiskPolicyVersionStr
    market_snapshot_id: UUIDStr
    strategy_version: StrategyVersionStr
    model_version: ModelVersionStr
    source: Literal["USER_DIRECTED", "AUTOMATION"]
    created_at: TimestampStr

    @model_validator(mode="after")
    def _enforce_conditional_rules(self) -> Self:
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
        if self.order_type == OrderType.LIMIT and self.time_in_force not in (
            TimeInForce.GTC,
            TimeInForce.ALO,
        ):
            raise ValueError("LIMIT orders must use GTC or ALO time_in_force")
        return self


class ConfirmationTicket(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
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
    def _intent_must_be_risk_increasing(self) -> Self:
        if self.intent.position_effect not in (
            PositionEffect.OPEN,
            PositionEffect.INCREASE,
        ):
            raise ValueError(
                f"confirmation intent must be OPEN or INCREASE, got "
                f"{self.intent.position_effect.value}"
            )
        return self


class Operation(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
    operation_id: UUIDStr
    intent_id: UUIDStr
    confirmation_id: UUIDStr | None = None
    state: OperationState
    state_version: int = Field(ge=0, strict=True)
    rejection_code: str | None = Field(default=None, max_length=96)
    created_at: TimestampStr
    updated_at: TimestampStr


class ExecutionAttempt(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
    attempt_id: UUIDStr
    operation_id: UUIDStr
    client_order_id: ClientOrderIdStr
    attempt_number: int = Field(ge=1, le=32, strict=True)
    state: AttemptState
    created_at: TimestampStr


class Order(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
    order_id: UUIDStr
    operation_id: UUIDStr
    client_order_id: ClientOrderIdStr
    exchange_order_id: str | None = Field(default=None, max_length=128)
    symbol: Symbol
    side: Side
    quantity: PositiveDecimalStr
    filled_quantity: NonNegativeDecimalStr
    limit_price: PositiveDecimalStr | None = None
    reduce_only: bool = Field(strict=True)
    state: OrderState
    updated_at: TimestampStr


class Fill(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
    fill_id: UUIDStr
    order_id: UUIDStr
    symbol: Symbol
    side: Side
    quantity: PositiveDecimalStr
    price: PositiveDecimalStr
    fee: NonNegativeDecimalStr
    occurred_at: TimestampStr


class PositionSnapshot(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
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
    schema_version: Literal["fit.trade.v1"]  # no default
    risk_policy_version: RiskPolicyVersionStr
    allowed_symbols: list[Symbol] = Field(min_length=1)
    maximum_trade_risk_fraction: PositiveFractionStr
    maximum_total_risk_fraction: PositiveFractionStr
    maximum_leverage_by_symbol: list[SymbolLeverage] = Field(
        min_length=3, max_length=3,
    )
    maximum_slippage_fraction: PositiveFractionStr
    created_at: TimestampStr

    @field_validator("allowed_symbols")
    @classmethod
    def _validate_unique_symbols(cls, v: list[Symbol]) -> list[Symbol]:
        if len(v) != len(set(v)):
            raise ValueError("allowed_symbols must have unique items")
        return v

    @field_validator("maximum_leverage_by_symbol")
    @classmethod
    def _validate_leverage_unique_symbols(
        cls, v: list[SymbolLeverage]
    ) -> list[SymbolLeverage]:
        symbols = [item.symbol for item in v]
        if len(symbols) != len(set(symbols)):
            raise ValueError("maximum_leverage_by_symbol must have unique symbols")
        expected = {Symbol.BTC_PERP, Symbol.ETH_PERP, Symbol.SOL_PERP}
        if set(symbols) != expected:
            raise ValueError(
                "maximum_leverage_by_symbol must contain exactly one entry "
                "for BTC-PERP, ETH-PERP, and SOL-PERP"
            )
        return v


class AutomationGrant(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
    grant_id: UUIDStr
    user_id: UUIDStr
    account_id: UUIDStr
    state: GrantState
    allowed_symbols: list[Symbol] = Field(min_length=1)
    risk_policy_version: VersionStr
    strategy_version: VersionStr
    model_version: VersionStr
    review_model_version: VersionStr
    maximum_notional: PositiveDecimalStr
    authorization_hash: HashStr

    @field_validator("allowed_symbols")
    @classmethod
    def _validate_unique_symbols(cls, v: list[Symbol]) -> list[Symbol]:
        if len(v) != len(set(v)):
            raise ValueError("allowed_symbols must have unique items")
        return v


class ModelProposal(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
    proposal_id: UUIDStr
    model_version: VersionStr
    decision: Literal["PROPOSE_TRADE", "NO_TRADE"]
    intent: TradeIntent | None = None
    created_at: TimestampStr

    @model_validator(mode="after")
    def _enforce_decision_rules(self) -> Self:
        if self.decision == "PROPOSE_TRADE" and self.intent is None:
            raise ValueError("PROPOSE_TRADE requires intent")
        if self.decision == "NO_TRADE" and self.intent is not None:
            raise ValueError("NO_TRADE must not carry an intent")
        return self


class ModelReview(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
    review_id: UUIDStr
    proposal_id: UUIDStr
    review_model_version: VersionStr
    decision: Literal["APPROVE", "REJECT", "NO_TRADE"]
    reason_codes: list[ReasonCodeStr] = Field(min_length=1, max_length=32)
    created_at: TimestampStr


class RiskDecision(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
    decision_id: UUIDStr
    intent_id: UUIDStr
    risk_policy_version: VersionStr
    result: Literal["ADMITTED", "REJECTED"]
    reason_codes: list[ReasonCodeStr] = Field(max_length=32)  # no default — omission must fail
    admitted_quantity: NonNegativeDecimalStr
    maximum_loss: NonNegativeDecimalStr
    maximum_loss_fraction: FractionStr
    post_trade_total_risk: NonNegativeDecimalStr
    post_trade_total_risk_fraction: FractionStr
    market_snapshot_id: UUIDStr
    decided_at: TimestampStr

    @field_validator("reason_codes")
    @classmethod
    def _validate_unique_reason_codes(cls, v: list[str]) -> list[str]:
        if len(v) != len(set(v)):
            raise ValueError("reason_codes must have unique items")
        return v


class ExecutionCommand(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
    command_id: UUIDStr
    operation_id: UUIDStr
    attempt_id: UUIDStr
    authorization_type: AuthorizationType
    authorization_id: UUIDStr
    action: ExecutionAction
    # Conditional fields (required when action='PLACE_ORDER')
    client_order_id: ClientOrderIdStr | None = None
    symbol: Symbol | None = None
    side: Side | None = None
    order_type: OrderType | None = None
    time_in_force: TimeInForce | None = None
    quantity: PositiveDecimalStr | None = None
    worst_acceptable_price: PositiveDecimalStr | None = None
    reduce_only: bool = Field(strict=True)  # schema-required, no default
    created_at: TimestampStr

    @model_validator(mode="after")
    def _enforce_conditional_rules(self) -> Self:
        # PLACE_ORDER requires order fields
        if self.action == ExecutionAction.PLACE_ORDER:
            missing = []
            for field_name in (
                "client_order_id", "symbol", "side", "order_type",
                "time_in_force", "quantity", "worst_acceptable_price",
            ):
                if getattr(self, field_name) is None:
                    missing.append(field_name)
            if missing:
                raise ValueError(
                    f"PLACE_ORDER requires fields: {', '.join(missing)}"
                )
            # MARKET => IOC only
            if self.order_type == OrderType.MARKET and self.time_in_force != TimeInForce.IOC:
                raise ValueError("PLACE_ORDER MARKET requires IOC time_in_force")
            # LIMIT => GTC or ALO
            if self.order_type == OrderType.LIMIT and self.time_in_force not in (
                TimeInForce.GTC,
                TimeInForce.ALO,
            ):
                raise ValueError("PLACE_ORDER LIMIT requires GTC or ALO time_in_force")

        # RISK_REDUCTION enforces reduce_only=true
        if self.authorization_type == AuthorizationType.RISK_REDUCTION:
            if self.reduce_only is not True:
                raise ValueError("RISK_REDUCTION requires reduce_only=true")

        return self


class ExecutionResult(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
    result_id: UUIDStr
    command_id: UUIDStr
    attempt_id: UUIDStr
    status: ExecutionResultStatus
    exchange_order_id: str | None = Field(default=None, max_length=128)
    error_code: ErrorCodeStr | None = None
    observed_at: TimestampStr


class ProtectionStatus(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
    protection_status_id: UUIDStr
    position_id: UUIDStr
    state: ProtectionState
    absolute_live_position_quantity: NonNegativeDecimalStr
    active_stop_order_ids: list[StopOrderIdStr] = Field(max_length=32)  # no default — omission must fail
    coverage_evidence_hash: HashStr | None = None
    data_status: DataStatus
    observed_at: TimestampStr

    @field_validator("active_stop_order_ids")
    @classmethod
    def _validate_unique_stop_ids(cls, v: list[str]) -> list[str]:
        if len(v) != len(set(v)):
            raise ValueError("active_stop_order_ids must have unique items")
        return v

    @model_validator(mode="after")
    def _enforce_protected_conditionals(self) -> Self:
        if self.state == ProtectionState.PROTECTED:
            # Must have at least one stop order
            if len(self.active_stop_order_ids) < 1:
                raise ValueError(
                    "PROTECTED state requires at least one active_stop_order_id"
                )
            # Must have coverage_evidence_hash
            if self.coverage_evidence_hash is None:
                raise ValueError(
                    "PROTECTED state requires coverage_evidence_hash"
                )
            # absolute_live_position_quantity must be positive (not zero)
            if not POSITIVE_DECIMAL_RE.match(self.absolute_live_position_quantity):
                raise ValueError(
                    "PROTECTED state requires positive absolute_live_position_quantity"
                )
        return self


class ReconciliationStatus(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
    reconciliation_id: UUIDStr
    operation_id: UUIDStr
    attempt_id: UUIDStr
    client_order_id: ClientOrderIdStr
    state: ReconciliationState
    evidence_sources: list[EvidenceSource] = Field(min_length=1)
    exchange_order_id: str | None = Field(default=None, max_length=128)
    checked_at: TimestampStr

    @field_validator("evidence_sources")
    @classmethod
    def _validate_unique_evidence_sources(
        cls, v: list[EvidenceSource]
    ) -> list[EvidenceSource]:
        if len(v) != len(set(v)):
            raise ValueError("evidence_sources must have unique items")
        return v


class AgentFeedback(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
    feedback_id: UUIDStr
    operation_id: UUIDStr
    user_id: UUIDStr
    account_id: UUIDStr
    rating: int = Field(ge=-2, le=2, strict=True)
    comment: str = Field(min_length=1, max_length=2000)
    include_in_learning: bool = Field(strict=True)
    created_at: TimestampStr


class AutomationAuthorization(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
    authorization_id: UUIDStr
    grant_id: UUIDStr
    user_id: UUIDStr
    account_id: UUIDStr
    device_id: UUIDStr
    session_id: UUIDStr
    risk_policy_version: VersionStr
    strategy_version: VersionStr
    model_version: VersionStr
    review_model_version: VersionStr
    allowed_symbols: list[Symbol] = Field(min_length=1)
    maximum_notional: PositiveDecimalStr
    authorization_hash: HashStr
    authorized_at: TimestampStr

    @field_validator("allowed_symbols")
    @classmethod
    def _validate_unique_symbols(cls, v: list[Symbol]) -> list[Symbol]:
        if len(v) != len(set(v)):
            raise ValueError("allowed_symbols must have unique items")
        return v


class AuditEvent(BaseDomain):
    schema_version: Literal["fit.trade.v1"]  # no default
    event_id: UUIDStr
    event_type: EventTypeStr
    actor_type: AuditActorType
    subject_type: AuditSubjectType
    subject_id: UUIDStr
    occurred_at: TimestampStr
    payload_hash: HashStr


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
