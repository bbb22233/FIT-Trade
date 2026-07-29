"""Test all valid fixtures validate successfully via both Pydantic and JSON Schema."""
import pytest
from hermes_agent.contracts import DOMAIN_TYPES
from hermes_agent.validator import (
    load_valid_fixtures,
    validate_with_pydantic,
    validate_with_jsonschema,
)


@pytest.mark.parametrize(
    "schema_name,value,path",
    load_valid_fixtures(),
    ids=lambda x: x[2].name if isinstance(x, tuple) else str(x),
)
def test_valid_fixture_validates_pydantic(schema_name, value, path):
    """Every valid fixture must pass Pydantic validation."""
    result = validate_with_pydantic(schema_name, value)
    assert result is not None
    # Round-trip: export and re-validate
    exported = result.model_dump()
    revalidated = validate_with_pydantic(schema_name, exported)
    assert revalidated.model_dump() == exported


@pytest.mark.parametrize(
    "schema_name,value,path",
    load_valid_fixtures(),
    ids=lambda x: x[2].name if isinstance(x, tuple) else str(x),
)
def test_valid_fixture_validates_jsonschema(schema_name, value, path):
    """Every valid fixture must also pass JSON Schema validation."""
    validate_with_jsonschema(schema_name, value)  # should not raise
