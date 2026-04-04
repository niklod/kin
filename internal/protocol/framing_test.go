package protocol_test

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/niklod/kin/internal/protocol"
	"github.com/niklod/kin/kinpb"
)

func TestWriteRead_RoundTrip(t *testing.T) {
	env := &kinpb.Envelope{Payload: &kinpb.Envelope_Error{
		Error: &kinpb.Error{Code: "ok", Message: "test message"},
	}}

	var buf bytes.Buffer
	if err := protocol.WriteMsg(&buf, env); err != nil {
		t.Fatalf("WriteMsg: %v", err)
	}
	got, err := protocol.ReadMsg(&buf)
	if err != nil {
		t.Fatalf("ReadMsg: %v", err)
	}
	if got.GetError().GetCode() != "ok" {
		t.Errorf("code = %q, want %q", got.GetError().GetCode(), "ok")
	}
}

func TestReadMsg_EOF(t *testing.T) {
	_, err := protocol.ReadMsg(bytes.NewReader(nil))
	if err != io.EOF {
		t.Errorf("empty reader: got %v, want io.EOF", err)
	}
}

func TestReadMsg_OversizedRejected(t *testing.T) {
	var buf bytes.Buffer
	// Write a length header claiming 5 MiB.
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], 5<<20)
	buf.Write(header[:])

	_, err := protocol.ReadMsg(&buf)
	if err == nil {
		t.Fatal("oversized message: expected error, got nil")
	}
}

func TestWriteMsg_MultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	for i := 0; i < 5; i++ {
		env := &kinpb.Envelope{Payload: &kinpb.Envelope_Error{
			Error: &kinpb.Error{Code: "msg"},
		}}
		if err := protocol.WriteMsg(&buf, env); err != nil {
			t.Fatalf("WriteMsg %d: %v", i, err)
		}
	}
	for i := 0; i < 5; i++ {
		_, err := protocol.ReadMsg(&buf)
		if err != nil {
			t.Fatalf("ReadMsg %d: %v", i, err)
		}
	}
}
