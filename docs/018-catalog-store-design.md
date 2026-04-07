# 018 — Catalog Store & Exchange Design

## What

Phase 2 introduces a persistent file catalog (`internal/catalog/`), a filesystem watcher (`internal/watcher/`), a catalog exchange protocol, and catalog-aware file download (`internal/download/`).

## Why

Phase 1 only supported file transfer by known SHA-256 hash with no discovery mechanism. For a shared folder experience, peers need to know what files each other has. The catalog makes file metadata persistent and exchangeable.

## Key Decisions

### Separate `catalog.db` (not in `peers.db`)

One concern per database file. This avoids lock contention between peer operations and catalog operations, and keeps the bbolt files focused.

### Composite key `[owner_node_id][file_id]` (64 bytes)

The same file_id from two different peers produces distinct entries. Prefix scan on `owner_node_id` gives efficient per-peer iteration — needed for catalog exchange (send all my files) and for purging a disconnected peer's entries.

### Availability index as denormalized bucket

The `availability` bucket maps `file_id → []OwnerHint` for fast "who has this file?" lookups without scanning all entries. Updated in the same transaction as entry writes for consistency.

### Replace-all semantics on `PutPeerEntries`

When a peer reconnects and sends its catalog, we delete all existing entries for that peer and insert the new set. This makes reconnection fully idempotent — no duplicate tracking needed.

### `CatalogOffer` / `CatalogAck` protobuf messages

Both sides send `CatalogOffer` on connection start. The acceptor's `Handler.Serve` sends it before entering the recv loop. The joiner (`cmdJoin`) sends it inline. Loop prevention: entries owned by the target peer are excluded from the offer.

### fsnotify with debouncing

File write events arrive in bursts (editor: CREATE, WRITE, WRITE, CHMOD). A 500ms debounce timer per path avoids hashing incomplete files mid-write.

### Consumer-side interfaces

The watcher defines `CatalogWriter` and `LocalIndexer` interfaces locally. The handler defines `CatalogExchanger`. The downloader defines `CatalogReader`, `PeerDialer`, `FileIndexer`. This follows the Go convention of accepting interfaces, returning structs.

## Tradeoffs

- **Full catalog exchange** (not incremental diff): Simple, correct, idempotent. Scales to ~20K files per 4 MiB message. Incremental sync can be added later if needed.
- **fsnotify only watches top-level dir**: Subdirectory watching would require recursive adds. Sufficient for the flat `~/Kin` folder in MVP.
- **Download tries owners sequentially**: Could be parallelized for faster fallback, but sequential is simpler and avoids wasted bandwidth.
