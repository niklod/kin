// Package protocol implements length-prefixed protobuf framing for peer connections.
//
// Wire format: [4 bytes BE uint32 length][protobuf Envelope payload]
// Maximum message size: 4 MiB.
package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	"github.com/niklod/kin/kinpb"
)

const maxMsgSize = 4 << 20 // 4 MiB

// WriteMsg serialises env and writes it with a 4-byte length prefix.
func WriteMsg(w io.Writer, env *kinpb.Envelope) error {
	data, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("framing: marshal: %w", err)
	}
	if len(data) > maxMsgSize {
		return fmt.Errorf("framing: message too large (%d > %d)", len(data), maxMsgSize)
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	if _, err := w.Write(header[:]); err != nil {
		return fmt.Errorf("framing: write header: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("framing: write body: %w", err)
	}
	return nil
}

// ReadMsg reads a length-prefixed message and deserialises it into an Envelope.
func ReadMsg(r io.Reader) (*kinpb.Envelope, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("framing: read header: %w", err)
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 {
		return &kinpb.Envelope{}, nil
	}
	if int(size) > maxMsgSize {
		return nil, fmt.Errorf("framing: message too large (%d > %d)", size, maxMsgSize)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("framing: read body: %w", err)
	}
	var env kinpb.Envelope
	if err := proto.Unmarshal(buf, &env); err != nil {
		return nil, fmt.Errorf("framing: unmarshal: %w", err)
	}
	return &env, nil
}
