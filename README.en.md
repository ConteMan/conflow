# Conflow

[中文](README.md) | **English**

> A local-first ConfigOps workbench for managing application configuration through business forms, validation, diffs, and safe publishing.

Conflow is a single Go binary with both a CLI and a local web GUI. It lets product, operations, and engineering teams manage configuration as business entities—such as ad placements, frequency policies, and feature flags—instead of copying long Firebase Remote Config JSON values.

## Status

Conflow is in the M2 configuration-core stage: the foundation and project/environment API are implemented. The first configuration pack is `mobile-ad-monetization/v1`, and the first publishing provider is Firebase Remote Config. See the [roadmap](docs/roadmap.md) for scope.

## Development

```sh
make bootstrap
make check
go run ./cmd/conflow init --dir ./examples/photo-editor
go run ./cmd/conflow serve --workspace ./examples/photo-editor
```

The frontend uses React, TypeScript, Tailwind, and shadcn/ui with Base UI primitives. Node is used only to build the frontend; release artifacts are single Go binaries.

## Documentation

- [Architecture](docs/design/architecture.md)
- [Configuration model](docs/design/config-model.md)
- [Frontend/backend HTTP API contract](docs/design/http-api.md)
- [UI design direction and prototyping workflow](DESIGN.md)
- [Roadmap and Specs](docs/roadmap.md)

## License

[MIT](LICENSE)
