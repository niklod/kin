# Decision 002: Protobuf wire protocol with length-prefixed framing

**Date:** 2026-04-04

## What
Use Protocol Buffers for all node-to-node messages, framed as `[4B BE uint32 length][serialized Envelope]` over TLS.

## Why
- Compact binary format with strict schemas
- Good forward/backward compatibility as the protocol evolves (field numbering)
- Standard for P2P systems
- Alternatives considered:
  - Length-prefixed JSON — simpler to debug but larger on wire, no schema evolution guarantees
  - Custom binary framing — maximum control but high manual effort, harder to evolve
  - gRPC — adds HTTP/2 overhead, connection lifecycle complexity unnecessary for long-lived P2P connections

## How
- Single `Envelope` message with `oneof` payload for all message types
- Max message size: 4 MiB
- File data streamed in `FileChunk` messages (64 KiB per chunk)
- No gRPC — raw protobuf over TLS gives full control over connection lifecycle
