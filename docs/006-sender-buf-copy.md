# Decision 006: Copy chunk data before sending in file sender

**Date:** 2026-04-04

## What
`transfer.Sender.HandleRequest` copies each 64 KiB chunk into a fresh slice before placing it in the protobuf `FileChunk.Data` field.

## Why
When using an in-memory channel transport (tests, future in-process use), the protobuf Envelope is passed by pointer without serialisation. If the sender reuses its read buffer across iterations, the receiver may read from a slice that the sender is concurrently overwriting — a data race detected by `go test -race`.

In production over TLS, `proto.Marshal` serialises the data into a new byte slice before writing, so the race cannot occur. The copy is therefore a correctness fix for in-memory transports and a minor overhead (<1 allocation per chunk) for the real transport path.

## Alternative considered
Reinstate the race by documenting that `MsgWriter.Send` must fully consume `Data` before returning. Rejected: too fragile a contract to maintain across future transport implementations.
