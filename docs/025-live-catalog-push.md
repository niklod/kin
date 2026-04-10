# 025 — Live Catalog Push to Connected Peers

## What

When local files change (add/modify/delete), the updated catalog is now pushed to all currently connected P2P peers in real-time, not just to IPC subscribers (TUI).

## Why

Before this change, catalog exchange only happened once at connection time. If peer A added a file while peer B was connected, B would never learn about it until the next reconnection. This caused asymmetric catalog visibility during real-world testing between macOS and Windows peers.

The root issue: the watcher's `onChange` callback only called `srv.BroadcastCatalogUpdated()` which notifies local IPC subscribers (TUI). Connected P2P peers were not notified.

## Design

- `protocol.Handler` now maintains a **connection registry** (`map[[32]byte]Conn` protected by `sync.Mutex`). Connections are registered on `Serve()` entry and unregistered on exit (via `defer`).

- `Handler.BroadcastCatalog()` iterates the registry snapshot, calls `ListForPeer(peerID)` for each (respecting loop prevention), and sends a `CatalogOffer` to each connected peer.

- The daemon wires two triggers to `BroadcastCatalog()`:
  1. **Local file change** (watcher `onChange`): notifies IPC + pushes to peers
  2. **Peer catalog received** (`onCatalogUpdate`): notifies IPC + re-broadcasts to other connected peers (catalog propagation)

## Tradeoffs

- **Full catalog re-send on every change**: simple but sends all entries, not just the delta. Acceptable for current scale (small shared folders). If catalogs grow large, a diff-based approach would be better.

- **No deduplication of rapid broadcasts**: if multiple files change in quick succession, each triggers a full broadcast. The watcher's 500ms debounce mitigates this per-file, but multiple files changing concurrently will each trigger a broadcast. Acceptable for now.

## Also Fixed

- Invite endpoints no longer include non-routable wildcard addresses (`[::]:7777`, `0.0.0.0:7777`) that caused spurious connection failures on Windows.
