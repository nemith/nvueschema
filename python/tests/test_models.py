import importlib

import pytest
from pydantic import ValidationError

from nvueschema.models._versions import VERSIONS


@pytest.mark.parametrize("version", VERSIONS)
def test_models(version):
    module = importlib.import_module(f"nvueschema.models.v{version.replace('.', '_')}")
    model = module.NvueConfig.model_validate(
        {"interface": {"swp1": {"link": {"mtu": 1500}}}}
    )
    assert (
        model.model_dump(by_alias=True, exclude_none=True)["interface"]["swp1"]["link"][
            "mtu"
        ]
        == 1500
    )
    module.NvueConfig.model_json_schema()
    with pytest.raises(ValidationError):
        module.NvueConfig.model_validate({"interface": {"swp1": {"link": {"mtu": -1}}}})
