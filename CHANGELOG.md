# Changelog

All notable changes are documented here. This project follows Keep a Changelog and Semantic Versioning.

## [0.4.0] - 2026-07-15

### Added

- **Entity change tracking against last release** (Spec 024, #52): publish/rollback now persists a per-entity content-hash baseline (`.conflow/released-baseline/`); draft entities expose `change_status` (unchanged / modified / created) and drafts expose `changed_entity_count`, so lists and the header badge only flag real unpublished changes instead of everything imported into the draft layer.
- **Tiered plan invalidation with auto-rebuild** (Spec 025, #55): plan responses carry a structured `invalidation` object (`code`, `tier`, `message`). Routine invalidations (TTL expiry, draft advance) rebuild automatically with a loop guard; external ones (remote template changed outside Conflow, pack change) surface a single in-page banner. The blocking invalidation modal was removed.
- **Entity descriptions in lists** (#53): placement / frequency-policy / feature-switch rows show the `description` field with a muted fallback; the unit-binding overview identifies rows by placement key + description instead of internal binding IDs.
- **Feature switch editing** (#54): switches gained a create/edit drawer (risk level, rollback method, description) with conflict handling, plus row-level edit and delete actions with reference checks.
- **Snapshot freshness for default downloads** (#57): the release history download card shows when the protected snapshot was observed and its remote version, warns when it is older than 24 h, and disables downloads until a snapshot exists. `RemoteProjection` now exposes `version`.

### Changed

- **Config lists migrated to TanStack Table** (Spec 026, #51): a shared headless DataTable provides first/last column pinning with scroll shadows, client-side sorting, and a unified toolbar (stats + filters + search); hand-rolled table markup and CSS width hacks were removed.
- **Overview information architecture** (#48, #49): environment management moved from the overview page into project settings; the Firebase connection card became the single entry point (project ID + service-account path + verification), action buttons gained captions, "拉取线上配置" was renamed "拉取远端快照", and connection status is checked automatically on mount.
- **Chinese-first copy** (#50): the v2 pack description, schema-version label, and capability tags now render in Chinese.

### Fixed

- Rolling back to a release that predates Conflow-state capture no longer fails; the entity baseline is cleared and rebuilt on the next successful release.
- An invalidated empty plan shows the rebuild flow instead of a misleading "no pending changes" state.
- Nine stale Playwright assertions were realigned with the current UI (stable-ID removal, MAX/AdMob binding matrix, switch drawer); the e2e suite grew from 42 to 45 passing tests.

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
