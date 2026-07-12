# Conflow

[中文](README.md) | **English**

> A local-first ConfigOps workbench for managing application configuration through business forms, validation, diffs, and controlled publishing.

Conflow is a single Go binary with both a CLI and a local web GUI. It represents configuration as business entities such as ad placements, frequency policies, and feature flags, rather than asking teams to copy long Firebase Remote Config JSON values directly.

## Quick Start

```sh
git clone https://github.com/ConteMan/conflow.git
cd conflow
make bootstrap
make check

# Interactively create a project and first environment. Firebase project ID can be added later.
go run ./cmd/conflow init --dir ./examples/photo-editor

# Automation must provide explicit creation values; missing required values exit with code 64.
go run ./cmd/conflow init --non-interactive --dir ./examples/photo-editor \
  --project-id photo-editor --project-name "Photo Editor" \
  --environment-id development --environment-name Development \
  --environment-kind development --provider-project-id photo-editor-dev

go run ./cmd/conflow serve --workspace ./examples/photo-editor
```

Open the local address printed by the terminal. The overview page can create additional environments. Firebase project ID may be blank initially, but must be completed in Environment Management before connecting or pulling.

## Connect Firebase

Service account JSON remains at its local path. Conflow stores only a path reference in ignored local `.conflow/` state. The GUI Firebase connection card clears the input after submission and displays only a redacted tail such as `…/firebase.json`.

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
