# Conflow

[中文](README.md) | **English**

> A local-first ConfigOps workbench: manage Firebase Remote Config through business forms, validation, semantic diffs, and controlled publishing.

Conflow is a single Go binary with both a CLI and a local web GUI. It represents remote configuration as business entities — ad placements, frequency policies, feature switches, custom parameters — instead of asking teams to edit long JSON values in the Firebase console. The single source of truth lives in your local workspace (optionally under Git); Firebase is the publish target and audit object.

**Highlights**

- **Business-shaped editing**: domain forms with field validation, descriptions, and reference-integrity checks
- **Release plans**: every publish starts from a semantic diff with risk analysis, down to field-level changes and the exact remote parameters they touch
- **Controlled publishing**: ETag concurrency protection, validate-only preflight, explicit production confirmation
- **Audit & rollback**: immutable release records; any successful release can be rolled back and re-verified in one step
- **Multiple environments**: each environment binds its own Firebase project; business config stays single-sourced while release cadence stays independent
- **Local-first**: data lives in your workspace, credentials never enter the repo; no server dependency, a single binary works out of the box

## Installation

**macOS (Homebrew)**

```sh
brew install ConteMan/tap/conflow
```

**Windows (Scoop)**

```sh
scoop bucket add conflow https://github.com/ConteMan/homebrew-tap
scoop install conflow
```

**Direct download**

Download the tar.gz/zip for your platform from [GitHub Releases](https://github.com/ConteMan/conflow/releases/latest), extract it, and put `conflow` on your `$PATH`.

> Unsigned binary note on macOS: directly downloaded binaries are flagged by Gatekeeper. Run once before first use:
> ```sh
> xattr -dr com.apple.quarantine conflow
> ```
> Homebrew installs handle this automatically.

**Updating**

```sh
conflow update          # direct installs
brew upgrade conflow    # Homebrew
scoop update conflow    # Scoop
```

## Quick start

```sh
# Create a project workspace interactively. Creates development and production
# environments by default; Firebase project IDs can be filled in later.
conflow init --dir ./my-app-config

# Start the local web GUI (binds to 127.0.0.1 only)
conflow serve --workspace ./my-app-config
```

Open the local address printed in the terminal. For automation, use non-interactive mode:

```sh
conflow init --non-interactive --dir ./my-app-config \
  --project-id my-app --project-name "My App" --json
```

## Connecting Firebase

Fill in the project ID and service-account JSON path in the environment manager or the Firebase connection card on the overview page, or use the CLI:

```sh
conflow provider connect --workspace ./my-app-config \
  --environment development --path "$HOME/.config/conflow/firebase.json"

conflow pull --workspace ./my-app-config --environment development
```

The service-account JSON always stays at its local path; Conflow only stores a path reference in the ignored `.conflow/` local state. Connecting validates existence, readability, JSON shape, `type=service_account`, and required fields first — nothing is written or overwritten if any step fails. Remote connectivity is checked on `pull`.

**Never commit service-account JSON, access tokens, or absolute credential paths to the repository or logs.**

## Daily workflow

Configuration changes follow one controlled flow, identical in the GUI and CLI:

```
edit → validate → build a release plan (semantic diff + risks) → publish → post-publish verification
```

```sh
conflow validate  --workspace . --environment development
conflow plan      --workspace . --environment development
conflow publish   --workspace . --environment development   # production requires explicit confirmation
conflow release list --workspace . --environment development
conflow rollback  --workspace . --environment development --release <id> --confirm --idempotency-key <key>
```

Other common commands: `conflow import` (cross-workspace config import), `conflow source`, `conflow environment`. Every command supports `--json` for a stable automation envelope; see `conflow --help`.

## Developer guide

The sections below are for contributors working on Conflow itself.

**Requirements**: Go 1.25+, Node.js 24+ (development builds only; releases remain a single Go binary).

```sh
git clone https://github.com/ConteMan/conflow.git
cd conflow
make bootstrap     # install dependencies
make check         # full checks: contracts, build, Go tests, e2e

# Common targets
make web-dev       # Vite dev server
make web-build     # build the React UI and sync it as embedded Go assets
make test          # Go tests
make check-ci      # CI check set (without e2e)
```

The frontend uses React, TypeScript, Tailwind, and Base UI primitives; tables are built on TanStack Table. API contracts live in `api/openapi.yaml`; run `make api-generate` after changes to sync TypeScript types. Feature work is organized around [implementation specs](docs/specs/README.md) — one focused PR per spec.

**Documentation**

- [Architecture overview](docs/design/architecture.md)
- [Configuration model](docs/design/config-model.md)
- [HTTP API between frontend and backend](docs/design/http-api.md)
- [Implementation specs](docs/specs/README.md)
- [UI design direction and prototype flows](DESIGN.md)
- [Architecture decision records](docs/decisions/README.md)
- [Roadmap](docs/roadmap.md)
- [Contributing guide](CONTRIBUTING.md)

## License

[MIT](LICENSE)
