# 023 — Debug File Logging

## What

When the `--debug` flag is set, kin writes structured logs (slog, LevelDebug) to a timestamped file at `~/.config/kin/logs/kin-debug-<timestamp>.log`. In CLI mode, logs also go to stderr. In TUI mode, logs go to the file only since the TUI owns the terminal.

## Why

Non-technical users can't describe errors. We need a diagnostic artifact they can share. stderr-only logging is ephemeral and invisible in TUI mode. File-based logs solve both problems — the user runs `kin --debug`, reproduces the issue, and sends us the log file.

## Design Choices

**Log location**: `<config-dir>/logs/` — consistent with existing `kin.sock` and `identity.key` living under `~/.config/kin/`.

**Retention**: Keep last 5 log files, prune oldest on startup. No size-based rotation — one file per session keeps things simple and each session maps to a user-reported issue.

**multiHandler**: A minimal `slog.Handler` wrapper that fans out to file + stderr. ~30 lines, no external dependencies.

**slog.SetDefault()**: Called in both `main()` and `tui.runEmbedded()` so that global slog calls in `connmgr`, `relay`, `nat` (which don't receive an injected logger) also route to the file.

**No always-on logging**: Debug logging is opt-in via `--debug`. Normal operation has zero file I/O overhead from logging.

## Alternatives Considered

- **Always-on file logging**: Rejected — unnecessary I/O for normal use, storage concerns on resource-constrained devices.
- **External logging library (zap, zerolog)**: Rejected — stdlib `slog` is sufficient, no new dependencies.
- **Size-based rotation (lumberjack)**: Rejected — over-engineering for debug-on-demand use case. Session-based retention is simpler and maps directly to user issue reports.
