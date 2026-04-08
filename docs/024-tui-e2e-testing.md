# 024: TUI E2E Testing via PTY + VT10x

## What

E2E tests for the TUI use a pseudo-terminal (PTY) with a VT10x virtual terminal emulator to test actual TUI process rendering and interaction.

## Why

The existing E2E framework only tested the headless daemon mode via CLI one-shot commands. The TUI (Bubbletea) uses ANSI escape codes, alternate screen buffer, and cursor positioning — raw output cannot be parsed as plain text. A VT10x terminal emulator interprets all escape sequences and maintains a clean screen buffer, giving tests reliable screen content for assertions.

Alternatives considered:
- **IPC-only testing**: Would miss rendering bugs; doesn't test the actual TUI process.
- **Bubbletea teatest**: Unit-level model testing, not process-level E2E.
- **ANSI stripping**: Bubbletea redraws the full screen via cursor positioning — stripping codes without a state machine gives garbled output.

## Key Decisions

1. **PTY via `creack/pty`**: Industry-standard Go PTY library. Starts the TUI process with a real terminal, supporting Bubbletea's terminal detection.

2. **VT10x via `hinshun/vt10x`**: Maintains a virtual terminal grid interpreting CSI sequences, alternate screen buffer (`\e[?1049h`), cursor movement, and color attributes. `String()` returns clean plaintext screen content.

3. **Connected mode**: TUI connects to a separate `kin run` daemon (not embedded mode). This tests the real IPC communication path and avoids embedded-mode complexity.

4. **Screen polling**: 50ms polling interval for `WaitForScreen()` assertions. No notification-based approach because vt10x lacks change callbacks. Fast enough for TUI frame rates.

5. **Hybrid verification**: Screen assertions for visual correctness + IPC client for structural data validation.

## Catalog Update Broadcast Fix

During TUI E2E development, two missing event broadcasts were discovered:
- **Watcher → IPC**: The file watcher updated the catalog store but never broadcast `catalog_updated` to IPC subscribers. Fixed by adding an `onChange` callback to `Watcher`.
- **Protocol handler → IPC**: Receiving a remote catalog offer updated the store but didn't notify subscribers. Fixed by adding `SetOnCatalogUpdate` to the protocol `Handler`.

Without these fixes, the TUI's real-time catalog display was non-functional — it only showed the catalog state at startup time.
