"""FIT-Trade MCP tool inventory — loads and validates the frozen ten-tool surface.

Consumes contracts/mcp-tools-v1.json and validates it against
contracts/jsonschema/mcp-tools-v1.schema.json with full format checking.
Derives tool names, risk effects, and all invariants from the validated
inventory — never from hardcoded duplicates.

Rejects forbidden capabilities: raw signing, raw order, withdraw, transfer,
SQL, shell, exec, spawn.
"""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any

from jsonschema import Draft202012Validator, SchemaError, ValidationError

# Relative to the repo root (services/hermes-agent/)
CONTRACTS_ROOT = Path(__file__).resolve().parent.parent.parent.parent.parent / "contracts"

MCP_TOOLS_PATH = CONTRACTS_ROOT / "mcp-tools-v1.json"
MCP_SCHEMA_PATH = CONTRACTS_ROOT / "jsonschema" / "mcp-tools-v1.schema.json"

FORBIDDEN_PATTERN = re.compile(
    r"(raw|sign|private.?key|wallet|withdraw|transfer|sql|shell|exec|spawn)",
    re.IGNORECASE,
)

FORBIDDEN_CAPABILITIES = {
    "raw_signing", "raw_sign", "sign_order", "sign_transaction",
    "place_order", "place_raw_order", "submit_order",
    "withdraw", "request_withdrawal", "transfer", "transfer_funds",
    "execute_sql", "run_query", "exec_shell", "spawn_process",
    "read_private_key", "get_wallet", "wallet_action",
}

SERVER_SCOPE_FIELDS = {"user_id", "account_id", "session_id"}
SERVER_SCOPE_CONST = ["user_id", "account_id", "session_id"]


# ---------------------------------------------------------------------------
# Cached validators and inventory
# ---------------------------------------------------------------------------

_inventory_schema_validator: Draft202012Validator | None = None
_validated_inventory: list[dict[str, Any]] | None = None


def _load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text())


def load_mcp_inventory() -> dict[str, Any]:
    """Load the frozen MCP tool inventory from contracts."""
    if not MCP_TOOLS_PATH.exists():
        raise FileNotFoundError(f"MCP inventory not found: {MCP_TOOLS_PATH}")
    return _load_json(MCP_TOOLS_PATH)


def _get_inventory_schema_validator() -> Draft202012Validator:
    """Return a cached Draft202012Validator for mcp-tools-v1.schema.json with format checking."""
    global _inventory_schema_validator
    if _inventory_schema_validator is None:
        schema = _load_json(MCP_SCHEMA_PATH)
        _inventory_schema_validator = Draft202012Validator(
            schema, format_checker=Draft202012Validator.FORMAT_CHECKER,
        )
    return _inventory_schema_validator


def validate_mcp_inventory(inventory: dict[str, Any] | None = None) -> list[dict[str, Any]]:
    """Validate the MCP tool inventory against the frozen JSON Schema and invariants.

    Steps:
      1. Validate the inventory document against mcp-tools-v1.schema.json (format checking on).
      2. Enforce generic security invariants: forbidden capabilities, exact count 10,
         server-injected scope, additionalProperties false.
      3. Cache and return the validated tool list.

    Raises ValidationError or ValueError on any violation.
    """
    if inventory is None:
        inventory = load_mcp_inventory()

    # 1. Validate against the frozen JSON Schema with format checking
    validator = _get_inventory_schema_validator()
    try:
        validator.validate(inventory)
    except ValidationError as e:
        raise ValueError(f"MCP inventory schema validation failed: {e.message}") from e

    tools: list[dict[str, Any]] = inventory.get("tools", [])

    # 2. Generic security invariants (schema enforces count=10 but double-check)
    if len(tools) != 10:
        raise ValueError(f"MCP inventory must have exactly 10 tools, got {len(tools)}")

    # 3. Forbidden capabilities
    for tool in tools:
        name = tool["name"]

        # Forbidden name patterns
        if FORBIDDEN_PATTERN.search(name):
            raise ValueError(f"MCP tool '{name}' matches forbidden pattern")

        # Forbidden explicit tool names
        if name in FORBIDDEN_CAPABILITIES:
            raise ValueError(f"MCP tool '{name}' is a forbidden capability")

        # input_schema must have additionalProperties: false
        input_schema = tool.get("input_schema", {})
        if input_schema.get("additionalProperties") is not False:
            raise ValueError(
                f"MCP tool '{name}' input_schema must forbid additionalProperties"
            )

        # Server-injected fields must NOT appear in input_schema properties
        props = input_schema.get("properties", {})
        for field in SERVER_SCOPE_FIELDS:
            if field in props:
                raise ValueError(
                    f"MCP tool '{name}' exposes server-injected field '{field}' in input_schema"
                )

    return tools


def get_tool_names() -> list[str]:
    """Return the sorted list of validated tool names, derived from the frozen inventory."""
    tools = validate_mcp_inventory()
    return sorted(t["name"] for t in tools)


def get_tool_by_name(name: str) -> dict[str, Any]:
    """Get a validated tool definition by name."""
    tools = validate_mcp_inventory()
    for tool in tools:
        if tool["name"] == name:
            return tool
    raise KeyError(f"MCP tool not found: {name}")
