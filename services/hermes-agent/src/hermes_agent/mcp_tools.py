"""FIT-Trade MCP tool inventory — loads and validates the frozen ten-tool surface.

Consumes contracts/mcp-tools-v1.json and exposes only that tool list.
Rejects forbidden capabilities: raw signing, raw order, withdraw, transfer,
SQL, shell, exec, spawn.
"""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any

# Relative to the repo root (services/hermes-agent/)
CONTRACTS_ROOT = Path(__file__).resolve().parent.parent.parent.parent.parent / "contracts"

MCP_TOOLS_PATH = CONTRACTS_ROOT / "mcp-tools-v1.json"
MCP_SCHEMA_PATH = CONTRACTS_ROOT / "jsonschema" / "mcp-tools-v1.schema.json"

FORBIDDEN_PATTERN = re.compile(
    r"(raw|sign|private.?key|wallet|withdraw|transfer|sql|shell|exec|spawn)",
    re.IGNORECASE,
)

EXPECTED_TOOL_NAMES = sorted([
    "cancel_entry_order",
    "create_trade_intent",
    "get_account_state",
    "get_market_snapshot",
    "get_open_orders",
    "get_positions",
    "get_risk_limits",
    "request_reduce_position",
    "submit_trade_feedback",
    "tighten_stop",
])

EXPECTED_RISK_EFFECTS: dict[str, str] = {
    "cancel_entry_order": "REDUCE_RISK",
    "create_trade_intent": "PROPOSE_INCREASE",
    "get_account_state": "READ_ONLY",
    "get_market_snapshot": "READ_ONLY",
    "get_open_orders": "READ_ONLY",
    "get_positions": "READ_ONLY",
    "get_risk_limits": "READ_ONLY",
    "request_reduce_position": "REDUCE_RISK",
    "submit_trade_feedback": "LEARNING_ONLY",
    "tighten_stop": "REDUCE_RISK",
}

SERVER_SCOPE_FIELDS = {"user_id", "account_id", "session_id"}


def load_mcp_inventory() -> dict[str, Any]:
    """Load the frozen MCP tool inventory from contracts."""
    if not MCP_TOOLS_PATH.exists():
        raise FileNotFoundError(f"MCP inventory not found: {MCP_TOOLS_PATH}")
    return json.loads(MCP_TOOLS_PATH.read_text())


def validate_mcp_inventory(inventory: dict[str, Any] | None = None) -> list[dict[str, Any]]:
    """Validate the MCP tool inventory against all frozen invariants.

    Returns the list of validated tool definitions.

    Raises ValueError on any violation.
    """
    if inventory is None:
        inventory = load_mcp_inventory()

    if inventory.get("schema_version") != "fit.mcp.v1":
        raise ValueError("MCP inventory must be schema_version fit.mcp.v1")

    tools: list[dict[str, Any]] = inventory.get("tools", [])
    if len(tools) != 10:
        raise ValueError(f"MCP inventory must have exactly 10 tools, got {len(tools)}")

    # Verify all expected names are present
    actual_names = sorted(t.get("name", "") for t in tools)
    if actual_names != EXPECTED_TOOL_NAMES:
        raise ValueError(
            f"MCP tool names mismatch: expected {EXPECTED_TOOL_NAMES}, got {actual_names}"
        )

    for tool in tools:
        name = tool["name"]
        risk = tool.get("risk_effect")

        # Forbidden names
        if FORBIDDEN_PATTERN.search(name):
            raise ValueError(f"MCP tool '{name}' matches forbidden pattern")

        # Risk effect must match
        expected_risk = EXPECTED_RISK_EFFECTS.get(name)
        if risk != expected_risk:
            raise ValueError(
                f"MCP tool '{name}' has risk_effect '{risk}', expected '{expected_risk}'"
            )

        # Server-injected scope must be correct
        scope = tool.get("server_injected_scope", [])
        if sorted(scope) != sorted(SERVER_SCOPE_FIELDS):
            raise ValueError(
                f"MCP tool '{name}' has wrong server_injected_scope: {scope}"
            )

        # Input schema must reject server-injected fields
        input_schema = tool.get("input_schema", {})
        props = input_schema.get("properties", {})
        for field in SERVER_SCOPE_FIELDS:
            if field in props:
                raise ValueError(
                    f"MCP tool '{name}' exposes server-injected field '{field}' in input_schema"
                )

        # Every input_schema must have additionalProperties: false
        if input_schema.get("additionalProperties") is not False:
            raise ValueError(f"MCP tool '{name}' input_schema must forbid additionalProperties")

    return tools


def get_tool_by_name(name: str) -> dict[str, Any]:
    """Get a validated tool definition by name."""
    tools = validate_mcp_inventory()
    for tool in tools:
        if tool["name"] == name:
            return tool
    raise KeyError(f"MCP tool not found: {name}")


def get_tool_names() -> list[str]:
    """Return the sorted list of validated tool names."""
    return EXPECTED_TOOL_NAMES.copy()
