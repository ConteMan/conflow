# Changelog

All notable changes are documented here. This project follows Keep a Changelog and Semantic Versioning.

## [0.3.0] - 2026-07-15

### Added

- **Config Import** (Spec 021, #36): `ImportBundle` core types, preview/apply/export backend logic and HTTP endpoints, `conflow import export/preview/apply` CLI commands, and an import dialog in the UI.
- **Plan Baseline from Last Release** (#46): plans now diff against the last published state instead of blank schema defaults.
- **Entity descriptions**: optional `description` field on placements, frequency policies, and feature switches for readability; stored in the baseline only, never compiled into Firebase output.

### Changed

- **v2 placement compile format** (Spec 022): output now matches the client contract (`id` / `units` map / `enabled_config_key`), with environment-scoped unit resolution.
- **Binding matrix columns** (Spec 023, #43): environment binding matrix columns changed from iOS/Android to MAX/AdMob; placement detail shows only the current environment's bindings.
- **Key as identifier**: placement/frequency-policy/feature-switch keys now double as the internal entity ID; the redundant separate "stable ID" input was removed from the UI.
- **Schema refinements**: `fallback_behavior` is now a select with `continue` / `skip_slot` / `show_empty_safe`; `network_mode` is nullable, where empty inherits the global `ad_network_mode`.

### Fixed

- Firebase validate-only requests send an `If-Match` header; published parameters set `valueType`.
- Release plan: remote parameter count, binding completeness calculation, risk list distinction, parameter value preview, and layout/copy usability.
- Entity table: long placement keys no longer push the "unpublished changes" column out of view.
- Frontend asset embedding path corrected; `make build` now detects when `web/dist` and `internal/webui/assets` are out of sync.

## [0.2.0] - 2026-07-14

### Added

- **Mobile Ad Pack v2** (Spec 020): Versioned `mobile-ad-monetization/v2` Pack with structured frequency controls (`duration`, `interval`, `count_limit`, `shift_limit`), feature-switch references, and six entity types. Includes dedicated v2 validator, compiler, and UI form controls (DurationField, IntervalField, CountLimitField, ShiftLimitField) with packRef-threaded rendering across FrequencyDrawer, FrequencyTable, PlacementDetail, and PlacementRow.

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
