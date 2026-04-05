# 016 — UDP Prime Burst: 5 Packets × 20 ms

## What

Before attempting a QUIC dial to a peer behind NAT, the local node sends 5 small UDP datagrams to the peer's external address at 20 ms intervals (80 ms total):

```go
const (
    primeBurst    = 5
    primeInterval = 20 * time.Millisecond
)
```

Both sides do this immediately upon receiving `RelayRendezvous`, before the initiating side calls `Listener.Dial`.

## Why: UDP NAT Mapping Is Stateless

A UDP NAT mapping is created by the router the moment an outbound UDP packet exits the local socket. There is no handshake — just "packet sent → mapping exists". This is fundamentally different from TCP, where the router waits for a SYN to establish state.

**Consequence:** there is no timing race between the two peers. Each side creates its own mapping independently. By the time the QUIC Initial packet arrives from the peer, the mapping is already in place.

This is the core reason UDP hole punching is more reliable than TCP simultaneous open. The TCP version required a `300 ms` `punchPrimeDelay` sleep to wait for the remote peer's SYN to arrive and for the NAT router to process it; with UDP, no delay is needed.

## Why a Burst of 5

Some home routers silently drop the **first** UDP packet to an unknown destination (anti-spoofing heuristic, rate limiting, or stateful-UDP mode). Sending 5 packets provides redundancy: even if packets 1–2 are dropped, packets 3–5 will open the mapping.

The 20 ms interval is chosen to avoid triggering per-second rate limits while still completing the burst well within the QUIC `HandshakeIdleTimeout` (5 s).

## QUIC Initial Retransmission as Final Safety Net

quic-go retransmits QUIC Initial packets at approximately 200 ms intervals if no response is received. This provides additional resilience: even if all 5 prime packets are dropped, the first QUIC Initial acts as a prime itself, and the retransmitted Initial (200 ms later) arrives after the peer's mapping is open.

In practice the burst + QUIC retransmission gives at least 3–4 independent opportunities to complete the handshake within the 5 s `punchTimeout`.

## What the Prime Packets Look Like

Each prime packet contains a single byte `[0x00]`, sent via `quic.Transport.WriteTo`. This is not a valid QUIC packet; the peer's QUIC stack discards it silently. Its only purpose is to trigger the NAT router to record an outbound mapping for the 4-tuple `(localIP:localPort, peerIP:peerPort)`.

## Related

- `internal/nat/punch.go`: `PrimeNAT`, `Punch`
- `docs/014-quic-shared-transport.md` — why the same socket must be used
- `docs/013-tcp-punch-failure-quic-migration.md` — the timing race that motivated this design
