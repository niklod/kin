# Decision 011: Connection Manager Dial Strategy

**Date:** 2026-04-04

## What

`connmgr.Dialer.Dial` tries connection strategies in order:

1. **Direct TCP** — `transport.DialContext` to each non-`relay://` endpoint.
2. **Relay + NAT punch** — for each `relay://addr` endpoint: connect to relay, `RequestRendezvous`, then `nat.Punch` to the peer's external address.
3. **ErrNoRoute** — if all strategies fail.

## Why sequential, not concurrent

Concurrent attempts would open multiple TCP connections and relay sessions unnecessarily. Direct TCP is cheap and fast to fail (connection refused is near-instant); relay registration has non-trivial overhead. Sequential keeps resource usage minimal.

## ServePunch has no punch-back

When `kin run --relay addr` is active, `ServePunch` keeps the node registered with the relay. When a `RelayRendezvous` arrives on `Incoming()`, the node does **not** punch back to the remote peer. Instead, the remote peer's `nat.Punch` call connects directly to the node's `transport.Listener`.

This avoids a TLS role conflict: if both sides called `nat.Punch` to each other, both would use TLS-client mode and the handshake would fail with "unexpected ClientHello". The listener always acts as TLS server; the punch initiator always acts as TLS client.

## Symmetric NAT

Not supported. If the peer is behind symmetric NAT (where each outgoing connection gets a different external port), `nat.Punch` times out and `ErrNoRoute` is returned. This is intentional per the product decision to keep the MVP simple.

## Relay reconnect backoff

`kin run` reconnects to the relay on drop using exponential backoff starting at 1 s, doubling up to 30 s max, to avoid hammering an unavailable relay server.
