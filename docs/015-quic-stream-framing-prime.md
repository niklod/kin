# 015 — QUIC Stream Framing Prime (4-byte zero header)

## What

When the dialer opens a QUIC stream via `qConn.OpenStreamSync`, it immediately writes 4 zero bytes (`[0, 0, 0, 0]`) before any application data. The listener's `Accept` path reads and discards these 4 bytes transparently before returning the `Conn` to the caller.

```
dialer:   writeFramingPrime(stream)  → writes [0x00, 0x00, 0x00, 0x00]
listener: consumeFramingPrime(stream) → reads and discards 4 bytes
```

Neither caller ever sees these bytes.

## Why: quic-go Lazy Stream Semantics

In quic-go, `(*quic.Conn).AcceptStream` does not unblock until the **first byte of data** arrives on the stream. The QUIC spec allows the server to defer processing a new stream until it carries payload; quic-go implements this strictly.

Without the framing prime, the following deadlock occurs:

1. Dialer calls `OpenStreamSync` — stream is opened, QUIC STREAM frame sent.
2. Listener calls `AcceptStream` — blocks, waiting for first byte.
3. Dialer calls `conn.Recv()` (trying to read the first application message).
4. Listener is blocked in `AcceptStream`, never reaches `conn.Send(response)`.
5. **Deadlock**: dialer waits for server response, server waits for dialer's first byte.

The 4-byte prime unblocks `AcceptStream` immediately, eliminating the deadlock regardless of whether the caller calls `Send` or `Recv` first.

## Why 4 Bytes

The existing `protocol.WriteMsg` / `ReadMsg` framing uses a 4-byte big-endian length prefix. Writing `[0, 0, 0, 0]` looks like a zero-length message to the framing layer, but `consumeFramingPrime` discards it before the framing layer ever sees it. The choice of 4 bytes is arbitrary; it matches the framing header size for conceptual symmetry.

## Alternatives Considered

- **Send a real message first**: would require callers to send a specific handshake message, changing the application API.
- **Open stream as half-closed (write-only)**: quic-go does not support this directly.
- **Use unidirectional streams for setup**: would complicate the connection model.
- **Patch quic-go**: upstream behavior; not our codebase to change.

The 4-byte zero prime is invisible to callers, adds negligible overhead (4 bytes on the first packet), and requires no API changes.

## Related

- `internal/transport/listener.go`: `writeFramingPrime`, `consumeFramingPrime`
- `docs/014-quic-shared-transport.md`
