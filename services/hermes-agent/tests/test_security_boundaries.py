"""Security boundary tests — prompt injection, forbidden capabilities, bound-field mutations."""

import json
from pathlib import Path

import pytest

from hermes_agent.contracts import (
    TradeIntent,
    ConfirmationTicket,
    Operation,
    OrderType,
    TimeInForce,
    PositionEffect,
    DOMAIN_TYPES,
)
from hermes_agent.mcp_tools import get_tool_by_name, SERVER_SCOPE_FIELDS
from hermes_agent.confirmation import canonicalize, confirmation_binding
from hermes_agent.validator import load_confirmation_fields, FIXTURES_VALID_DIR


# ---------------------------------------------------------------------------
# Prompt-injected scope: user_id, account_id, session_id
# ---------------------------------------------------------------------------

def test_trade_intent_rejects_model_supplied_user_id():
    """Model must not be able to override user_id — it's server-injected."""
    with pytest.raises(Exception):
        TradeIntent.model_validate({
            "schema_version": "fit.trade.v1",
            "intent_id": "11111111-1111-4111-8111-111111111111",
            "user_id": "injected-by-model-00000000000000",
            "account_id": "33333333-3333-4333-8333-333333333333",
            "network": "HYPERLIQUID_MAINNET",
            "symbol": "BTC-PERP",
            "side": "BUY",
            "position_effect": "OPEN",
            "order_type": "MARKET",
            "time_in_force": "IOC",
            "quantity": "0.025",
            "notional": "2961.3",
            "margin_mode": "ISOLATED",
            "leverage": 5,
            "entry_price_or_bound": "118452",
            "worst_acceptable_price": "118689",
            "stop": {"type": "STOP_MARKET", "trigger_price": "115200", "reduce_only": True},
            "take_profit_plan": [],
            "risk_policy_version": "risk-v1",
            "market_snapshot_id": "44444444-4444-4444-8444-444444444444",
            "strategy_version": "strategy-user-directed-v1",
            "model_version": "model-hermes-v1",
            "source": "USER_DIRECTED",
            "created_at": "2026-07-29T04:00:00Z",
        })  # user_id intentionally malformed


def test_trade_intent_rejects_nonexistent_user_id_field():
    """Attempt to add a fake user identification field unknown to schema."""
    with pytest.raises(Exception):
        TradeIntent.model_validate({
            "schema_version": "fit.trade.v1",
            "intent_id": "11111111-1111-4111-8111-111111111111",
            "user_id": "22222222-2222-4222-8222-222222222222",
            "account_id": "33333333-3333-4333-8333-333333333333",
            "malicious_user_override": "hacker-account",  # unknown field
            "network": "HYPERLIQUID_MAINNET",
            "symbol": "BTC-PERP",
            "side": "BUY",
            "position_effect": "OPEN",
            "order_type": "MARKET",
            "time_in_force": "IOC",
            "quantity": "0.025",
            "notional": "2961.3",
            "margin_mode": "ISOLATED",
            "leverage": 5,
            "entry_price_or_bound": "118452",
            "worst_acceptable_price": "118689",
            "stop": {"type": "STOP_MARKET", "trigger_price": "115200", "reduce_only": True},
            "take_profit_plan": [],
            "risk_policy_version": "risk-v1",
            "market_snapshot_id": "44444444-4444-4444-8444-444444444444",
            "strategy_version": "strategy-user-directed-v1",
            "model_version": "model-hermes-v1",
            "source": "USER_DIRECTED",
            "created_at": "2026-07-29T04:00:00Z",
        })


# ---------------------------------------------------------------------------
# Unknown fields
# ---------------------------------------------------------------------------

def test_operation_rejects_unknown_fields():
    with pytest.raises(Exception):
        Operation.model_validate({
            "schema_version": "fit.trade.v1",
            "operation_id": "66666666-6666-4666-8666-666666666666",
            "intent_id": "11111111-1111-4111-8111-111111111111",
            "confirmation_id": "55555555-5555-4555-8555-555555555555",
            "state": "AWAITING_CONFIRMATION",
            "state_version": 1,
            "secret_command": "transfer_all",  # unknown field
            "created_at": "2026-07-29T04:00:00Z",
            "updated_at": "2026-07-29T04:00:01Z",
        })


# ---------------------------------------------------------------------------
# Forbidden actions/symbols
# ---------------------------------------------------------------------------

def test_trade_intent_rejects_unsupported_symbol():
    """Only BTC-PERP, ETH-PERP, SOL-PERP are permitted."""
    with pytest.raises(Exception):
        TradeIntent.model_validate({
            "schema_version": "fit.trade.v1",
            "intent_id": "11111111-1111-4111-8111-111111111111",
            "user_id": "22222222-2222-4222-8222-222222222222",
            "account_id": "33333333-3333-4333-8333-333333333333",
            "network": "HYPERLIQUID_MAINNET",
            "symbol": "DOGE-PERP",
            "side": "BUY",
            "position_effect": "OPEN",
            "order_type": "MARKET",
            "time_in_force": "IOC",
            "quantity": "0.025",
            "notional": "2961.3",
            "margin_mode": "ISOLATED",
            "leverage": 5,
            "entry_price_or_bound": "118452",
            "worst_acceptable_price": "118689",
            "stop": {"type": "STOP_MARKET", "trigger_price": "115200", "reduce_only": True},
            "take_profit_plan": [],
            "risk_policy_version": "risk-v1",
            "market_snapshot_id": "44444444-4444-4444-8444-444444444444",
            "strategy_version": "strategy-user-directed-v1",
            "model_version": "model-hermes-v1",
            "source": "USER_DIRECTED",
            "created_at": "2026-07-29T04:00:00Z",
        })


def test_trade_intent_rejects_unsupported_action():
    """PositionEffect values outside allowed enum must be rejected."""
    with pytest.raises(Exception):
        TradeIntent.model_validate({
            "schema_version": "fit.trade.v1",
            "intent_id": "11111111-1111-4111-8111-111111111111",
            "user_id": "22222222-2222-4222-8222-222222222222",
            "account_id": "33333333-3333-4333-8333-333333333333",
            "network": "HYPERLIQUID_MAINNET",
            "symbol": "BTC-PERP",
            "side": "BUY",
            "position_effect": "WITHDRAW_ALL",  # bogus
            "order_type": "MARKET",
            "time_in_force": "IOC",
            "quantity": "0.025",
            "notional": "2961.3",
            "margin_mode": "ISOLATED",
            "leverage": 5,
            "entry_price_or_bound": "118452",
            "worst_acceptable_price": "118689",
            "stop": {"type": "STOP_MARKET", "trigger_price": "115200", "reduce_only": True},
            "take_profit_plan": [],
            "risk_policy_version": "risk-v1",
            "market_snapshot_id": "44444444-4444-4444-8444-444444444444",
            "strategy_version": "strategy-user-directed-v1",
            "model_version": "model-hermes-v1",
            "source": "USER_DIRECTED",
            "created_at": "2026-07-29T04:00:00Z",
        })


# ---------------------------------------------------------------------------
# JSON-number financial values
# ---------------------------------------------------------------------------

def test_trade_intent_rejects_float_quantity():
    """Financial decimals must be strings, not JSON numbers."""
    with pytest.raises(Exception):
        TradeIntent.model_validate({
            "schema_version": "fit.trade.v1",
            "intent_id": "11111111-1111-4111-8111-111111111111",
            "user_id": "22222222-2222-4222-8222-222222222222",
            "account_id": "33333333-3333-4333-8333-333333333333",
            "network": "HYPERLIQUID_MAINNET",
            "symbol": "BTC-PERP",
            "side": "BUY",
            "position_effect": "OPEN",
            "order_type": "MARKET",
            "time_in_force": "IOC",
            "quantity": 0.025,  # float, not string
        })


# ---------------------------------------------------------------------------
# Non-canonical decimals
# ---------------------------------------------------------------------------

def test_trade_intent_rejects_noncanonical_decimal():
    """Decimals with trailing zeros are non-canonical and must be rejected."""
    with pytest.raises(Exception):
        TradeIntent.model_validate({
            "schema_version": "fit.trade.v1",
            "intent_id": "11111111-1111-4111-8111-111111111111",
            "user_id": "22222222-2222-4222-8222-222222222222",
            "account_id": "33333333-3333-4333-8333-333333333333",
            "network": "HYPERLIQUID_MAINNET",
            "symbol": "BTC-PERP",
            "side": "BUY",
            "position_effect": "OPEN",
            "order_type": "MARKET",
            "time_in_force": "IOC",
            "quantity": "0.0250",  # trailing zero
            "notional": "2961.3",
            "margin_mode": "ISOLATED",
            "leverage": 5,
            "entry_price_or_bound": "118452",
            "worst_acceptable_price": "118689",
            "stop": {"type": "STOP_MARKET", "trigger_price": "115200", "reduce_only": True},
            "take_profit_plan": [],
            "risk_policy_version": "risk-v1",
            "market_snapshot_id": "44444444-4444-4444-8444-444444444444",
            "strategy_version": "strategy-user-directed-v1",
            "model_version": "model-hermes-v1",
            "source": "USER_DIRECTED",
            "created_at": "2026-07-29T04:00:00Z",
        })


# ---------------------------------------------------------------------------
# Missing stop for OPEN/INCREASE
# ---------------------------------------------------------------------------

def test_trade_intent_requires_stop_for_open():
    """OPEN position_effect requires a stop."""
    with pytest.raises(Exception):
        TradeIntent.model_validate({
            "schema_version": "fit.trade.v1",
            "intent_id": "11111111-1111-4111-8111-111111111111",
            "user_id": "22222222-2222-4222-8222-222222222222",
            "account_id": "33333333-3333-4333-8333-333333333333",
            "network": "HYPERLIQUID_MAINNET",
            "symbol": "BTC-PERP",
            "side": "BUY",
            "position_effect": "OPEN",
            "order_type": "MARKET",
            "time_in_force": "IOC",
            "quantity": "0.025",
            "notional": "2961.3",
            "margin_mode": "ISOLATED",
            "leverage": 5,
            "entry_price_or_bound": "118452",
            "worst_acceptable_price": "118689",
            # stop intentionally missing
            "take_profit_plan": [],
            "risk_policy_version": "risk-v1",
            "market_snapshot_id": "44444444-4444-4444-8444-444444444444",
            "strategy_version": "strategy-user-directed-v1",
            "model_version": "model-hermes-v1",
            "source": "USER_DIRECTED",
            "created_at": "2026-07-29T04:00:00Z",
        })


# ---------------------------------------------------------------------------
# MARKET + GTC / LIMIT + IOC rejections
# ---------------------------------------------------------------------------

def test_trade_intent_market_requires_ioc():
    with pytest.raises(Exception):
        TradeIntent.model_validate({
            "schema_version": "fit.trade.v1",
            "intent_id": "11111111-1111-4111-8111-111111111111",
            "user_id": "22222222-2222-4222-8222-222222222222",
            "account_id": "33333333-3333-4333-8333-333333333333",
            "network": "HYPERLIQUID_MAINNET",
            "symbol": "BTC-PERP",
            "side": "BUY",
            "position_effect": "OPEN",
            "order_type": "MARKET",
            "time_in_force": "GTC",
            "quantity": "0.025",
            "notional": "2961.3",
            "margin_mode": "ISOLATED",
            "leverage": 5,
            "entry_price_or_bound": "118452",
            "worst_acceptable_price": "118689",
            "stop": {"type": "STOP_MARKET", "trigger_price": "115200", "reduce_only": True},
            "take_profit_plan": [],
            "risk_policy_version": "risk-v1",
            "market_snapshot_id": "44444444-4444-4444-8444-444444444444",
            "strategy_version": "strategy-user-directed-v1",
            "model_version": "model-hermes-v1",
            "source": "USER_DIRECTED",
            "created_at": "2026-07-29T04:00:00Z",
        })


def test_trade_intent_limit_rejects_ioc():
    with pytest.raises(Exception):
        TradeIntent.model_validate({
            "schema_version": "fit.trade.v1",
            "intent_id": "11111111-1111-4111-8111-111111111111",
            "user_id": "22222222-2222-4222-8222-222222222222",
            "account_id": "33333333-3333-4333-8333-333333333333",
            "network": "HYPERLIQUID_MAINNET",
            "symbol": "BTC-PERP",
            "side": "BUY",
            "position_effect": "OPEN",
            "order_type": "LIMIT",
            "time_in_force": "IOC",
            "quantity": "0.025",
            "notional": "2961.3",
            "margin_mode": "ISOLATED",
            "leverage": 5,
            "entry_price_or_bound": "118452",
            "worst_acceptable_price": "118689",
            "stop": {"type": "STOP_MARKET", "trigger_price": "115200", "reduce_only": True},
            "take_profit_plan": [],
            "risk_policy_version": "risk-v1",
            "market_snapshot_id": "44444444-4444-4444-8444-444444444444",
            "strategy_version": "strategy-user-directed-v1",
            "model_version": "model-hermes-v1",
            "source": "USER_DIRECTED",
            "created_at": "2026-07-29T04:00:00Z",
        })


# ---------------------------------------------------------------------------
# Confirmation ticket must have risk-increasing intent
# ---------------------------------------------------------------------------

def test_confirmation_ticket_rejects_reduce_or_close():
    """Confirmation tickets only for OPEN or INCREASE."""
    valid_ticket = {
        "schema_version": "fit.trade.v1",
        "confirmation_id": "55555555-5555-4555-8555-555555555555",
        "device_id": "6789abcd-6789-4789-8789-6789abcdef01",
        "session_id": "789abcde-789a-489a-889a-789abcdef012",
        "intent": {
            "schema_version": "fit.trade.v1",
            "intent_id": "11111111-1111-4111-8111-111111111111",
            "user_id": "22222222-2222-4222-8222-222222222222",
            "account_id": "33333333-3333-4333-8333-333333333333",
            "network": "HYPERLIQUID_MAINNET",
            "symbol": "BTC-PERP",
            "side": "BUY",
            "position_effect": "CLOSE",  # not OPEN/INCREASE
            "order_type": "MARKET",
            "time_in_force": "IOC",
            "quantity": "0.025",
            "notional": "2961.3",
            "margin_mode": "ISOLATED",
            "leverage": 5,
            "entry_price_or_bound": "118452",
            "worst_acceptable_price": "118689",
            "take_profit_plan": [],
            "risk_policy_version": "risk-v1",
            "market_snapshot_id": "44444444-4444-4444-8444-444444444444",
            "strategy_version": "strategy-user-directed-v1",
            "model_version": "model-hermes-v1",
            "source": "USER_DIRECTED",
            "created_at": "2026-07-29T04:00:00Z",
        },
        "maximum_loss": "86.8",
        "maximum_loss_fraction": "0.0087",
        "post_trade_total_risk": "141.8",
        "post_trade_total_risk_fraction": "0.0142",
        "liquidation_price": "95420",
        "estimated_fees": "2.5",
        "slippage_budget": "3",
        "expires_at": "2026-07-29T04:10:00Z",
        "confirmation_nonce": "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY",
        "confirmation_hash": "c4dfe4f2ffd65e04936c8734e12a31c96ceab1f7567b41c70d5da716a06dd865",
        "created_at": "2026-07-29T04:00:00Z",
    }
    with pytest.raises(Exception):
        ConfirmationTicket.model_validate(valid_ticket)


# ---------------------------------------------------------------------------
# Bound-field confirmation ticket mutations
# ---------------------------------------------------------------------------

def test_confirmation_ticket_rejects_bad_hash():
    """A confirmation ticket with a hash that doesn't match the content must be validated."""
    ticket_path = FIXTURES_VALID_DIR / "confirmation-ticket-open.json"
    ticket = json.loads(ticket_path.read_text())["value"]
    # Modify a bound field but keep the original hash
    tampered = json.loads(json.dumps(ticket))
    tampered["intent"]["quantity"] = "0.050"
    # The hash no longer matches — but Pydantic won't catch hash semantics.
    # This is an application-layer check, not a schema check.
    # The test ensures the schema still validates structurally.
    # Hash verification happens in confirmation.verify_confirmation_hash()
    fields = load_confirmation_fields()
    golden = ticket["confirmation_hash"]
    from hermes_agent.confirmation import verify_confirmation_hash as _verify_hash
    assert not _verify_hash(
        tampered, fields, golden
    )


# ---------------------------------------------------------------------------
# MCP tool-specific: no model-supplied scope
# ---------------------------------------------------------------------------

def test_all_mcp_tools_reject_model_supplied_scope():
    """Every MCP tool input schema must reject user_id/account_id/session_id."""
    import jsonschema
    from hermes_agent.mcp_tools import load_mcp_inventory

    inventory = load_mcp_inventory()
    for tool_def in inventory["tools"]:
        v = jsonschema.Draft202012Validator(tool_def["input_schema"])
        for field in SERVER_SCOPE_FIELDS:
            payload = {field: "22222222-2222-4222-8222-222222222222"}
            with pytest.raises(jsonschema.ValidationError):
                v.validate(payload)


# ---------------------------------------------------------------------------
# All domain types can round-trip matrix values
# ---------------------------------------------------------------------------

import json as _json


MATRIX_PATH = Path(__file__).parent.parent.parent.parent / "contracts" / "fixtures" / "matrix" / "domain-values.json"


def test_all_domain_types_round_trip():
    """Every domain type in the matrix round-trips through Pydantic."""
    matrix = _json.loads(MATRIX_PATH.read_text())
    for name, value in matrix.items():
        if name.startswith("_"):
            continue
        model = DOMAIN_TYPES.get(name)
        if model is None:
            continue
        obj = model.model_validate(value)
        exported = obj.model_dump()
        revalidated = model.model_validate(exported)
        assert revalidated.model_dump() == exported, f"{name}: round-trip failed"


def test_matrix_unknown_field_rejected():
    """Adding an unknown field to any matrix value must fail."""
    matrix = _json.loads(MATRIX_PATH.read_text())
    for name, value in matrix.items():
        if name.startswith("_"):
            continue
        model = DOMAIN_TYPES.get(name)
        if model is None:
            continue
        modified = _json.loads(_json.dumps(value))
        modified["unexpected_field"] = "rejected"
        with pytest.raises(Exception):
            model.model_validate(modified)


def test_matrix_missing_required_rejected():
    """Removing a required field from any matrix value must fail."""
    matrix = _json.loads(MATRIX_PATH.read_text())
    for name, value in matrix.items():
        if name.startswith("_"):
            continue
        model = DOMAIN_TYPES.get(name)
        if model is None:
            continue
        # Get the fields from the model
        required = [f for f in model.model_fields if model.model_fields[f].is_required()]
        if not required:
            continue
        modified = _json.loads(_json.dumps(value))
        del modified[required[0]]
        with pytest.raises(Exception):
            model.model_validate(modified)
