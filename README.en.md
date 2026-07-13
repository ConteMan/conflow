# Conflow

[中文](README.md) | **English**

> A local-first ConfigOps workbench for managing application configuration through business forms, validation, diffs, and controlled publishing.

Conflow is a single Go binary with both a CLI and a local web GUI. It represents configuration as business entities such as ad placements, frequency policies, and feature flags, rather than asking teams to copy long Firebase Remote Config JSON values directly.

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

Download the archive for your platform from [GitHub Releases](https://github.com/ConteMan/conflow/releases/latest), extract, and place `conflow` in your `$PATH`.

> macOS unsigned binary: downloads are quarantined by Gatekeeper. Run once before first use:
> ```sh
> xattr -dr com.apple.quarantine conflow
> ```
> Homebrew installation handles this automatically.

**Updating**

```sh
conflow update          # direct install
brew upgrade conflow    # Homebrew
scoop update conflow    # Scoop
```

## Quick Start

```sh
git clone https://github.com/ConteMan/conflow.git
cd conflow
make bootstrap
make check

# Interactively create a project. Development and production are created by default; Firebase project IDs can be added later.
go run ./cmd/conflow init --dir ./examples/photo-editor

# Automation must provide explicit project values; missing required values exit with code 64.
# --json returns project_path and a next_steps array.
go run ./cmd/conflow init --non-interactive --dir ./examples/photo-editor \
  --project-id photo-editor --project-name "Photo Editor" --json

go run ./cmd/conflow serve --workspace ./examples/photo-editor
```

Open the local address printed by the terminal. The overview page can create additional environments. Firebase project ID may be blank initially, but must be completed in Environment Management before connecting or pulling.

## Connect Firebase

Service account JSON remains at its local path. Conflow stores only a path reference in ignored local `.conflow/` state. Connect first validates existence, readability, JSON syntax, `type=service_account`, and required fields; a failure never writes or replaces an existing reference. The GUI Firebase connection card clears the input after submission and displays only a redacted tail such as `…/firebase.json`. Remote connectivity is checked by `pull`.

```sh
go run ./cmd/conflow provider connect --workspace ./examples/photo-editor \
  --environment development --path "$HOME/.config/conflow/firebase.json"

go run ./cmd/conflow pull --workspace ./examples/photo-editor --environment development
```

Do not commit service account JSON, access tokens, or absolute credential paths to the repository or logs.

## Development

```sh
make web-dev       # Vite development server
make web-build     # Build React UI and sync Go embedded assets
make test
make check
```

The frontend uses React, TypeScript, Tailwind, and shadcn/ui Base UI primitives. Node is only used for development builds; releases remain single Go binaries.

## Documentation

- [Architecture](docs/design/architecture.md)
- [Configuration model](docs/design/config-model.md)
- [Frontend/backend HTTP API contract](docs/design/http-api.md)
- [UI design direction and prototyping workflow](DESIGN.md)
- [Architecture decisions](docs/decisions/README.md)
- [Roadmap](docs/roadmap.md)
- [Contributing](CONTRIBUTING.md)

## License

[MIT](LICENSE)
