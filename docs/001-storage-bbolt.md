# Decision 001: bbolt for local storage

**Date:** 2026-04-04

## What
Use bbolt (embedded key-value store) for PeerStore and nonce storage.

## Why
- No CGO dependencies — simplifies cross-compilation for macOS and Windows
- Single file database — easy backup, no external process
- Sufficient for the data model: peer list (tens to hundreds of entries) and nonce replay tracking
- Used in etcd, proven reliability
- Alternative considered: SQLite via `modernc.org/sqlite` — more powerful query capabilities (JOINs, WHERE), but heavier and unnecessary for our simple key-value access patterns

## How
- Bucket `peers`: key = NodeID (32 bytes), value = JSON-encoded Peer struct
- Bucket `nonces`: key = nonce (16 bytes), value = empty (presence check)
- JSON encoding chosen for debuggability; internal to store, swappable later
