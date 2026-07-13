# Changelog

All notable changes are documented here. This project follows Keep a Changelog and Semantic Versioning.

## [0.1.0] - 2026-07-13

### Added

- **Foundation** (Spec 001): Go 1.25 single-binary CLI with embedded React 19/Vite/Tailwind frontend, Cobra command tree, automated contract verification, and project-level conventions.
- **Project & Environments API** (Spec 002): Project manifest (`project.yaml`), environment CRUD, optimistic revision control, and generated TypeScript API types.
- **Config Pack Contract** (Spec 003): Versioned, declarative Pack registry with schema compatibility checks and concurrency-safe built-in registration.
- **Draft Layering** (Spec 004): Targeted draft-layer contract with scoped replacements, field presence/origin tracking, and typed concurrency errors.
- **Managed File Source** (Spec 005): Local JSON source adapter with snapshot tokens and idempotent save.
- **Mobile Ad Pack** (Spec 006): `mobile_ad` Pack defining `frequency_policy` and `ad_placement` entity schemas with field validation.
- **Validation Engine** (Spec 007): Pack-driven validation producing structured error envelopes; blocking vs. warning severity; exit codes 1/2.
- **Plan Semantic Diff** (Spec 008): Async Operation model, snapshot tokens, 15-min TTL, stable semantic diff output for CI consumption.
- **Firebase Read & Pre-validation** (Spec 009): Firebase Remote Config provider adapter; remote-state read, pre-validation, and `preview_only` plan gate.
- **Firebase Publish** (Spec 010): Atomic publish with concurrency protection, per-environment etag lock, and `--confirm` gate.
- **Release Audit & Rollback** (Spec 011): Immutable release log, audit trail, configurable rollback with `--confirm` gate.
- **UI Design Prototype** (Spec 012): Pencil-based design system with component library, flow diagrams, and interaction spec.
- **UI Project Shell** (Spec 013): React project shell — environment selector, persistent Production context, revision conflict recovery, read-only states.
- **UI Domain Editor** (Spec 014): Config Pack editor with frequency policy and ad placement drawers, field validation, 422 inline errors.
- **UI Plan & Release** (Spec 015): Validation center, release plan page, release flow with diff view, history, and rollback.
- **Git JSON Source** (Spec 016): Git-backed JSON source adapter; `source import/preview-save` with branch/file detection.
- **Git Review Workflow** (Spec 017): `git prepare/create-branch/commit/status` commands for CI-driven review PRs.
- **CLI Automation Contract** (Spec 018): Stable JSON envelope (`--json`) for every command, typed exit codes, automation contract test, and `conflow --example` output.
- **First-run Onboarding**: Interactive `conflow init` wizard with per-field validation and next-step guide; entity creation entries (frequency policy, feature switch) in UI; `conflow provider connect` with 5-step validation chain.
- **Cross-platform Release** (Spec 019): GoReleaser multi-platform builds (macOS/Linux/Windows, amd64/arm64), checksums, SBOM, GitHub Releases, Homebrew cask, and Scoop bucket. `conflow update` command with checksum-verified binary replacement.
