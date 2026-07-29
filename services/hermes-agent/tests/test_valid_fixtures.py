"""Test all valid fixtures validate successfully."""
import pytest
from hermes_agent.contracts import DOMAIN_TYPES
from hermes_agent.validator import load_valid_fixtures


@pytest.mark.parametrize(
    "schema_name,value,path",
    load_valid_fixtures(),
    ids=lambda x: x[2].name if isinstance(x, tuple) else str(x),
)
def test_valid_fixture_validates(schema_name, value, path):
    """Every valid fixture must pass Pydantic validation."""
    model = DOMAIN_TYPES[schema_name]
    result = model.model_validate(value)
    assert result is not None
    # Re-export to dict and validate round-trip
    exported = result.model_dump()
    revalidated = model.model_validate(exported)
    assert revalidated == result
