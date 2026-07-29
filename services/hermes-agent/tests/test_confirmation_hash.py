"""Test RFC 8785 confirmation hash binding against the golden vector.

Includes official rfc8785==0.1.4 edge-vector tests for ECMAScript number
formatting, UTF-16 key ordering, and non-finite rejection.
"""
import hashlib
import json
from pathlib import Path

import pytest

from rfc8785 import CanonicalizationError, FloatDomainError, IntegerDomainError

from hermes_agent.confirmation import (
    canonicalize,
    confirmation_binding,
    compute_confirmation_hash,
    verify_confirmation_hash,
)
from hermes_agent.validator import load_confirmation_fields, load_golden_hash, FIXTURES_VALID_DIR

FIELDS_PATH = Path(__file__).parent.parent.parent.parent / "contracts" / "confirmation-fields.json"


# ---------------------------------------------------------------------------
# Golden hash parity
# ---------------------------------------------------------------------------


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


# ---------------------------------------------------------------------------
# Basic canonicalize smoke tests
# ---------------------------------------------------------------------------


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
    with pytest.raises((CanonicalizationError, ValueError)):
        canonicalize(float("nan"))
    with pytest.raises((CanonicalizationError, ValueError)):
        canonicalize(float("inf"))


# ---------------------------------------------------------------------------
# RFC 8785 edge vectors — ECMAScript number formatting
# ---------------------------------------------------------------------------


def test_rfc8785_float_trailing_zero_stripped():
    """1.0 must canonicalize as '1' (no trailing zero, no decimal point)."""
    assert canonicalize(1.0) == "1"


def test_rfc8785_float_zero():
    """0.0 must canonicalize as '0'."""
    assert canonicalize(0.0) == "0"


def test_rfc8785_float_fractional_preserved():
    """1.5 must canonicalize as '1.5' (fractional part preserved)."""
    assert canonicalize(1.5) == "1.5"


def test_rfc8785_negative_float_trailing_stripped():
    """-3.0 must canonicalize as '-3'."""
    assert canonicalize(-3.0) == "-3"


def test_rfc8785_large_float_no_exponential():
    """Large floats must not use scientific notation; must be fixed-point per JCS."""
    result = canonicalize(1000000.0)
    assert result == "1000000"
    assert "e" not in result and "E" not in result


def test_rfc8785_negative_inf_rejected():
    """Negative infinity must be rejected (non-finite number)."""
    with pytest.raises((CanonicalizationError, FloatDomainError, ValueError)):
        canonicalize(float("-inf"))


def test_rfc8785_nan_rejected():
    """NaN must be rejected (non-finite number)."""
    with pytest.raises((CanonicalizationError, FloatDomainError, ValueError)):
        canonicalize(float("nan"))


def test_rfc8785_inf_rejected():
    """Infinity must be rejected (non-finite number)."""
    with pytest.raises((CanonicalizationError, FloatDomainError, ValueError)):
        canonicalize(float("inf"))


# ---------------------------------------------------------------------------
# RFC 8785 edge vectors — UTF-16 key ordering
# ---------------------------------------------------------------------------


def test_rfc8785_unicode_key_ordering():
    """Keys containing non-ASCII characters must sort per UTF-16 code unit order."""
    obj = {"z": 1, "\u00e9": 2, "a": 3}
    # \u00e9 (é, U+00E9) has code unit 0x00E9, which is > 'a' (0x0061) but < 'z' (0x007A)
    result = canonicalize(obj)
    assert result == '{"a":3,"z":1,"\u00e9":2}'


def test_rfc8785_supplementary_multilingual_key_ordering():
    """Supplementary plane keys sort by UTF-16 surrogate pair order."""
    # U+1F600 (😀) encodes as surrogate pair D83D DE00 (0xD83D, 0xDE00)
    obj = {"\U0001F600": 1, "a": 2}
    # Surrogate pair starts at 0xD83D > 0x0061, so 'a' comes first
    result = canonicalize(obj)
    assert result.startswith('{"a":2')
    assert "\U0001F600" in result


# ---------------------------------------------------------------------------
# RFC 8785 edge vectors — canonicalize return type
# ---------------------------------------------------------------------------


def test_canonicalize_returns_str():
    """canonicalize must always return str (not bytes)."""
    assert isinstance(canonicalize({"a": 1}), str)
    assert isinstance(canonicalize([1, 2, 3]), str)
    assert isinstance(canonicalize(None), str)
    assert isinstance(canonicalize("hello"), str)


# ---------------------------------------------------------------------------
# Helper
# ---------------------------------------------------------------------------


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
