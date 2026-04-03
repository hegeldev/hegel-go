#!/usr/bin/env python3
"""Check Go test coverage using go-test-coverage and enforce a ratchet on coverage-ignore annotations.

Runs go-test-coverage for threshold enforcement, then counts // coverage-ignore
annotations and enforces a ratchet to prevent growth.
"""

import json
import os
import re
import subprocess
import sys
from glob import glob
RATCHET_FILE = ".github/coverage-ratchet.json"


def main() -> int:
    # Sanitize coverage.out: remove any lines that don't match the expected format.
    # When 'go test ./...' runs multiple packages, stray lines (e.g. test-count
    # output) can get appended to the coverage profile and confuse tools.
    sanitize_coverage("coverage.out", "coverage.sanitized.out")
    os.replace("coverage.sanitized.out", "coverage.out")

    # Run go-test-coverage for threshold enforcement.
    result = subprocess.run(
        ["go", "tool", "github.com/vladopajic/go-test-coverage/v2",
         "--config", ".testcoverage.yml"],
        capture_output=True,
        text=True,
    )
    print(result.stdout)
    if result.stderr:
        print(result.stderr, file=sys.stderr)
    if result.returncode != 0:
        return 1

    # Enforce ratchet on coverage-ignore annotation count.
    return check_ratchet()


def sanitize_coverage(input_path: str, output_path: str) -> None:
    """Write a sanitized copy of the Go coverage profile, removing malformed lines."""
    valid_line = re.compile(r".+:\d+\.\d+,\d+\.\d+\s+\d+\s+\d+")
    with open(input_path) as fin, open(output_path, "w") as fout:
        for line in fin:
            stripped = line.strip()
            if stripped.startswith("mode:") or valid_line.match(stripped):
                fout.write(line)


def count_coverage_ignore() -> int:
    """Count // coverage-ignore annotations in non-test .go files."""
    count = 0
    for path in glob("**/*.go", recursive=True):
        if path.endswith("_test.go"):
            continue
        # Skip cmd/, internal/conformance/, and examples/ directories.
        if any(path.startswith(d) for d in ("cmd/", "internal/conformance/", "examples/")):
            continue
        with open(path) as f:
            for line in f:
                if "// coverage-ignore" in line:
                    count += 1
    return count


def check_ratchet() -> int:
    """Enforce the coverage-ignore annotation ratchet."""
    current = count_coverage_ignore()
    print(f"Coverage-ignore annotations: {current}")

    if not os.path.exists(RATCHET_FILE):
        print(f"No ratchet file found at {RATCHET_FILE}, creating with current count.")
        save_ratchet(current)
        return 0

    with open(RATCHET_FILE) as f:
        data = json.load(f)
    limit = data.get("coverage_ignore_count", 0)

    if current > limit:
        print(f"\n\u274c Coverage-ignore count {current} exceeds ratchet limit {limit}.")
        print("Either remove unnecessary annotations or update the ratchet.")
        return 1

    if current < limit:
        print(f"\u2705 Ratchet tightened: {limit} \u2192 {current}")
        save_ratchet(current)

    return 0


def save_ratchet(count: int) -> None:
    """Save the ratchet count to the ratchet file."""
    os.makedirs(os.path.dirname(RATCHET_FILE), exist_ok=True)
    with open(RATCHET_FILE, "w") as f:
        json.dump({"coverage_ignore_count": count}, f, indent=2)
        f.write("\n")


if __name__ == "__main__":
    sys.exit(main())
