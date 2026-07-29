"""Test the MCP tool inventory against all frozen invariants."""
import pytest
import jsonschema
from hermes_agent.mcp_tools import (
    validate_mcp_inventory,
    get_tool_by_name,
    get_tool_names,
    FORBIDDEN_PATTERN,
    FORBIDDEN_CAPABILITIES,
    SERVER_SCOPE_FIELDS,
    load_mcp_inventory,
)


def test_mcp_inventory_has_exactly_ten_tools():
    tools = validate_mcp_inventory()
    assert len(tools) == 10


def test_mcp_inventory_names_derived_from_inventory():
    """get_tool_names must derive names from the validated inventory only."""
    tools = validate_mcp_inventory()
    actual = sorted(t["name"] for t in tools)
    names = get_tool_names()
    assert names == actual


def test_mcp_inventory_names_match_schema_enum():
    """Tool names must match the schema enum (validated by JSON Schema)."""
    tools = validate_mcp_inventory()
    allowed = {
        "get_market_snapshot", "get_account_state", "get_positions",
        "get_open_orders", "get_risk_limits", "create_trade_intent",
        "request_reduce_position", "cancel_entry_order", "tighten_stop",
        "submit_trade_feedback",
    }
    actual = {t["name"] for t in tools}
    assert actual == allowed


def test_mcp_inventory_risk_effects_are_valid():
    """Every tool must have a valid risk_effect from the frozen enum."""
    tools = validate_mcp_inventory()
    valid_effects = {"READ_ONLY", "PROPOSE_INCREASE", "REDUCE_RISK", "LEARNING_ONLY"}
    for tool in tools:
        assert tool["risk_effect"] in valid_effects, (
            f"{tool['name']}: invalid risk_effect '{tool['risk_effect']}'"
        )


def test_mcp_inventory_no_forbidden_names():
    tools = validate_mcp_inventory()
    for tool in tools:
        assert not FORBIDDEN_PATTERN.search(tool["name"]), (
            f"forbidden MCP name: {tool['name']}"
        )
        assert tool["name"] not in FORBIDDEN_CAPABILITIES, (
            f"forbidden capability: {tool['name']}"
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
    tools = validate_mcp_inventory()
    tool_names = {t["name"] for t in tools}
    for forbidden in FORBIDDEN_CAPABILITIES:
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


def test_mcp_inventory_validates_against_schema_with_format():
    """The entire inventory document must validate against mcp-tools-v1.schema.json."""
    inventory = load_mcp_inventory()
    from jsonschema import Draft202012Validator
    from hermes_agent.mcp_tools import MCP_SCHEMA_PATH, _load_json as _load
    schema = _load(MCP_SCHEMA_PATH)
    validator = Draft202012Validator(
        schema, format_checker=Draft202012Validator.FORMAT_CHECKER,
    )
    validator.validate(inventory)  # should not raise


def test_get_tool_names_returns_from_validated_inventory():
    """get_tool_names must return names sorted from the validated inventory."""
    names = get_tool_names()
    assert len(names) == 10
    assert names == sorted(names)  # must be sorted
    # Verify each name resolves to a tool
    for name in names:
        get_tool_by_name(name)  # should not raise
