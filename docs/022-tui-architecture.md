# 022 — TUI Architecture

## What

Interactive terminal UI built with bubbletea (Elm architecture). Launched via
`kin` with no subcommand. Requires a running daemon (`kin run`).

## Why

A TUI provides real-time visibility into peers, catalog, and transfers without
needing a web UI or desktop app. The Elm architecture (Model/Update/View) makes
the UI logic testable as pure functions.

## Design

### IPC Connection Model

The TUI opens **two** IPC connections to the daemon:
1. One for request/response RPC (Status, Peers, Catalog, Invite, Join)
2. One dedicated to `Subscribe()` which takes over the connection for event streaming

This avoids multiplexing events and RPC on the same connection.

### Component Structure

```
internal/tui/
  model.go          — Top-level Model composing sub-models
  catalog_model.go  — File catalog with vim-like navigation
  peer_model.go     — Peer list with navigation
  statusbar.go      — Bottom status bar
  overlay_*.go      — Modal overlays (help, invite, join, confirm, detail)
  progress.go       — Transfer progress tracking
  commands.go       — tea.Cmd functions bridging IPC to bubbletea
```

### Event Flow

Daemon events arrive via a `tea.Cmd` that blocks on the event channel:

```go
func waitForEvent(ch <-chan daemon.Event) tea.Cmd {
    return func() tea.Msg {
        ev, ok := <-ch
        if !ok { return daemonDisconnected{} }
        return daemonEvent{ev}
    }
}
```

After processing each event, the model re-issues `waitForEvent` to keep listening.

### Overlay Pattern

All overlays implement `Overlay` interface (`Update` + `View`). When active, the
top-level model routes all keys to the overlay. Returning `nil` from `Update`
closes the overlay. This is simple and requires no special close messages.

### Layout

Two-panel layout (70/30 split) with bottom status bar. At terminal width < 80,
peer panel collapses. Overlays render centered over dimmed background.

## Tradeoffs

- Consumer-side `DaemonClient` interface decouples TUI from IPC implementation.
- Download via daemon IPC (not direct file access) — requires daemon-side wiring
  of the download package, currently stubbed with "not implemented".
- No clipboard integration for invite tokens — left to the terminal emulator.
