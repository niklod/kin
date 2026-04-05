# Decision 009: Relay — signaling only, no traffic forwarding

**Date:** 2026-04-04

## What
The relay server acts as a **rendezvous / signaling** point only. It never forwards application traffic between nodes. Its three roles are:

1. **UDP port discovery** — node sends a UDP datagram to the relay from its listen socket; relay echoes the source `ip:port` back so the node learns its actual NAT-mapped external UDP port (see `docs/017-nat-port-discovery.md`).
2. **Address registration** — node connects via TCP, sends `RelayRegister{node_id, pubkey, discovered_port}`; relay records `external_addr = tcp_source_ip + ":" + discovered_port`.
3. **Rendezvous** — when node B wants to reach node A, B asks the relay; the relay pushes each party's external address to the other simultaneously, triggering UDP hole punching.

## Why
Kin is designed as a P2P system. Running a traffic relay would:
- Require significant bandwidth/infrastructure from the relay operator
- Centralise all file transfer through a single server, contradicting the P2P model
- Create a privacy concern (relay operator can see transfer metadata even if content is encrypted)

Signaling-only relay is cheap to operate (tiny messages, short-lived rendezvous exchanges) and keeps all file data on the direct P2P path.

## Connection priority
```
1. Direct QUIC  (host:port in invite)              — tried first, always
2. Relay-assisted UDP hole punch (relay:// endpoint) — UDP port discovery + prime burst + QUIC dial
3. Error "cannot connect"                          — reported to user
```

Traffic relay (TURN) is explicitly **out of scope** for the MVP. Symmetric NAT users
cannot connect without port forwarding or a VPS node on one side.

## NAT compatibility
| NAT type | Works? |
|---|---|
| No NAT (public IP / VPS) | ✅ direct |
| Full cone NAT | ✅ relay-assisted UDP punch |
| Address-restricted cone NAT | ✅ relay-assisted UDP punch |
| Port-restricted cone NAT | ✅ relay-assisted UDP punch |
| CGNAT with port remapping | ✅ relay-assisted UDP punch (port discovery fixes the port) |
| Symmetric NAT | ❌ fails, error returned |

~80–90 % of home setups fall into the working categories (verified in real-world test 2026-04-06).
