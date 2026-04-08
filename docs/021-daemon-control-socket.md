# 021 — Daemon Control Socket (IPC)

## What

`kin run` opens a Unix domain socket at `~/.config/kin/kin.sock` and accepts
JSON-RPC-like commands from CLI and TUI clients. All daemon operations (status,
peers, invite, join, catalog) are available over the socket.

## Why

bbolt uses an exclusive file lock — only one process can open the database for
writing. When `kin run` holds the write lock, `kin status` and other CLI
commands cannot access peer data directly. A control socket decouples the
daemon's internal state from direct file access, enabling:

- CLI commands to query and mutate state while the daemon is running
- TUI (future) to receive real-time events via subscription
- Single-instance enforcement (second daemon detects live socket)

Alternatives considered:
- **gRPC over Unix socket**: adds protobuf code generation overhead for simple
  internal messages. The IPC messages are not part of the wire protocol.
- **TCP loopback**: works but requires port allocation and lacks single-instance
  semantics.
- **Shared bbolt with read-only mode**: already tried (`OpenReadOnly`), fails
  when the write lock is held.

## Wire Format

`[4 bytes BE uint32 length][JSON payload]` — same framing as the peer wire
protocol (`internal/protocol/framing.go`), but JSON instead of protobuf.

Request: `{"id": N, "method": "...", "params": {...}}`
Response: `{"id": N, "data": {...}}` or `{"id": N, "error": {...}}`

## Methods

| Method    | Description                                  |
|-----------|----------------------------------------------|
| status    | Node identity, peer count, relay status      |
| peers     | List of known peers with trust state         |
| invite    | Generate a new invite token                  |
| join      | Join a peer via invite token                 |
| catalog   | List all catalog entries                     |
| subscribe | Event stream (peer online/offline, catalog)  |

## Stale Socket Handling

On startup, if `kin.sock` exists:
1. Try connecting to it
2. Connection refused → stale socket from crash, delete and proceed
3. Connection accepted → another daemon is running, exit with error

## Subscription Model

The `subscribe` method takes over the connection for push events. TUI opens
two connections: one for request/response, one for the event stream.

## Tradeoffs

- JSON adds ~2x overhead vs protobuf for IPC messages, but these are small
  and infrequent — debuggability is more valuable here.
- Unix sockets work on macOS, Linux, and Windows 10 1803+. For older Windows,
  named pipes would be needed (deferred to T.3).
