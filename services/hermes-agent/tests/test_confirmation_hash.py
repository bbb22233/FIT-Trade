"""Test RFC 8785 confirmation hash binding against the golden vector."""
import json
import hashlib
from pathlib import Path

import pytest

from hermes_agent.confirmation import (
    canonicalize,
    confirmation_binding,
    compute_confirmation_hash,
    verify_confirmation_hash,
)
from hermes_agent.validator import load_confirmation_fields, load_golden_hash, FIXTURES_VALID_DIR

FIELDS_PATH = Path(__file__).parent.parent.parent.parent / "contracts" / "confirmation-fields.json"


def test_golden_hash_matches():
    """The computed confirmation hash must match the frozen golden vector."""
    ticket_path = FIXTURES_VALID_DIR / "confirmation-ticket-open.json"
    ticket = json.loads(ticket_path.read_text())["value"]
    fields = load_confirmation_fields()
    computed = compute_confirmation_hash(ticket, fields)
    golden = load_golden_hash()
    assert computed == golden, (
        f"Golden mismatch: computed={computed}, expected={golden}"
    )
    # Also verify the ticket's embedded hash matches the golden
    assert ticket["confirmation_hash"] == golden


def test_golden_hash_key_order_independent():
    """Key insertion order must not affect the digest (RFC 8785 sorts keys)."""
    ticket_path = FIXTURES_VALID_DIR / "confirmation-ticket-open.json"
    ticket = json.loads(ticket_path.read_text())["value"]
    fields = load_confirmation_fields()
    binding = confirmation_binding(ticket, fields)
    canonical_forward = canonicalize(binding)
    reversed_binding = dict(reversed(list(binding.items())))
    canonical_reversed = canonicalize(reversed_binding)
    assert canonical_forward == canonical_reversed, (
        "Key order must not affect RFC 8785 canonical output"
    )
    digest_forward = hashlib.sha256(canonical_forward.encode("utf-8")).hexdigest()
    digest_reversed = hashlib.sha256(canonical_reversed.encode("utf-8")).hexdigest()
    assert digest_forward == digest_reversed


def test_confirmation_hash_field_sensitivity():
    """Every bound field mutation must change the digest."""
    ticket_path = FIXTURES_VALID_DIR / "confirmation-ticket-open.json"
    ticket = json.loads(ticket_path.read_text())["value"]
    fields = load_confirmation_fields()
    golden = load_golden_hash()
    binding = confirmation_binding(ticket, fields)

    for field in fields:
        mutated = json.loads(json.dumps(binding))  # deep copy
        mutated[field] = _mutate_value(mutated[field])
        mutated_canonical = canonicalize(mutated)
        mutated_digest = hashlib.sha256(mutated_canonical.encode("utf-8")).hexdigest()
        assert mutated_digest != golden, (
            f"Confirmation digest ignored mutation of '{field}'"
        )


def test_confirmation_hash_excludes_confirmation_hash_field():
    """The confirmation_hash field must not be included in the binding input."""
    fields = load_confirmation_fields()
    assert "confirmation_hash" not in fields


def test_canonicalize_null():
    assert canonicalize(None) == "null"


def test_canonicalize_bool():
    assert canonicalize(True) == "true"
    assert canonicalize(False) == "false"


def test_canonicalize_int():
    assert canonicalize(0) == "0"
    assert canonicalize(42) == "42"
    assert canonicalize(-1) == "-1"


def test_canonicalize_string():
    assert canonicalize("hello") == '"hello"'
    assert canonicalize('a"b') == '"a\\"b"'


def test_canonicalize_array_empty():
    assert canonicalize([]) == "[]"


def test_canonicalize_array_items():
    assert canonicalize([1, "two", True]) == '[1,"two",true]'


def test_canonicalize_object_sorted_keys():
    obj = {"c": 1, "a": 2, "b": 3}
    result = canonicalize(obj)
    assert result == '{"a":2,"b":3,"c":1}'


def test_canonicalize_nested_object():
    obj = {"outer": {"inner": 1}}
    result = canonicalize(obj)
    assert result == '{"outer":{"inner":1}}'


def test_canonicalize_rejects_non_finite():
    with pytest.raises(ValueError):
        canonicalize(float("nan"))
    with pytest.raises(ValueError):
        canonicalize(float("inf"))


def _mutate_value(value):
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
