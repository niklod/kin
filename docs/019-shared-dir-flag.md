# 019 — `--shared-dir` CLI Flag

## What

Added `--shared-dir` flag to `kin run` to override the default shared folder path (`~/Kin`).

## Why

The default shared folder is hardcoded to `~/Kin`. Running two kin instances on the same machine (for E2E testing or development) requires separate shared folders. Without a flag, both instances would watch and modify the same directory, making isolated testing impossible.

## Alternatives Considered

- **Environment variable (`KIN_SHARED_DIR`)**: No precedent in the codebase (zero `os.Getenv` calls). CLI flags are the established configuration pattern.
- **Symlink per instance**: Fragile, platform-dependent cleanup, race conditions.

## Decision

CLI flag `--shared-dir` mirrors the existing `--config-dir` pattern. When omitted, `config.DefaultSharedDir()` is used as before.
