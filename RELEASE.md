RELEASE_TYPE: patch

Bump our pinned hegel-core to [0.7.1](https://github.com/hegeldev/hegel-core/releases/tag/v0.7.1), incorporating the following changes:

> This patch changes the default Hegel server settings when running inside Antithesis (i.e. when `ANTITHESIS_OUTPUT_DIR` is set in the environment) to disable health checks and database. Health checks are designed for the sort of small fast test you would run in your unit tests and are not sensible defaults for Antithesis, and the database is essentially useless inside Antithesis as replay is done via the fuzzer.
>
> — [v0.6.1](https://github.com/hegeldev/hegel-core/releases/tag/v0.6.1)

> This release adds support for the `phases` parameter in the `run_test` protocol message,
> allowing clients to control which Hypothesis phases run (e.g. `generate`, `shrink`,
> `reuse`, `target`, `explicit`, `explain`).
>
> — [v0.7.0](https://github.com/hegeldev/hegel-core/releases/tag/v0.7.0)

> This patch adds support for the `uuid` schema type, exposing Hypothesis's
> `uuids` strategy. UUIDs are returned to the client as strings in the
> canonical hyphenated form. An optional `version` field selects a specific
> UUID version (1-5).
>
> — [v0.7.1](https://github.com/hegeldev/hegel-core/releases/tag/v0.7.1)
