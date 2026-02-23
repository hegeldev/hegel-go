#!/usr/bin/env python3
"""Check Go test coverage and enforce 100% on library code.

Parses Go coverage profiles and reports uncovered lines.
Exits non-zero if any production code line is uncovered.
"""

import re
import subprocess
import sys


def main() -> int:
    # Run go tool cover to get per-function coverage
    result = subprocess.run(
        ["go", "tool", "cover", "-func=coverage.out"],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        print(f"Error running go tool cover: {result.stderr}", file=sys.stderr)
        return 1

    print(result.stdout)

    # Check total coverage
    for line in result.stdout.strip().splitlines():
        if "total:" in line:
            match = re.search(r"(\d+\.\d+)%", line)
            if match:
                coverage = float(match.group(1))
                if coverage == 100.0:
                    print(f"\n✅ Coverage: {coverage}%")
                    return 0
                else:
                    print(f"\n❌ Coverage: {coverage}% (required: 100%)")

    # If we didn't find 100%, parse the coverage profile for uncovered lines
    print("\nUncovered lines:")
    uncovered = parse_uncovered("coverage.out")
    real_uncovered = []
    for file, start_line, end_line, count in uncovered:
        if count == 0:
            # Filter out known false positives
            if is_false_positive(file, start_line, end_line):
                continue
            real_uncovered.append((file, start_line, end_line))
            print(f"  {file}:{start_line}-{end_line}")

    if real_uncovered:
        print(f"\n❌ {len(real_uncovered)} uncovered region(s) found")
        return 1

    print("\n✅ All uncovered lines are false positives — coverage is effectively 100%")
    return 0


def parse_uncovered(profile_path: str) -> list[tuple[str, int, int, int]]:
    """Parse a Go coverage profile and return uncovered regions."""
    uncovered = []
    with open(profile_path) as f:
        for line in f:
            line = line.strip()
            if line.startswith("mode:") or not line:
                continue
            # Format: file:startLine.startCol,endLine.endCol count statements
            match = re.match(
                r"(.+):(\d+)\.\d+,(\d+)\.\d+\s+(\d+)\s+\d+", line
            )
            if match:
                file = match.group(1)
                start_line = int(match.group(2))
                end_line = int(match.group(3))
                count = int(match.group(4))
                if count == 0:
                    uncovered.append((file, start_line, end_line, count))
    return uncovered


def is_false_positive(file: str, start_line: int, end_line: int) -> bool:
    """Check if an uncovered region is a known false positive.

    False positives include:
    - Lines containing only closing braces
    - Lines containing only unreachable panics
    """
    try:
        with open(file) as f:
            lines = f.readlines()
        for i in range(start_line - 1, min(end_line, len(lines))):
            content = lines[i].strip()
            # Skip empty lines and closing braces
            if content in ("}", "}", "});", ""):
                continue
            # Skip unreachable panics
            if content.startswith("panic(") and "unreachable" in content.lower():
                continue
            # Any other content means it's real uncovered code
            return False
        return True
    except (FileNotFoundError, IndexError):
        return False


if __name__ == "__main__":
    sys.exit(main())
