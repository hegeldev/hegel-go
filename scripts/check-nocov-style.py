#!/usr/bin/env python3
"""Check that //nocov annotations follow style rules.

1. No 3+ consecutive inline //nocov lines (use //nocov start/end blocks).
2. No inline //nocov adjacent to a //nocov start or //nocov end marker
   (expand the block instead).
3. No //nocov end immediately followed by //nocov start (merge the blocks).
4. //nocov start and //nocov end must be on their own lines (no code).
5. No inline //nocov inside a //nocov start/end block (redundant).
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

nocov_inline_re = re.compile(r"//\s*nocov\b")
nocov_start_re = re.compile(r"//\s*nocov\s+start\b")
nocov_end_re = re.compile(r"//\s*nocov\s+end\b")
nocov_block_re = re.compile(r"//\s*nocov\s+(start|end)\b")
nocov_block_own_line_re = re.compile(r"^\s*//\s*nocov\s+(start|end)\b\s*$")


def is_inline_nocov(line: str) -> bool:
    return bool(nocov_inline_re.search(line)) and not bool(nocov_block_re.search(line))


def find_go_source_files() -> list[Path]:
    """Find all non-test Go source files."""
    files = []
    for p in Path(".").rglob("*.go"):
        if p.name.endswith("_test.go"):
            continue
        if "vendor" in p.parts:
            continue
        files.append(p)
    return sorted(files)


def check() -> int:
    violations: list[str] = []

    for go_file in find_go_source_files():
        try:
            lines = go_file.read_text().splitlines()
        except (OSError, IOError):
            continue

        in_block = False
        run_start = -1
        run_length = 0

        for i, line in enumerate(lines):
            lineno = i + 1

            if nocov_start_re.search(line):
                # Rule 4: start must be on its own line
                if not nocov_block_own_line_re.match(line):
                    violations.append(
                        f"  {go_file}:{lineno}: //nocov start must be on its own line"
                    )
                # Rule 2: inline nocov right before this start
                if i > 0 and is_inline_nocov(lines[i - 1]):
                    violations.append(
                        f"  {go_file}:{lineno - 1}: inline //nocov adjacent to //nocov start (expand the block)"
                    )
                # Rule 1: run ending at a block boundary
                if run_length >= 3:
                    violations.append(
                        f"  {go_file}:{run_start}: {run_length} consecutive inline //nocov (use a block)"
                    )
                run_length = 0
                in_block = True
                continue

            if nocov_end_re.search(line):
                # Rule 4: end must be on its own line
                if not nocov_block_own_line_re.match(line):
                    violations.append(
                        f"  {go_file}:{lineno}: //nocov end must be on its own line"
                    )
                in_block = False
                # Rule 2: inline nocov right after this end
                if i + 1 < len(lines) and is_inline_nocov(lines[i + 1]):
                    violations.append(
                        f"  {go_file}:{lineno + 1}: inline //nocov adjacent to //nocov end (expand the block)"
                    )
                # Rule 3: start right after this end (merge the blocks)
                if i + 1 < len(lines) and nocov_start_re.search(lines[i + 1]):
                    violations.append(
                        f"  {go_file}:{lineno}: //nocov end immediately followed by //nocov start (merge the blocks)"
                    )
                continue

            if in_block:
                # Rule 5: inline nocov inside a block is redundant
                if is_inline_nocov(line):
                    violations.append(
                        f"  {go_file}:{lineno}: inline //nocov inside a //nocov start/end block (redundant)"
                    )
                continue

            # Track consecutive inline nocov runs
            if is_inline_nocov(line):
                if run_length == 0:
                    run_start = lineno
                run_length += 1
            else:
                # Rule 1: 3+ consecutive inline nocov
                if run_length >= 3:
                    violations.append(
                        f"  {go_file}:{run_start}: {run_length} consecutive inline //nocov (use a block)"
                    )
                run_length = 0

        # Check trailing run
        if run_length >= 3:
            violations.append(
                f"  {go_file}:{run_start}: {run_length} consecutive inline //nocov (use a block)"
            )

    if violations:
        print("nocov style violations:")
        for v in violations:
            print(v)
        print(f"\n{len(violations)} violation(s) found")
        return 1

    print("nocov style OK")
    return 0


if __name__ == "__main__":
    sys.exit(check())
