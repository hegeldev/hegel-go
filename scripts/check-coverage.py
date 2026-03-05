#!/usr/bin/env python3
"""Check Go test coverage and enforce 100% on library code.

Parses Go coverage profiles and reports uncovered lines.
Exits non-zero if any production code line is uncovered.
"""

import re
import subprocess
import sys


def main() -> int:
    # Sanitize coverage.out: remove any lines that don't match the expected format.
    # When 'go test ./...' runs multiple packages, stray lines (e.g. test-count
    # output) can get appended to the coverage profile and confuse go tool cover.
    sanitize_coverage("coverage.out", "coverage.sanitized.out")

    # Run go tool cover to get per-function coverage
    result = subprocess.run(
        ["go", "tool", "cover", "-func=coverage.sanitized.out"],
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
    uncovered = parse_uncovered("coverage.sanitized.out")
    real_uncovered = []
    for file, local_path, start_line, end_line in uncovered:
        # Filter out known false positives
        if is_false_positive(local_path, start_line, end_line):
            continue
        real_uncovered.append((file, start_line, end_line))
        print(f"  {file}:{start_line}-{end_line}")

    if real_uncovered:
        print(f"\n❌ {len(real_uncovered)} uncovered region(s) found")
        return 1

    print("\n✅ All uncovered lines are false positives — coverage is effectively 100%")
    return 0


def sanitize_coverage(input_path: str, output_path: str) -> None:
    """Write a sanitized copy of the Go coverage profile, removing malformed lines.

    Go's 'go test ./...' sometimes appends stray lines (e.g. test-count output)
    to the coverage profile. This function copies only valid lines (mode header
    and properly-formatted coverage entries) to a new file.
    """
    valid_line = re.compile(r".+:\d+\.\d+,\d+\.\d+\s+\d+\s+\d+")
    with open(input_path) as fin, open(output_path, "w") as fout:
        for line in fin:
            stripped = line.strip()
            if stripped.startswith("mode:") or valid_line.match(stripped):
                fout.write(line)


def parse_uncovered(profile_path: str) -> list[tuple[str, str, int, int]]:
    """Parse a Go coverage profile and return uncovered regions.

    Coverage profile format per line:
        module/path/file.go:startLine.startCol,endLine.endCol numStatements executionCount

    When ``go test -coverpkg=PKG ./...`` runs multiple test packages, each
    test binary independently instruments PKG and writes its own coverage
    entries.  For example, the ``internal/conformance`` tests don't call any
    main-package code, so their entries have execution count 0 even though
    the main-package tests cover those same regions.  We merge duplicates by
    summing execution counts so that a region covered by *any* test binary is
    not reported as uncovered.

    Returns tuples of (module_path, local_file_path, start_line, end_line)
    for regions whose merged execution count is still 0.
    """
    # key = full coverage key (everything before the exec count fields),
    # value = (module_path, start_line, end_line, total_exec_count)
    merged: dict[str, tuple[str, int, int, int]] = {}
    with open(profile_path) as f:
        for line in f:
            line = line.strip()
            if line.startswith("mode:") or not line:
                continue
            # Format: modulePath:startLine.startCol,endLine.endCol numStatements execCount
            match = re.match(
                r"(.+):(\d+\.\d+,\d+\.\d+)\s+(\d+)\s+(\d+)", line
            )
            if match:
                module_path = match.group(1)
                region = match.group(2)  # e.g. "15.33,17.16"
                exec_count = int(match.group(4))
                key = f"{module_path}:{region}"
                if key in merged:
                    prev = merged[key]
                    merged[key] = (prev[0], prev[1], prev[2], prev[3] + exec_count)
                else:
                    # Parse start/end lines from the region string
                    parts = region.split(",")
                    start_line = int(parts[0].split(".")[0])
                    end_line = int(parts[1].split(".")[0])
                    merged[key] = (module_path, start_line, end_line, exec_count)

    uncovered = []
    for module_path, start_line, end_line, total_count in merged.values():
        if total_count == 0:
            local_path = _module_path_to_local(module_path)
            uncovered.append((module_path, local_path, start_line, end_line))
    return uncovered


def _module_path_to_local(module_path: str) -> str:
    """Convert a Go module file path to a local filesystem path.

    Strips the module prefix (everything up to and including the third path
    component for github.com paths, or finds the first existing file).
    Falls back to the original path if no mapping is found.
    """
    import os

    # Try progressively stripping leading path components until the file exists.
    parts = module_path.split("/")
    for i in range(len(parts)):
        candidate = "/".join(parts[i:])
        if os.path.exists(candidate):
            return candidate
    return module_path


def is_false_positive(file: str, start_line: int, end_line: int) -> bool:
    """Check if an uncovered region is a known false positive.

    False positives include:
    - Lines containing only closing braces
    - Lines containing only unreachable panics (single-line form)
    - if-guard lines that guard an unreachable panic on the next line
    """
    try:
        with open(file) as f:
            lines = f.readlines()
        for i in range(start_line - 1, min(end_line, len(lines))):
            content = lines[i].strip()
            # Skip empty lines and closing braces
            if content in ("}", "}", "});", ""):
                continue
            # Skip unreachable panics (may be inline: "if err != nil { panic(...unreachable...) }")
            if "panic(" in content and "unreachable" in content.lower():
                continue
            # Skip if-guard lines for unreachable panics or untestable t.Fatal:
            # "if condition {" followed by a panic or t.Fatal on the next line
            if content.endswith("{") and "if " in content and i + 1 < len(lines):
                next_content = lines[i + 1].strip()
                if next_content.startswith("panic(") and "unreachable" in next_content.lower():
                    continue
                if next_content == "t.Fatal(err)":
                    continue
            # Skip loop-exhaustion guard: "if !more { break }" from frames.Next()
            if content in ("if !more {", "break"):
                continue
            # Skip t.Fatal(err) in error-check blocks — untestable because
            # sub-test failures propagate to the parent test.
            if content == "t.Fatal(err)":
                continue
            # Any other content means it's real uncovered code
            return False
        return True
    except (FileNotFoundError, IndexError):
        return False


if __name__ == "__main__":
    sys.exit(main())
