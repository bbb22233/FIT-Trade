"""FIT-Trade P0 validator — wires contracts, MCP tools, and confirmation hash.

Primary entry point for P0-003 acceptance checks.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from jsonschema import Draft202012Validator, SchemaError

from . import confirmation
from .contracts import BaseDomain, DOMAIN_TYPES
from .mcp_tools import validate_mcp_inventory

CONTRACTS_ROOT = Path(__file__).resolve().parent.parent.parent.parent.parent / "contracts"
SCHEMA_PATH = CONTRACTS_ROOT / "jsonschema" / "fit-trade-v1.schema.json"
CONFIRMATION_FIELDS_PATH = CONTRACTS_ROOT / "confirmation-fields.json"
FIXTURES_VALID_DIR = CONTRACTS_ROOT / "fixtures" / "valid"
FIXTURES_INVALID_DIR = CONTRACTS_ROOT / "fixtures" / "invalid"
GOLDEN_DIR = CONTRACTS_ROOT / "fixtures" / "golden"


def _load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text())


# ---------------------------------------------------------------------------
# Fixture helpers
# ---------------------------------------------------------------------------

def _load_fixture(path: Path) -> tuple[str, dict[str, Any]]:
    """Load a fixture file. Returns (schema_name, value)."""
    fixture = _load_json(path)
    return fixture["schema"], fixture["value"]


def load_valid_fixtures() -> list[tuple[str, dict[str, Any], Path]]:
    """Load all valid fixtures. Returns list of (schema_name, value, path)."""
    result = []
    for p in sorted(FIXTURES_VALID_DIR.glob("*.json")):
        schema_name, value = _load_fixture(p)
        result.append((schema_name, value, p))
    return result


def load_invalid_fixtures() -> list[tuple[str, dict[str, Any], str, Path]]:
    """Load all invalid fixtures. Returns list of (schema_name, value, reason, path)."""
    result = []
    for p in sorted(FIXTURES_INVALID_DIR.glob("*.json")):
        fixture = _load_json(p)
        result.append((fixture["schema"], fixture["value"], fixture["reason"], p))
    return result


# ---------------------------------------------------------------------------
# Pydantic validation
# ---------------------------------------------------------------------------

def validate_with_pydantic(schema_name: str, value: dict[str, Any]) -> BaseDomain:
    """Validate a domain value using the strict Pydantic model.

    Raises ValidationError on failure.
    Rejects unknown fields, non-canonical decimals, unsupported
    symbols/actions, and any prompt-injected user_id/account_id/session_id.
    """
    model = DOMAIN_TYPES.get(schema_name)
    if model is None:
        raise ValueError(f"Unknown schema: {schema_name}")
    return model.model_validate(value)


# ---------------------------------------------------------------------------
# JSON Schema validation (for cross-check)
# ---------------------------------------------------------------------------

def _compile_validators() -> dict[str, Draft202012Validator]:
    schema = _load_json(SCHEMA_PATH)
    validators: dict[str, Draft202012Validator] = {}
    for name, defn in schema["$defs"].items():
        validators[name] = Draft202012Validator(defn, registry={"$id": schema["$id"]})
    return validators


def validate_with_jsonschema(schema_name: str, value: dict[str, Any]) -> None:
    """Validate a domain value using the raw JSON Schema.

    Raises jsonschema.ValidationError on failure.
    """
    validators = _compile_validators()
    v = validators.get(schema_name)
    if v is None:
        raise ValueError(f"Unknown schema: {schema_name}")
    v.validate(value)


# ---------------------------------------------------------------------------
# Confirmation hash
# ---------------------------------------------------------------------------

def load_confirmation_fields() -> list[str]:
    """Load the confirmation binding field list."""
    data = _load_json(CONFIRMATION_FIELDS_PATH)
    assert data["schema_version"] == "fit.confirmation-hash.v1"
    assert data["algorithm"] == "sha256"
    assert data["canonicalization"] == "RFC8785-JCS"
    assert len(set(data["fields"])) == len(data["fields"]), "duplicate fields"
    return list(data["fields"])


def load_golden_hash() -> str:
    """Load the golden confirmation hash vector."""
    return (GOLDEN_DIR / "confirmation-open.sha256").read_text().strip()


# ---------------------------------------------------------------------------
# Full acceptance check
# ---------------------------------------------------------------------------

def run_acceptance_checks() -> dict[str, Any]:
    """Run all P0-003 acceptance checks.

    Returns a dict with pass/fail status for each check.
    """
    results: dict[str, Any] = {}

    # 1. All valid fixtures pass Pydantic validation
    valid_ok = True
    for schema_name, value, path in load_valid_fixtures():
        try:
            validate_with_pydantic(schema_name, value)
        except Exception as e:
            results.setdefault("valid_failures", []).append(
                {"fixture": path.name, "error": str(e)}
            )
            valid_ok = False
    results["valid_fixtures"] = "PASS" if valid_ok else "FAIL"

    # 2. All invalid fixtures are rejected
    invalid_ok = True
    for schema_name, value, reason, path in load_invalid_fixtures():
        try:
            validate_with_pydantic(schema_name, value)
            results.setdefault("invalid_acceptances", []).append(
                {"fixture": path.name, "reason": reason}
            )
            invalid_ok = False
        except Exception:
            pass  # expected
    results["invalid_fixtures"] = "PASS" if invalid_ok else "FAIL"

    # 3. MCP inventory
    try:
        validate_mcp_inventory()
        results["mcp_inventory"] = "PASS"
    except Exception as e:
        results["mcp_inventory"] = f"FAIL: {e}"

    # 4. Confirmation hash golden vector
    try:
        fields = load_confirmation_fields()
        golden = load_golden_hash()
        ticket_fixture = _load_json(FIXTURES_VALID_DIR / "confirmation-ticket-open.json")
        ticket = ticket_fixture["value"]
        computed = confirmation.compute_confirmation_hash(ticket, fields)
        if computed == golden:
            results["confirmation_hash"] = "PASS"
        else:
            results["confirmation_hash"] = (
                f"FAIL: computed={computed}, expected={golden}"
            )
    except Exception as e:
        results["confirmation_hash"] = f"FAIL: {e}"

    # 5. Confirmation hash field mutation sensitivity
    try:
        fields = load_confirmation_fields()
        golden = load_golden_hash()
        ticket_fixture = _load_json(FIXTURES_VALID_DIR / "confirmation-ticket-open.json")
        ticket = ticket_fixture["value"]
        binding = confirmation.confirmation_binding(ticket, fields)
        mutated_any = False
        ignored_fields = []
        for field in fields:
            import copy
            mutated = copy.deepcopy(binding)
            mutated[field] = _mutate_value(mutated[field])
            mutated_hash = hashlib.sha256(
                confirmation.canonicalize(mutated).encode("utf-8")
            ).hexdigest()
            if mutated_hash == golden:
                ignored_fields.append(field)
                mutated_any = True
        if not mutated_any:
            results["confirmation_field_sensitivity"] = "PASS"
        else:
            results["confirmation_field_sensitivity"] = f"FAIL: ignored {ignored_fields}"
    except Exception as e:
        results["confirmation_field_sensitivity"] = f"FAIL: {e}"

    return results


def _mutate_value(value: Any) -> Any:
    """Mutate a value for field sensitivity testing."""
    if isinstance(value, str):
        return value + "x"
    if isinstance(value, bool):
        return not value
    if isinstance(value, int):
        return value + 1
    if isinstance(value, float):
        return value + 1.0
    if isinstance(value, list):
        return value + [{"mutation": True}]
    if value is None:
        return "mutation"
    return {**value, "mutation": True}


import hashlib  # needed at module scope for the checks
