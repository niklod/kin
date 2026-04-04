# Decision 009: Relay — signaling only, no traffic forwarding

**Date:** 2026-04-04

## What
The relay server acts as a **rendezvous / signaling** point only. It never forwards application traffic between nodes. Its two roles are:

1. **Address discovery** — when a node connects to the relay, the relay records the node's external IP:port (as seen by the relay's TCP listener). Nodes can query this to learn their own external address.
2. **Rendezvous** — when node B wants to reach node A, B asks the relay; the relay pushes each party's external address to the other simultaneously, triggering a direct TCP connection attempt (NAT hole punching).

## Why
Kin is designed as a P2P system. Running a traffic relay would:
- Require significant bandwidth/infrastructure from the relay operator
- Centralise all file transfer through a single server, contradicting the P2P model
- Create a privacy concern (relay operator can see transfer metadata even if content is encrypted)

Signaling-only relay is cheap to operate (tiny messages, short-lived rendezvous exchanges) and keeps all file data on the direct P2P path.

## Connection priority
```
1. Direct TCP  (host:port in invite)          — tried first, always
2. Relay-assisted TCP simultaneous open       — if direct fails and relay:// endpoint present
3. Error "cannot connect"                     — reported to user
```

Traffic relay (TURN) is explicitly **out of scope** for the MVP. Symmetric NAT users cannot use Kin without port forwarding or a VPS.

## NAT compatibility
| NAT type | Works? |
|---|---|
| No NAT (public IP / VPS) | ✅ direct |
| Full cone NAT | ✅ relay-assisted |
| Restricted cone NAT | ✅ relay-assisted (with simultaneous open) |
| Port-restricted cone NAT | ✅ relay-assisted (with simultaneous open) |
| Symmetric NAT | ❌ fails at relay step, error returned |

~70–80 % of home routers fall into the first three working categories.
