# 014 — QUIC Shared Transport: One UDP Socket per Process

## What

`transport.Listener` owns a single `*quic.Transport` that wraps one `*net.UDPConn`. This transport is used for:
- **Inbound**: accepting QUIC connections via `*quic.Listener` (obtained from `quic.Transport.Listen`)
- **Outbound**: dialing QUIC connections via `quic.Transport.Dial`
- **Priming**: sending raw UDP datagrams via `quic.Transport.WriteTo`

Both `Listener.Accept` and `Listener.Dial` funnel through the same underlying UDP socket.

## Why

NAT hole punching depends on the 4-tuple `(srcIP:srcPort, dstIP:dstPort)`. When a node sends a UDP packet from port P to the peer, the local NAT router creates a mapping: `P → peer`. Subsequent QUIC packets from the peer can only traverse that mapping if they arrive at the exact same local port P.

This means the QUIC dial that follows the priming burst **must originate from the same UDP port** that sent the prime packets. A separate `net.ListenUDP` socket would get a different ephemeral port and the mapping would not match.

`quic.Transport` makes this trivially correct: one call to `Transport.Dial` reuses the same socket that `Transport.WriteTo` used for priming.

## Consequence: One UDP Port per Process

Because all I/O goes through one socket, a single kin process can only bind one UDP port. Two instances on the same host (e.g. `kin run` and `kin join` running concurrently) cannot share the default port `0.0.0.0:7777`.

**Mitigation:** `kin join` documents that users should pass `--listen 0.0.0.0:0` (OS-assigned ephemeral port) when `kin run` is already running on the default port.

## SO_REUSEPORT Removed

The previous TCP implementation used SO_REUSEPORT to allow a second socket (the punch dialer) to bind the same port as the listener. With QUIC and `quic.Transport`, SO_REUSEPORT is no longer needed — and would be harmful: the kernel would split incoming UDP datagrams non-deterministically between two sockets, breaking quic-go's connection tracking.

Files `internal/transport/listenconfig_unix.go`, `listenconfig_other.go`, `internal/nat/sockopt_unix.go`, and `sockopt_other.go` were deleted.

## Standalone Dial

`transport.Dial` / `transport.DialContext` (used only by tests and simple non-punch scenarios) open a fresh UDP socket + transient `*quic.Transport` per call. The transport is closed asynchronously via a goroutine watching `qConn.Context().Done()`. These functions are not suitable for NAT punch; use `Listener.Dial` instead.

## Related

- `docs/013-tcp-punch-failure-quic-migration.md` — why TCP punch was abandoned
- `docs/016-udp-prime-burst.md` — how priming works
