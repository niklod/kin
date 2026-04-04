package transfer

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/niklod/kin/kinpb"
)

const chunkSize = 64 << 10 // 64 KiB

// Sender serves files from a LocalIndex over a message-oriented connection.
type Sender struct {
	idx *LocalIndex
}

// NewSender creates a Sender backed by the given index.
func NewSender(idx *LocalIndex) *Sender {
	return &Sender{idx: idx}
}

// MsgWriter is implemented by transport.Conn.
type MsgWriter interface {
	Send(env *kinpb.Envelope) error
}

func sendNotFound(w MsgWriter, fileID [32]byte) error {
	return w.Send(&kinpb.Envelope{Payload: &kinpb.Envelope_FileResponse{
		FileResponse: &kinpb.FileResponse{FileId: fileID[:], NotFound: true},
	}})
}

// HandleRequest looks up fileID in the index and streams the file to w.
// Sends FileResponse (with not_found=true if missing) then FileChunk stream.
func (s *Sender) HandleRequest(ctx context.Context, fileID [32]byte, w MsgWriter) error {
	path, ok := s.idx.Lookup(fileID)
	if !ok {
		return sendNotFound(w, fileID)
	}

	info, err := os.Stat(path)
	if err != nil {
		return sendNotFound(w, fileID)
	}

	if err := w.Send(&kinpb.Envelope{Payload: &kinpb.Envelope_FileResponse{
		FileResponse: &kinpb.FileResponse{
			FileId: fileID[:],
			Size:   info.Size(),
		},
	}}); err != nil {
		return fmt.Errorf("send file response: %w", err)
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	buf := make([]byte, chunkSize)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, readErr := f.Read(buf)
		eof := readErr == io.EOF
		if n > 0 || eof {
			// Copy data before sending: in-memory transports don't marshal,
			// so the buf slice must not be reused while the receiver holds a reference.
			data := make([]byte, n)
			copy(data, buf[:n])
			chunk := &kinpb.Envelope{Payload: &kinpb.Envelope_FileChunk{
				FileChunk: &kinpb.FileChunk{
					FileId: fileID[:],
					Data:   data,
					Eof:    eof,
				},
			}}
			if err := w.Send(chunk); err != nil {
				return fmt.Errorf("send chunk: %w", err)
			}
		}
		if eof {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read file: %w", readErr)
		}
	}
}
