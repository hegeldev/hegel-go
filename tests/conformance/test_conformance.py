from pathlib import Path

import pytest

from hegel.conformance import run_conformance_test

BUILD_DIR = Path(__file__).parent / "go" / "bin"


def test_booleans():
    run_conformance_test("booleans", BUILD_DIR / "test_booleans")


def test_integers():
    run_conformance_test("integers", BUILD_DIR / "test_integers")


def test_floats():
    run_conformance_test("floats", BUILD_DIR / "test_floats")


@pytest.mark.skip(
    reason="Disabled due to hypothesis-jsonschema bug with Unicode surrogate pairs"
)
def test_text():
    run_conformance_test("text", BUILD_DIR / "test_text")


def test_lists():
    run_conformance_test("lists", BUILD_DIR / "test_lists")


def test_sampled_from():
    run_conformance_test("sampled_from", BUILD_DIR / "test_sampled_from")


def test_binary():
    run_conformance_test("binary", BUILD_DIR / "test_binary")
