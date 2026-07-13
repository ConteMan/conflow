# Contributing to Conflow

[中文](CONTRIBUTING.md) | **English**

Conflow is documentation-first. Read [`AGENTS.md`](AGENTS.md), then [`docs/`](docs/README.md), before changing code.

Requirements: Go 1.25+, Node.js 24+, npm, and Playwright Chromium. `make bootstrap` installs the browser used by frontend E2E tests.

```sh
make bootstrap
make check      # full local gate, including Playwright E2E
make check-ci   # browser-free gate used by GitHub Actions
```

**Reporting issues:** Use [GitHub Issues](https://github.com/ConteMan/conflow/issues). Labels: `bug` (wrong behaviour), `design-gap` (diverges from the UI prototype), `enhancement` (improvement within existing scope), `spec-needed` (requires a new Spec before work can start). See [AGENTS.md](AGENTS.md) for the full workflow.

Create `feat/<name>`, `fix/<name>`, or `docs/<name>` branches from `main`. Keep architecture, schema, public CLI, and HTTP API changes aligned with design documents or ADRs. Use English Conventional Commits and run the full local `make check` before opening a PR; GitHub Actions runs the browser-free `make check-ci` target.
