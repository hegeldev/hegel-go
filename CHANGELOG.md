# Changelog

## 0.3.0 - 2026-04-02

This release changes how hegel-core is installed and run, and improves server crash handling:

* Instead of creating a local `.hegel/venv` and pip-installing into it, hegel now uses `uv tool run` to run hegel-core directly.
* If `uv` isn't on your PATH, hegel will automatically download a private copy to `~/.cache/hegel/uv` — no hard requirement on having uv pre-installed.
* When a server crash is detected, the next test run transparently starts a fresh server instead of failing permanently.
* Server crash error messages now include the last few lines of the server log so the root cause is visible without inspecting the log file manually.

## 0.2.1 - 2026-04-01

This patch updates documentation: moves the getting-started guide into the package doc comment, fixes code examples, and updates the README to match the style of other Hegel SDKs.

## 0.2.0 - 2026-03-30

Switch from Unix socket transport to stdio-based communication with the hegel-core binary.

## 0.1.8 - 2026-03-30

This release brings in various robustness features already implemented in hegel-rust:

* Write server output to .hegel/server.log
* Better handling of server crashes
* Health check support
* Flaky test detection

## 0.1.7 - 2026-03-26

Move conformance binaries and improve staticcheck linting

## 0.1.6 - 2026-03-25

This release removes an internal implementation detail (`runtime.Goexit`) from public documentation and adds a compile-time check that `T` satisfies `testing.TB`.

## 0.1.5 - 2026-03-25

Fix a race condition in installer tests.

## 0.1.4 - 2026-03-24

Rename the module to hegel.dev/go/hegel

## 0.1.3 - 2026-03-20

Migrate generators to builder style options instead of functional options.

## 0.1.2 - 2026-03-19

This release changes the way the go library automatically manages its hegel binary to match the rust library.

## 0.1.1 - 2026-03-18

Improve docs on URLs() and remove Times()

## 0.1.0 - 2026-03-18

Allow running test cases in parallel

## 0.0.2 - 2026-03-12

Remove requirement to pass test name in `runHegel`.

## 0.0.6 - 2026-03-10

Add validation to generator arguments.

## 0.0.5 - 2026-03-10

Change generators to use standard library types where it makes sense and use functional options throughout

## 0.0.4 - 2026-03-09

Improve documentation by removing implementation details.

## 0.0.3 - 2026-03-05

Refactor internal generator code.

## 0.0.2 - 2026-03-05

Rework the API to integrate better with testing.T

## 0.0.1 - 2026-02-27

Complete rewrite with full protocol implementation, generator
combinators, type-directed derivation, conformance tests, and getting-started
documentation. Adds release automation.
