RELEASE_TYPE: minor

This release changes how hegel-core is installed and run, and improves server crash handling:

* Instead of creating a local `.hegel/venv` and pip-installing into it, hegel now uses `uv tool run` to run hegel-core directly.
* If `uv` isn't on your PATH, hegel will automatically download a private copy to `~/.cache/hegel/uv` — no hard requirement on having uv pre-installed.
* When a server crash is detected, the next test run transparently starts a fresh server instead of failing permanently.
* Server crash error messages now include the last few lines of the server log so the root cause is visible without inspecting the log file manually.
