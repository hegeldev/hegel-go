from pathlib import Path

from hegel.conformance import (
    BinaryConformance,
    BooleanConformance,
    DictConformance,
    FloatConformance,
    IntegerConformance,
    ListConformance,
    SampledFromConformance,
    TextConformance,
    run_conformance_tests,
)

BUILD_DIR = Path(__file__).parent / "go" / "bin"

INT64_MIN = -(2**63)
INT64_MAX = 2**63 - 1


def test_conformance(subtests):
    run_conformance_tests(
        [
            BooleanConformance(BUILD_DIR / "test_booleans"),
            IntegerConformance(
                BUILD_DIR / "test_integers", min_value=INT64_MIN, max_value=INT64_MAX
            ),
            FloatConformance(BUILD_DIR / "test_floats"),
            TextConformance(BUILD_DIR / "test_text"),
            BinaryConformance(BUILD_DIR / "test_binary"),
            ListConformance(
                BUILD_DIR / "test_lists", min_value=INT64_MIN, max_value=INT64_MAX
            ),
            SampledFromConformance(BUILD_DIR / "test_sampled_from"),
            DictConformance(
                BUILD_DIR / "test_hashmaps",
                min_key=INT64_MIN,
                max_key=INT64_MAX,
                min_value=INT64_MIN,
                max_value=INT64_MAX,
            ),
        ],
        subtests,
    )
