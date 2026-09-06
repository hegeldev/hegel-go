import os
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent.parent
RUST_REPO = "hegeldev/hegel-rust"
# One branch per pinned version. A fixed, reused branch meant every release
# force-recreated it from main, clobbering any manual or agent work pushed to
# the open PR; a per-version branch keeps re-runs of the *same* version
# idempotent (the force-push below) while never touching another version's
# work. The workflow closes superseded bot-only bump PRs after pushing.
BRANCH_PREFIX = "ci/bump-hegel-rust-"
VERSION_FILE = ROOT / "internal" / "libhegel" / "version.go"
LIBS_DIR = ROOT / "internal" / "libhegel" / "libs"
RELEASE_MD = ROOT / "RELEASE.md"


def git(*args: str) -> None:
    subprocess.run(["git", *args], check=True, cwd=ROOT)


def set_output(name: str, value: str) -> None:
    """Expose a step output to later workflow steps (no-op outside Actions)."""
    out = os.environ.get("GITHUB_OUTPUT")
    if not out:
        return
    with open(out, "a") as f:
        f.write(f"{name}={value}\n")


def get_pinned_version() -> str:
    text = VERSION_FILE.read_text()
    m = re.search(r'^const hegelVersion = "([^"]+)"', text, re.MULTILINE)
    assert m is not None, "could not find hegelVersion in version.go"
    return m.group(1)


def bump(requested: str) -> None:
    """Pin the new libhegel and leave a fully-formed commit on the local branch.

    The commit is intentionally *not* pushed: the workflow then runs the
    align-libhegel skill, amends the result into this commit, and pushes once.
    """
    current = get_pinned_version()

    # `just vendor-libhegel` downloads the target release's libhegel artifacts
    # into libs/ (empty requested -> latest) and rewrites version.go.
    # vendor-libhegel.go discovers the resolved version itself, so we read it
    # back rather than trusting the request (which may be empty for a manual
    # latest-bump).
    subprocess.run(["just", "vendor-libhegel", requested], check=True, cwd=ROOT)
    new = get_pinned_version()

    if new == current:
        print(f"Already pinned to v{current}; nothing to do.")
        set_output("bumped", "false")
        return

    current_url = f"https://github.com/{RUST_REPO}/releases/tag/v{current}"
    new_url = f"https://github.com/{RUST_REPO}/releases/tag/v{new}"

    RELEASE_MD.write_text(
        "RELEASE_TYPE: patch\n\n"
        f"This patch bumps our pinned libhegel ([hegel-rust]({RUST_REPO})) from "
        f"[{current}]({current_url}) to [{new}]({new_url}).\n"
    )

    app_id = os.environ["HEGEL_RELEASE_APP_ID"]
    git("config", "user.name", "hegel-release[bot]")
    git("config", "user.email", f"{app_id}+hegel-release[bot]@users.noreply.github.com")

    # The per-version branch for this release. Commit locally only; the
    # workflow pushes it after folding in the FFI alignment.
    branch = BRANCH_PREFIX + new
    git("checkout", "-B", branch)
    git("add", str(VERSION_FILE), str(LIBS_DIR), str(RELEASE_MD))
    git(
        "commit",
        "-m",
        f"Bump pinned libhegel to {new}",
        "-m",
        f"Pins libhegel from {current} to {new} ({new_url}).",
    )

    set_output("bumped", "true")
    set_output("version", new)
    set_output("branch", branch)


if __name__ == "__main__":
    # An optional argument pins that exact version; with none we take the
    # latest. The repository_dispatch trigger passes client_payload.version.
    bump(sys.argv[1] if len(sys.argv) > 1 else "")
