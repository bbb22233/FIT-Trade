"""Test the MCP tool inventory against all frozen invariants."""
import pytest
from hermes_agent.mcp_tools import (
    validate_mcp_inventory,
    get_tool_by_name,
    get_tool_names,
    FORBIDDEN_PATTERN,
    EXPECTED_TOOL_NAMES,
    EXPECTED_RISK_EFFECTS,
    SERVER_SCOPE_FIELDS,
)


def test_mcp_inventory_has_exactly_ten_tools():
    tools = validate_mcp_inventory()
    assert len(tools) == 10


def test_mcp_inventory_names_match_expected():
    tools = validate_mcp_inventory()
    actual = sorted(t["name"] for t in tools)
    assert actual == EXPECTED_TOOL_NAMES


def test_mcp_inventory_risk_effects_correct():
    tools = validate_mcp_inventory()
    for tool in tools:
        name = tool["name"]
        assert tool["risk_effect"] == EXPECTED_RISK_EFFECTS[name], (
            f"{name}: wrong risk_effect"
        )


def test_mcp_inventory_no_forbidden_names():
    tools = validate_mcp_inventory()
    for tool in tools:
        assert not FORBIDDEN_PATTERN.search(tool["name"]), (
            f"forbidden MCP name: {tool['name']}"
        )


def test_mcp_inventory_server_injected_scope():
    """Every tool must inject user_id, account_id, session_id from server context."""
    tools = validate_mcp_inventory()
    for tool in tools:
        scope = set(tool.get("server_injected_scope", []))
        assert scope == SERVER_SCOPE_FIELDS, (
            f"{tool['name']}: server_injected_scope mismatch"
        )


def test_mcp_input_schemas_reject_server_fields():
    """Model input must not define user_id, account_id, or session_id."""
    tools = validate_mcp_inventory()
    for tool in tools:
        input_schema = tool.get("input_schema", {})
        props = input_schema.get("properties", {})
        for field in SERVER_SCOPE_FIELDS:
            assert field not in props, (
                f"{tool['name']}: model input exposes {field}"
            )


def test_mcp_input_schemas_forbid_additional_properties():
    """Every tool's input_schema must have additionalProperties: false."""
    tools = validate_mcp_inventory()
    for tool in tools:
        input_schema = tool.get("input_schema", {})
        assert input_schema.get("additionalProperties") is False, (
            f"{tool['name']}: allows additionalProperties"
        )


def test_mcp_forbidden_tool_types():
    """Verify no raw signing, order, withdraw, transfer, SQL, or shell tools exist."""
    FORBIDDEN_NAMES = {
        "raw_signing", "raw_sign", "sign_order", "sign_transaction",
        "place_order", "place_raw_order", "submit_order",
        "withdraw", "request_withdrawal", "transfer", "transfer_funds",
        "execute_sql", "run_query", "exec_shell", "spawn_process",
        "read_private_key", "get_wallet", "wallet_action",
    }
    tools = validate_mcp_inventory()
    tool_names = {t["name"] for t in tools}
    for forbidden in FORBIDDEN_NAMES:
        assert forbidden not in tool_names, f"forbidden MCP tool present: {forbidden}"


def test_mcp_create_trade_intent_enforces_ioc_for_market():
    """create_trade_intent must reject MARKET + GTC combination."""
    tool = get_tool_by_name("create_trade_intent")
    from jsonschema import Draft202012Validator
    v = Draft202012Validator(tool["input_schema"])
    invalid = {
        "symbol": "BTC-PERP",
        "side": "BUY",
        "position_effect": "OPEN",
        "order_type": "MARKET",
        "time_in_force": "GTC",
        "quantity": "0.025",
        "margin_mode": "ISOLATED",
        "leverage": 5,
        "entry_price_or_bound": "118452",
        "worst_acceptable_price": "118689",
        "stop_trigger": "115200",
    }
    with pytest.raises(Exception):
        v.validate(invalid)


def test_mcp_create_trade_intent_rejects_model_supplied_ids():
    """Model must not be able to inject user_id/account_id/session_id into tool inputs."""
    tool = get_tool_by_name("create_trade_intent")
    from jsonschema import Draft202012Validator
    v = Draft202012Validator(tool["input_schema"])
    valid = {
        "symbol": "BTC-PERP",
        "side": "BUY",
        "position_effect": "OPEN",
        "order_type": "MARKET",
        "time_in_force": "IOC",
        "quantity": "0.025",
        "margin_mode": "ISOLATED",
        "leverage": 5,
        "entry_price_or_bound": "118452",
        "worst_acceptable_price": "118689",
        "stop_trigger": "115200",
    }
    # Valid passes
    v.validate(valid)
    # Inject user_id should fail (additionalProperties: false)
    for field in SERVER_SCOPE_FIELDS:
        injected = {**valid, field: "22222222-2222-4222-8222-222222222222"}
        with pytest.raises(Exception):
            v.validate(injected)


def test_mcp_covers_get_tool_names():
    """get_tool_names returns the correct list."""
    assert get_tool_names() == EXPECTED_TOOL_NAMES
