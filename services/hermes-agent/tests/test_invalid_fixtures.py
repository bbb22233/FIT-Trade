"""Test all invalid fixtures are rejected by both Pydantic and JSON Schema."""
import pytest
from hermes_agent.contracts import DOMAIN_TYPES
from hermes_agent.validator import (
    load_invalid_fixtures,
    validate_with_pydantic,
    validate_with_jsonschema,
)


@pytest.mark.parametrize(
    "schema_name,value,reason,path",
    load_invalid_fixtures(),
    ids=lambda x: f"{x[3].name}: {x[2]}" if isinstance(x, tuple) else str(x),
)
def test_invalid_fixture_rejected_pydantic(schema_name, value, reason, path):
    """Every invalid fixture must be rejected by Pydantic validation."""
    model = DOMAIN_TYPES.get(schema_name)
    assert model is not None, f"Unknown schema: {schema_name}"
    with pytest.raises(Exception):
        validate_with_pydantic(schema_name, value)


@pytest.mark.parametrize(
    "schema_name,value,reason,path",
    load_invalid_fixtures(),
    ids=lambda x: f"{x[3].name}: {x[2]}" if isinstance(x, tuple) else str(x),
)
def test_invalid_fixture_rejected_jsonschema(schema_name, value, reason, path):
    """Every invalid fixture must also be rejected by JSON Schema validation."""
    with pytest.raises(Exception):
        validate_with_jsonschema(schema_name, value)
