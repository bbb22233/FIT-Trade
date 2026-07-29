"""FIT-Trade MCP tool inventory — loads and validates the frozen ten-tool surface.

Consumes contracts/mcp-tools-v1.json and validates it against
contracts/jsonschema/mcp-tools-v1.schema.json with full format checking.
Derives tool names, risk effects, and all invariants from the validated
inventory — never from hardcoded duplicates.

Rejects forbidden capabilities: raw signing, raw order, withdraw, transfer,
SQL, shell, exec, spawn.

P0-003 hardening: fail-closed for schema-valid tampering.
  - Expected tool names are derived from the frozen MCP schema enum,
    never from the inventory itself or from hardcoded lists.
  - Exact name coverage is enforced: inventory must contain every name
    in the schema enum and no others.
  - When a caller-supplied inventory dict is provided, it is validated
    against the schema and then deep-compared with the authoritative
    on-disk frozen inventory — any structural difference is rejected.
  - Replay consumption and cross-object aggregate protection enforcement
    (e.g. PROTECTION_FULL_COVERAGE) are deliberate Go-core responsibilities
    per coordination/tasks/P0-002.md; Python does not duplicate them.
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


# ---------------------------------------------------------------------------
# Cached validators and inventory
# ---------------------------------------------------------------------------

_inventory_schema_validator: Draft202012Validator | None = None
_validated_inventory: list[dict[str, Any]] | None = None
_schema_expected_names: set[str] | None = None


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


def _get_schema_expected_names() -> set[str]:
    """Derive the approved tool name set from the frozen MCP schema enum.

    The schema defines a closed enum of allowed tool names at
    $defs/Tool/properties/name/enum.  This is the sole authoritative source;
    no hardcoded lists or inventory-derived names are used for the coverage check.
    """
    global _schema_expected_names
    if _schema_expected_names is not None:
        return _schema_expected_names

    schema = _load_json(MCP_SCHEMA_PATH)
    try:
        name_prop = schema["$defs"]["Tool"]["properties"]["name"]
        _schema_expected_names = set(name_prop["enum"])
    except (KeyError, TypeError) as e:
        raise ValueError(
            "Frozen MCP schema missing expected $defs/Tool/properties/name/enum"
        ) from e

    # Sanity: the schema enum must contain exactly 10 names
    if len(_schema_expected_names) != 10:
        raise ValueError(
            f"Frozen MCP schema name enum must contain exactly 10 entries, "
            f"got {len(_schema_expected_names)}: {sorted(_schema_expected_names)}"
        )
    return _schema_expected_names


def validate_mcp_inventory(inventory: dict[str, Any] | None = None) -> list[dict[str, Any]]:
    """Validate the MCP tool inventory against the frozen JSON Schema and invariants.

    Steps:
      1. Load the authoritative on-disk frozen inventory.
      2. Validate the inventory document against mcp-tools-v1.schema.json
         (format checking on).
      3. Derive expected tool names from the frozen MCP schema enum and
         enforce exact name coverage (set equality).  Reject duplicate
         names, omitted approved names, or extra names.
      4. Enforce generic security invariants: forbidden capabilities,
         server-injected scope, additionalProperties false.
      5. When a caller-supplied inventory is provided, require exact
         structural equality with the authoritative on-disk frozen
         inventory after both have been independently schema-validated.
         Any difference in name, description, risk_effect, input_schema,
         or server_injected_scope is rejected.  This closes the attack
         vector where a schema-valid but differently-structured inventory
         (swapped risk effects, changed descriptions, modified schemas,
         added forbidden properties) could pass validation.
      6. Cache and return the validated tool list.

    Raises ValidationError or ValueError on any violation.
    """
    # 1. Load the authoritative on-disk frozen inventory
    authoritative = load_mcp_inventory()

    # Determine which inventory to validate; also track whether caller
    # supplied a custom inventory so we can enforce structural equality.
    caller_supplied = inventory is not None
    target = inventory if caller_supplied else authoritative

    # 2. Validate against the frozen JSON Schema with format checking
    validator = _get_inventory_schema_validator()
    try:
        validator.validate(target)
    except ValidationError as e:
        raise ValueError(f"MCP inventory schema validation failed: {e.message}") from e

    tools: list[dict[str, Any]] = target.get("tools", [])

    # 3. Derive expected names from the frozen schema and enforce exact coverage.
    expected_names = _get_schema_expected_names()
    actual_names: set[str] = {t.get("name", "") for t in tools}

    if len(actual_names) != len(tools):
        # Duplicate names — set construction above collapses them
        seen: set[str] = set()
        duplicates: set[str] = set()
        for t in tools:
            n: str = t.get("name", "")
            if n in seen:
                duplicates.add(n)
            seen.add(n)
        raise ValueError(
            f"MCP inventory contains duplicate tool names: {sorted(duplicates)}"
        )

    if actual_names != expected_names:
        missing = expected_names - actual_names
        extra = actual_names - expected_names
        parts = []
        if missing:
            parts.append(f"missing approved names: {sorted(missing)}")
        if extra:
            parts.append(f"extra unapproved names: {sorted(extra)}")
        raise ValueError(
            "MCP inventory tool names do not match frozen schema enum: " + "; ".join(parts)
        )

    # 4. Generic security invariants
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

    # 5. When caller supplied a custom inventory, require exact structural
    #    equality with the authoritative on-disk frozen inventory.
    #    Both have been independently schema-validated at this point.
    if caller_supplied:
        # Also validate authoritative against schema (belt-and-suspenders)
        try:
            validator.validate(authoritative)
        except ValidationError as e:
            raise ValueError(
                f"Authoritative on-disk MCP inventory failed schema validation: {e.message}"
            ) from e

        # Validate authoritative's invariants (name coverage, security) too
        auth_tools: list[dict[str, Any]] = authoritative.get("tools", [])
        auth_names: set[str] = {t.get("name", "") for t in auth_tools}
        if auth_names != expected_names:
            raise ValueError(
                "Authoritative on-disk MCP inventory has inconsistent tool names"
            )

        # Deep structural equality: must match after JSON round-trip to
        # normalize key ordering and whitespace.
        authoritative_normalized = json.loads(json.dumps(authoritative, sort_keys=True))
        supplied_normalized = json.loads(json.dumps(target, sort_keys=True))
        if authoritative_normalized != supplied_normalized:
            raise ValueError(
                "Supplied MCP inventory differs structurally from the authoritative "
                "on-disk frozen inventory.  Tampering with any tool name, description, "
                "risk_effect, input_schema, or server_injected_scope is rejected."
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
