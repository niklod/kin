> **SUPERSEDED** — TCP simultaneous open was abandoned due to a timing race on real NAT routers.
> See `docs/013-tcp-punch-failure-quic-migration.md` for the failure analysis and
> `docs/014-quic-shared-transport.md` + `docs/016-udp-prime-burst.md` for the replacement design.

# Decision 010: TCP Simultaneous Open for NAT Hole Punching

**Date:** 2026-04-04

## What
NAT traversal uses **TCP simultaneous open** (RFC 793 §3.4): both sides bind to a known local port with `SO_REUSEADDR`, then concurrently:
- listen for an incoming connection on that port
- dial outward to the peer's external address

On cone NATs the outgoing SYN opens a hole; the peer's SYN arrives through that hole and the connection completes.

## Why this approach
- **No UDP dependency** — the rest of Kin uses TCP/TLS; keeping one transport simplifies the codebase.
- **No extra STUN server** — the relay already provides external address discovery.
- **Correct on most home NATs** — cone NATs (full, address-restricted, port-restricted) all support simultaneous open.

## Why not UDP hole punching (STUN/ICE)
UDP hole punching (used by WebRTC) is more reliable across NAT types but requires:
- UDP transport infrastructure (new framing, new TLS handshake approach)
- A STUN server (or the relay to speak STUN)
- Significant additional code for ICE candidate gathering

This is deferred until a future phase if UDP hole punching turns out to be necessary in practice.

## Timing coordination
The relay sends `RelayRendezvous` to both sides **at the same moment**. Each side starts the simultaneous open immediately on receiving the message. The 5 s deadline (`ErrPunchTimeout`) is sufficient for RTTs up to ~4 s; typical internet RTTs are well under 200 ms.

## Limitations
- **Symmetric NAT** — each outgoing TCP connection gets a different external port; the peer cannot predict which port to connect to. Fails with `ErrPunchTimeout`.
- **Windows** — `SO_REUSEPORT` is not available; simultaneous-open fallback does direct dial only (no reuse of listen port). Works if at least one side is not behind NAT.
