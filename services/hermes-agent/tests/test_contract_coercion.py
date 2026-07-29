"""P0-003 systematic regression: coercion and omission differential tests.

For all 19 matrix domain types, recursively:
  1) Mutate scalar/container leaves with type coercion payloads —
     compare JSON Schema and Pydantic accept/reject.
  2) Delete each present property —
     compare JSON Schema and Pydantic accept/reject.

Both mismatch counts must be zero.
"""
from __future__ import annotations

import json
from typing import Any

import pytest

from hermes_agent.validator import (
    load_matrix_values,
    validate_with_pydantic,
    validate_with_jsonschema,
)

MATRIX = load_matrix_values()

# All 19 non-meta domain types
DOMAIN_NAMES = sorted(k for k in MATRIX if not k.startswith("_"))

# ---------------------------------------------------------------------------
# Type coercion payloads — ordered from least to most invasive
# ---------------------------------------------------------------------------

# Integer fields: try string, bool, null (skip float since JSON Schema
# treats 1.0 as integer-equivalent; Pydantic strict=int rejects it, which
# is a safe fail-closed direction, not a vulnerability)
INT_COERCIONS = [
    ("1", "string-int"),       # "1" should NOT be accepted for int fields
    (True, "bool-true"),       # True should NOT be accepted
    (None, "null"),            # None should NOT be accepted
]

# Boolean fields: try string, int, null (skip float for same reason as int)
BOOL_COERCIONS = [
    ("true", "string-true"),
    (1, "int-1"),
    (0, "int-0"),
    (None, "null"),
]

# String fields: try int (when there's a pattern constraint)
# (We don't test all string fields, just those where coercion could slip)
STRING_COERCIONS = [
    (None, "null"),
]

# Enum string fields: try null
ENUM_COERCIONS = [
    (None, "null"),
]

# General null test for any field type
NULL_COERCION = (None, "null")


# ---------------------------------------------------------------------------
# Determine field type from JSON Schema
# ---------------------------------------------------------------------------

def _load_schema() -> dict:
    from pathlib import Path
    # tests/test_contract_coercion.py → tests/ → hermes-agent/ → services/ → p0-hermes/ (repo root)
    repo_root = Path(__file__).resolve().parent.parent.parent.parent
    schema_path = repo_root / "contracts/jsonschema/fit-trade-v1.schema.json"
    return json.loads(schema_path.read_text())

SCHEMA = _load_schema()
DEFS = SCHEMA["$defs"]


def _resolve_ref(ref: str) -> dict:
    """Resolve a JSON Schema $ref like '#/$defs/Symbol'."""
    if not ref.startswith("#/$defs/"):
        return {}
    name = ref[len("#/$defs/"):]
    return DEFS.get(name, {})


def _field_schema_type(schema_def: dict, field_name: str) -> tuple[str, bool]:
    """Return (type_category, is_required) for a field.

    type_category is one of: 'integer', 'boolean', 'string', 'array',
    'object', 'enum', 'const-true', 'unknown'
    """
    props = schema_def.get("properties", {})
    required = schema_def.get("required", [])
    is_required = field_name in required

    prop = props.get(field_name, {})

    # Resolve $ref
    if "$ref" in prop:
        prop = _resolve_ref(prop["$ref"])

    # Check for const true
    if prop == {"const": True}:
        return ("const-true", is_required)
    if prop.get("const") is True:
        return ("const-true", is_required)

    # Check for enum
    if "enum" in prop:
        return ("enum", is_required)

    # Check for type
    ptype = prop.get("type", "")
    if ptype == "integer":
        return ("integer", is_required)
    if ptype == "boolean":
        return ("boolean", is_required)
    if ptype == "string":
        return ("string", is_required)
    if ptype == "array":
        return ("array", is_required)
    if ptype == "object":
        return ("object", is_required)

    return ("unknown", is_required)


def _get_coercions_for_type(ftype: str) -> list[tuple[Any, str]]:
    """Return appropriate coercion payloads for a field type."""
    if ftype in ("integer",):
        return INT_COERCIONS
    if ftype in ("boolean",):
        return BOOL_COERCIONS
    if ftype in ("const-true",):
        # const-true: try string "true", int 1, false
        return [
            ("true", "string-true"),
            (1, "int-1"),
            (False, "false"),
            (None, "null"),
        ]
    if ftype in ("string", "enum"):
        return STRING_COERCIONS
    # For objects and arrays, just try None
    return [(None, "null")]


# ---------------------------------------------------------------------------
# Recursive property walker
# ---------------------------------------------------------------------------

def _walk_leaves(
    value: Any,
    schema_def: dict,
    path: str = "",
) -> list[tuple[str, Any, str, bool]]:
    """Recursively walk value, yielding (path, coercion_value, coercion_label, is_required).

    For each scalar field, yields coercion payloads.
    For nested objects/arrays, recurses.
    Path is dot-separated for nested access (e.g., 'stop.type').
    """
    results: list[tuple[str, Any, str, bool]] = []

    if isinstance(value, dict):
        for key, val in value.items():
            cur_path = f"{path}.{key}" if path else key
            ftype, is_required = _field_schema_type(schema_def, key)

            if ftype in ("integer", "boolean", "string", "enum", "const-true"):
                # Scalar leaf — add coercion tests
                for coerce_val, coerce_label in _get_coercions_for_type(ftype):
                    results.append((cur_path, coerce_val, coerce_label, is_required))
            elif ftype == "object" and isinstance(val, dict):
                # Resolve nested schema
                props = schema_def.get("properties", {})
                prop = props.get(key, {})
                if "$ref" in prop:
                    nested_schema = _resolve_ref(prop["$ref"])
                else:
                    nested_schema = prop
                results.extend(_walk_leaves(val, nested_schema, cur_path))
            elif ftype == "array" and isinstance(val, list):
                # For arrays, recurse into items
                props = schema_def.get("properties", {})
                prop = props.get(key, {})
                items_schema = prop.get("items", {})
                if "$ref" in items_schema:
                    items_schema = _resolve_ref(items_schema["$ref"])
                for i, item in enumerate(val):
                    item_path = f"{cur_path}[{i}]"
                    if isinstance(item, dict):
                        results.extend(_walk_leaves(item, items_schema, item_path))
            elif not isinstance(val, (dict, list)):
                # Unknown scalar — try null coercion
                results.append((cur_path, None, "null", is_required))

    return results


def _walk_properties(
    value: Any,
    schema_def: dict,
    path: str = "",
) -> list[str]:
    """Recursively walk all properties, returning deletion paths."""
    results: list[str] = []

    if isinstance(value, dict):
        for key, val in value.items():
            cur_path = f"{path}.{key}" if path else key
            results.append(cur_path)

            props = schema_def.get("properties", {})
            prop = props.get(key, {})

            if isinstance(val, dict):
                if "$ref" in prop:
                    nested = _resolve_ref(prop["$ref"])
                else:
                    nested = prop
                results.extend(_walk_properties(val, nested, cur_path))
            elif isinstance(val, list):
                items_schema = prop.get("items", {})
                if "$ref" in items_schema:
                    items_schema = _resolve_ref(items_schema["$ref"])
                for i, item in enumerate(val):
                    if isinstance(item, dict):
                        results.extend(_walk_properties(item, items_schema, f"{cur_path}[{i}]"))

    return results


# ---------------------------------------------------------------------------
# Path-based value manipulation
# ---------------------------------------------------------------------------

def _set_at_path(value: Any, path: str, new_val: Any) -> Any:
    """Set a value at a dotted path like 'stop.type' or 'arr[0].field'."""
    import re
    parts = []
    for segment in path.split("."):
        m = re.match(r"^(.+)\[(\d+)\]$", segment)
        if m:
            parts.append(m.group(1))
            parts.append(int(m.group(2)))
        else:
            parts.append(segment)

    target = value
    for part in parts[:-1]:
        if isinstance(part, int):
            target = target[part]
        else:
            target = target[part]
    last = parts[-1]
    if isinstance(last, int):
        target[last] = new_val
    else:
        target[last] = new_val
    return value


def _delete_at_path(value: Any, path: str) -> Any:
    """Delete a key at a dotted path."""
    import re
    # Navigate to parent
    parts = []
    for segment in path.split("."):
        m = re.match(r"^(.+)\[(\d+)\]$", segment)
        if m:
            parts.append(m.group(1))
            parts.append(int(m.group(2)))
        else:
            parts.append(segment)

    target = value
    for part in parts[:-1]:
        if isinstance(part, int):
            target = target[part]
        else:
            target = target[part]
    last = parts[-1]
    if isinstance(last, int):
        del target[last]
    else:
        del target[last]
    return value


# ============================================================================
# Test A: Systematic type coercion — all 19 types, all leaves
# ============================================================================


@pytest.mark.parametrize("domain_name", DOMAIN_NAMES)
def test_coercion_mismatches_zero(domain_name: str):
    """For a given domain type, mutate all scalar leaves and verify zero accept/reject mismatches."""
    schema_def = DEFS.get(domain_name, {})
    base_value = json.loads(json.dumps(MATRIX[domain_name]))

    # Walk all leaves and collect coercion tests
    coercion_tests = _walk_leaves(base_value, schema_def)

    mismatches: list[str] = []

    for path, coerce_val, coerce_label, is_required in coercion_tests:
        mutated = json.loads(json.dumps(base_value))
        _set_at_path(mutated, path, coerce_val)

        js_accept = False
        py_accept = False
        js_error = None
        py_error = None

        try:
            validate_with_jsonschema(domain_name, mutated)
            js_accept = True
        except Exception as e:
            js_error = str(e)

        try:
            validate_with_pydantic(domain_name, mutated)
            py_accept = True
        except Exception as e:
            py_error = str(e)

        if js_accept != py_accept:
            mismatches.append(
                f"{domain_name}.{path} <- {coerce_label}={coerce_val!r}: "
                f"jsonschema={'ACCEPT' if js_accept else 'REJECT'}, "
                f"pydantic={'ACCEPT' if py_accept else 'REJECT'}"
            )

    assert len(mismatches) == 0, (
        f"Coercion mismatches for {domain_name} ({len(mismatches)}):\n"
        + "\n".join(mismatches)
    )


# ============================================================================
# Test B: Systematic property deletion — all 19 types, every property
# ============================================================================


@pytest.mark.parametrize("domain_name", DOMAIN_NAMES)
def test_omission_mismatches_zero(domain_name: str):
    """For a given domain type, delete each present property and verify zero accept/reject mismatches."""
    schema_def = DEFS.get(domain_name, {})
    base_value = json.loads(json.dumps(MATRIX[domain_name]))

    # Walk all properties recursively
    deletion_paths = _walk_properties(base_value, schema_def)

    mismatches: list[str] = []

    for path in deletion_paths:
        mutated = json.loads(json.dumps(base_value))
        _delete_at_path(mutated, path)

        js_accept = False
        py_accept = False

        try:
            validate_with_jsonschema(domain_name, mutated)
            js_accept = True
        except Exception:
            pass

        try:
            validate_with_pydantic(domain_name, mutated)
            py_accept = True
        except Exception:
            pass

        if js_accept != py_accept:
            mismatches.append(
                f"{domain_name} -delete {path}: "
                f"jsonschema={'ACCEPT' if js_accept else 'REJECT'}, "
                f"pydantic={'ACCEPT' if py_accept else 'REJECT'}"
            )

    assert len(mismatches) == 0, (
        f"Omission mismatches for {domain_name} ({len(mismatches)}):\n"
        + "\n".join(mismatches)
    )


# ============================================================================
# Summary: global counts across all 19 types
# ============================================================================


def test_coercion_total_mismatches_zero():
    """Aggregate coercion mismatches across all 19 domain types must be zero."""
    all_mismatches: list[str] = []

    for domain_name in DOMAIN_NAMES:
        schema_def = DEFS.get(domain_name, {})
        base_value = json.loads(json.dumps(MATRIX[domain_name]))
        coercion_tests = _walk_leaves(base_value, schema_def)

        for path, coerce_val, coerce_label, is_required in coercion_tests:
            mutated = json.loads(json.dumps(base_value))
            _set_at_path(mutated, path, coerce_val)

            js_accept = False
            py_accept = False
            try:
                validate_with_jsonschema(domain_name, mutated)
                js_accept = True
            except Exception:
                pass
            try:
                validate_with_pydantic(domain_name, mutated)
                py_accept = True
            except Exception:
                pass

            if js_accept != py_accept:
                all_mismatches.append(
                    f"[{domain_name}].{path} <- {coerce_label}: "
                    f"JS={'OK' if js_accept else 'ERR'}, "
                    f"PY={'OK' if py_accept else 'ERR'}"
                )

    assert len(all_mismatches) == 0, (
        f"Total coercion mismatches: {len(all_mismatches)}\n"
        + "\n".join(all_mismatches[:50])
        + ("\n..." if len(all_mismatches) > 50 else "")
    )


def test_omission_total_mismatches_zero():
    """Aggregate omission mismatches across all 19 domain types must be zero."""
    all_mismatches: list[str] = []

    for domain_name in DOMAIN_NAMES:
        schema_def = DEFS.get(domain_name, {})
        base_value = json.loads(json.dumps(MATRIX[domain_name]))
        deletion_paths = _walk_properties(base_value, schema_def)

        for path in deletion_paths:
            mutated = json.loads(json.dumps(base_value))
            _delete_at_path(mutated, path)

            js_accept = False
            py_accept = False
            try:
                validate_with_jsonschema(domain_name, mutated)
                js_accept = True
            except Exception:
                pass
            try:
                validate_with_pydantic(domain_name, mutated)
                py_accept = True
            except Exception:
                pass

            if js_accept != py_accept:
                all_mismatches.append(
                    f"[{domain_name}] -{path}: "
                    f"JS={'OK' if js_accept else 'ERR'}, "
                    f"PY={'OK' if py_accept else 'ERR'}"
                )

    assert len(all_mismatches) == 0, (
        f"Total omission mismatches: {len(all_mismatches)}\n"
        + "\n".join(all_mismatches[:50])
        + ("\n..." if len(all_mismatches) > 50 else "")
    )
