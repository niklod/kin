# 017 — UDP Port Discovery via Relay Echo

## What

Before registering with the relay, each node sends a small UDP datagram from its
listen socket to the relay's UDP port. The relay echoes the datagram's source
address (as seen by the relay) back to the sender. The node parses the echoed
`ip:port` and registers with the relay using the **discovered external port**
instead of its local listen port.

```
Client (local :7777)  ──UDP 0x00──►  Relay UDP :7778
                      ◄──"5.35.113.250:2243"──
Client registers with relay: ListenPort = 2243
Relay stores: external = tcpSourceIP + ":2243"  ✓
```

## Why: self-reported port ≠ NAT-mapped port

The old approach combined the relay's TCP source IP with the client's
self-reported listen port (always `7777`). This broke when the home router
remapped the port:

```
Client local port:    7777
Router external port: 2243   ← router assigns different port (NAT port remapping)
Relay registered:     ip:7777 ✗  (wrong)
Friend dials:         ip:7777 → nobody answers
```

Discovered in a real-world test on 2026-04-06: user's port 7777 was mapped to
external port 2243; friend's port 7777 was mapped to 5391. Both holes were
punched successfully after the fix.

This happens with:
- **CGNAT** (carrier-grade NAT, common with mobile/budget ISPs)
- **Port-remapping routers** that don't do port preservation
- Any NAT that assigns a different external port per mapping

## Why not STUN

Standard STUN (RFC 5389) would also solve the problem but requires:
- An additional server speaking the STUN protocol, or a library (`pion/stun`)
- An extra dependency
- More protocol surface

Since we already operate a relay server and it has a UDP socket, a minimal
echo protocol achieves the same result with ~15 lines of code.

## Implementation

**Relay** (`cmd/relay/main.go`): binds `net.ListenUDP` on the same address as
the TCP listener. For every incoming UDP datagram, writes `addr.String()` back
to the sender. No parsing, no state.

**Client** (`internal/transport/listener.go` `DiscoverExternalAddr`):
1. Sends 3 × `[0x00]` datagrams via `quic.Transport.WriteTo` from the shared
   listener socket (port 7777). Byte `0x00` is not a valid QUIC header (bits 7
   and 6 must not both be zero per quic-go's detection logic), so the relay's
   QUIC stack will not try to parse it as a QUIC packet.
2. Calls `quic.Transport.ReadNonQUICPacket` (available in quic-go ≥ 0.44) to
   receive the relay's echo without competing with QUIC traffic on the same
   socket. Filters by source IP to discard stray packets.
3. Returns the external `ip:port` string; 2 s timeout, falls back to local port
   on failure.

**connmgr** (`internal/connmgr/dial.go` `externalPort`): wraps the discovery,
logs the result, and falls back to `ln.Port()` on any error. Called in both
`ServePunch` (listener side) and `punchViaRelay` (dialer side) before
`relay.Connect`.

## Limitations

- **Symmetric NAT**: the external port may differ per destination. Discovery
  via the relay gives the port for packets sent *to the relay*. If the router
  assigns a different port for packets sent to the peer, the discovered port is
  still wrong. In this case UDP hole punching fails regardless; relay traffic
  forwarding (TURN) is the only remedy.
- **Relay reachable via UDP**: requires UDP 7778 open on the relay host. With
  `network_mode: host` in Docker this is automatic; no explicit port mapping
  needed.

## VPN interaction (observed failure mode)

If the client routes traffic to the relay through a VPN whose exit node *is*
the relay server itself, the relay sees its own IP as the UDP source. It echoes
back `relay_ip:PORT`, which is wrong. Kin then registers with the relay's own
IP, and the peer tries to punch to the relay host — not the client's home
router. Symptom: relay logs show `external=<relay_ip>:PORT` for the client.

Fix: run `kin run` without a VPN that routes to the relay, or configure
split-tunneling to exclude the relay address.

## Related

- `docs/009-relay-architecture.md` — relay design overview
- `docs/014-quic-shared-transport.md` — why the shared socket is used
- `docs/016-udp-prime-burst.md` — how NAT mappings are opened for hole punching
- `internal/transport/listener.go`: `DiscoverExternalAddr`
- `internal/connmgr/dial.go`: `externalPort`
- `cmd/relay/main.go`: `serveUDPEcho`
