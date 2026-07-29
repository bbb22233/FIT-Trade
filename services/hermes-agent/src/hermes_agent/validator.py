"""FIT-Trade P0 validator — wires contracts, MCP tools, and confirmation hash.

Primary entry point for P0-003 acceptance checks.
"""

from __future__ import annotations

import datetime as _datetime
import json
import re
from pathlib import Path
from typing import Any

from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource
from referencing.jsonschema import DRAFT202012 as _REF_DRAFT202012

from . import confirmation
from .contracts import BaseDomain, DOMAIN_TYPES
from .mcp_tools import validate_mcp_inventory

CONTRACTS_ROOT = Path(__file__).resolve().parent.parent.parent.parent.parent / "contracts"
SCHEMA_PATH = CONTRACTS_ROOT / "jsonschema" / "fit-trade-v1.schema.json"
CONFIRMATION_FIELDS_PATH = CONTRACTS_ROOT / "confirmation-fields.json"
FIXTURES_VALID_DIR = CONTRACTS_ROOT / "fixtures" / "valid"
FIXTURES_INVALID_DIR = CONTRACTS_ROOT / "fixtures" / "invalid"
GOLDEN_DIR = CONTRACTS_ROOT / "fixtures" / "golden"
MATRIX_PATH = CONTRACTS_ROOT / "fixtures" / "matrix" / "domain-values.json"


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


def load_matrix_values() -> dict[str, dict[str, Any]]:
    """Load the frozen domain-values matrix. Returns {schema_name: value, ...}."""
    return _load_json(MATRIX_PATH)


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
# JSON Schema validation (with referencing Registry and format checking)
# ---------------------------------------------------------------------------

# Custom date-time format checker matching project UTC-only rules:
# YYYY-MM-DD[ Tt]HH:MM:SS[.f+][Zz] — rejects no-zone and numeric offsets.
# Leap second 23:59:60Z is valid.
_DATETIME_UTC_RE = re.compile(
    r"^\d{4}-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12]\d|3[01])"
    r"[Tt ](?:[01]\d|2[0-3]):[0-5]\d:(?:[0-5]\d|60)"
    r"(?:\.\d+)?[Zz]$",
)


def _check_datetime_utc(instance: object) -> bool:
    """Reject date-time values that lack terminal Z/z or use numeric offsets.

    Raises ValueError on invalid input; returns True for valid UTC date-times.
    """
    if not isinstance(instance, str):
        raise ValueError("date-time must be a string")
    m = _DATETIME_UTC_RE.match(instance)
    if not m:
        raise ValueError(
            f"date-time must use terminal Z/z (UTC only): {instance!r}"
        )
    # Validate the calendar date
    try:
        date_part = instance[:10]
        _datetime.date.fromisoformat(date_part)
    except ValueError as exc:
        raise ValueError(f"date-time date invalid: {instance!r}") from exc
    # Leap second 60 is only valid at 23:59:60
    time_part = instance[11:]
    second_str = time_part[6:8]
    if second_str == "60" and time_part[:5] != "23:59":
        raise ValueError(
            f"leap second 60 only valid at 23:59:60: {instance!r}"
        )
    return True


# Custom format checker: merge stock checkers with our date-time checker
_CUSTOM_FORMAT_CHECKER = FormatChecker(())
_CUSTOM_FORMAT_CHECKER.checkers = (
    Draft202012Validator.FORMAT_CHECKER.checkers
    | {"date-time": (_check_datetime_utc, ValueError)}
)


def _build_jsonschema_validators() -> dict[str, Draft202012Validator]:
    """Build per-$def Draft202012Validator instances with referencing Registry.

    Uses referencing.Registry with the full frozen schema resource so that
    $ref: '#/$defs/...' resolves correctly within each sub-schema.  Full
    format checking (uuid, custom UTC date-time, etc.) is active.
    """
    schema = _load_json(SCHEMA_PATH)
    schema_id: str = schema.get("$id", "")

    # Build a referencing Registry containing the full schema as a resource
    resource = Resource.from_contents(
        schema, default_specification=_REF_DRAFT202012,
    )
    registry: Registry = Registry().with_resource(
        uri=schema_id, resource=resource,
    )

    validators: dict[str, Draft202012Validator] = {}
    for name in schema["$defs"]:
        # Compile each $def via an absolute $ref so that cross-$def
        # references and local pointers resolve through the Registry.
        validators[name] = Draft202012Validator(
            {"$ref": f"{schema_id}#/$defs/{name}"},
            registry=registry,
            format_checker=_CUSTOM_FORMAT_CHECKER,
        )
    return validators


# Cache compiled validators
_json_schema_validators: dict[str, Draft202012Validator] | None = None


def _get_jsonschema_validators() -> dict[str, Draft202012Validator]:
    global _json_schema_validators
    if _json_schema_validators is None:
        _json_schema_validators = _build_jsonschema_validators()
    return _json_schema_validators


def validate_with_jsonschema(schema_name: str, value: dict[str, Any]) -> None:
    """Validate a domain value using the raw JSON Schema with format checking.

    Resolves $ref correctly via referencing Registry.
    Raises jsonschema.ValidationError on failure.
    """
    validators = _get_jsonschema_validators()
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
        ignored_fields = []
        for field in fields:
            import copy
            mutated = copy.deepcopy(binding)
            mutated[field] = _mutate_value(mutated[field])
            import hashlib
            mutated_hash = hashlib.sha256(
                confirmation.canonicalize(mutated).encode("utf-8")
            ).hexdigest()
            if mutated_hash == golden:
                ignored_fields.append(field)
        if not ignored_fields:
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
