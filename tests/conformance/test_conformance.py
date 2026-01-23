from pathlib import Path

import pytest

from hegel.conformance import conformance_tests, run_conformance_test

BUILD_DIR = Path(__file__).parent / "go" / "bin"

TESTS = conformance_tests({
    "booleans": BUILD_DIR / "test_booleans",
    "integers": BUILD_DIR / "test_integers",
    "floats": BUILD_DIR / "test_floats",
    "text": None,  # skipped due to hypothesis-jsonschema bug with Unicode surrogate pairs
    "binary": BUILD_DIR / "test_binary",
    "lists": BUILD_DIR / "test_lists",
    "sampled_from": BUILD_DIR / "test_sampled_from",
})


@pytest.mark.parametrize("test_name,binary_path", TESTS, ids=[t[0] for t in TESTS])
def test_conformance(test_name, binary_path):
    run_conformance_test(test_name, binary_path)
