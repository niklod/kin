package daemon

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const maxIPCMsgSize = 4 << 20 // 4 MiB

// WriteJSON writes a length-prefixed JSON message to w.
func WriteJSON(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("ipc: marshal: %w", err)
	}
	if len(data) > maxIPCMsgSize {
		return fmt.Errorf("ipc: message too large (%d > %d)", len(data), maxIPCMsgSize)
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	if _, err := w.Write(header[:]); err != nil {
		return fmt.Errorf("ipc: write header: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("ipc: write body: %w", err)
	}
	return nil
}

// ReadJSON reads a length-prefixed JSON message from r into v.
func ReadJSON(r io.Reader, v any) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return fmt.Errorf("ipc: read header: %w", err)
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 {
		return nil
	}
	if int(size) > maxIPCMsgSize {
		return fmt.Errorf("ipc: message too large (%d > %d)", size, maxIPCMsgSize)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return fmt.Errorf("ipc: read body: %w", err)
	}
	if err := json.Unmarshal(buf, v); err != nil {
		return fmt.Errorf("ipc: unmarshal: %w", err)
	}
	return nil
}
