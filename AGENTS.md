# AGENTS.md

- Design spec: `docs/superpowers/specs/2026-11-15-socquette-design.md`; implementation plan:
  `docs/superpowers/plans/2026-11-15-socquette.md`. Read both before changing behavior.
- Public project: no dependency on private infrastructure (private registries, private modules or images), and no
  reference to the owner's private domains anywhere in the repository, this file included.
- Pure logic (validation, codecs, auth decisions, user edits) stays free of I/O; `internal/gui`, `internal/control` and
  `main` stay thin shells.
- Every user-visible string goes through `internal/i18n` (FR/EN/ES fields of `Strings`).
- `internal/gui` and `main` need CGO; without a C compiler, build them in Docker (see `docs/development.md`).
- Quality gate: `just lint`, `just test`, `just coverage`.
