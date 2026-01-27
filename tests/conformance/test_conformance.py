from pathlib import Path

from hegel.conformance import (
    BinaryConformance,
    BooleanConformance,
    FloatConformance,
    IntegerConformance,
    ListConformance,
    SampledFromConformance,
    TextConformance,
    run_conformance_tests,
)

BUILD_DIR = Path(__file__).parent / "go" / "bin"


def test_conformance(subtests):
    run_conformance_tests(
        [
            BooleanConformance(BUILD_DIR / "test_booleans"),
            IntegerConformance(BUILD_DIR / "test_integers"),
            FloatConformance(BUILD_DIR / "test_floats"),
            TextConformance(BUILD_DIR / "test_text"),
            BinaryConformance(BUILD_DIR / "test_binary"),
            ListConformance(BUILD_DIR / "test_lists"),
            SampledFromConformance(BUILD_DIR / "test_sampled_from"),
        ],
        subtests,
    )
