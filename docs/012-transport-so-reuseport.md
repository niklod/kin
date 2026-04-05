> **SUPERSEDED** — SO_REUSEPORT was removed when the transport migrated to QUIC (UDP).
> `quic.Transport` owns the single UDP socket for both listening and dialing, making
> SO_REUSEPORT unnecessary (and harmful — it would split datagrams non-deterministically).
> See `docs/014-quic-shared-transport.md`.

# Decision 012: SO_REUSEPORT on the Transport Listener

**Date:** 2026-04-04

## What

`transport.Listen` sets `SO_REUSEPORT` on the TCP listen socket (macOS and Linux only). `nat.Punch` also sets `SO_REUSEADDR` + `SO_REUSEPORT` on its outgoing dial socket.

## Why

NAT hole punching requires the outgoing TCP SYN to leave from the node's declared `listenPort` (the port reported to the relay). This allows the NAT to map `externalIP:listenPort → internalIP:listenPort`, so the remote peer's SYN — addressed to `externalIP:listenPort` — arrives at the node's internal listener.

Without `SO_REUSEPORT` on both sockets, the kernel rejects the second `bind()` with `EADDRINUSE` because the listener already holds the port.

## Platform coverage

| Platform | SO_REUSEPORT | Result |
|---|---|---|
| Linux, macOS (darwin) | Set on both sockets | Full punch support |
| Other (Windows, etc.) | No-op (`sockopt_other.go`) | Punch proceeds with OS-chosen source port; works if local node is not behind NAT |

## `transport.DialContext`

Added alongside `Dial` so connection manager and callers can pass a context for cancellation. `Dial` now delegates to `DialContext(context.Background(), ...)`.
