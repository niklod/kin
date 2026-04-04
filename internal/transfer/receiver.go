package transfer

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/niklod/kin/kinpb"
)

// ErrHashMismatch is returned when the received file's SHA-256 doesn't match the requested fileID.
var ErrHashMismatch = errors.New("transfer: hash mismatch after download")

// ErrFileNotFound is returned when the sender reports the file is not available.
var ErrFileNotFound = errors.New("transfer: file not found on sender")

// MsgReader is implemented by transport.Conn.
type MsgReader interface {
	Recv() (*kinpb.Envelope, error)
}

// MsgReadWriter combines MsgReader and MsgWriter.
type MsgReadWriter interface {
	MsgReader
	MsgWriter
}

// RequestFile sends a FileRequest to rw and streams the response to destDir.
// Returns the path of the downloaded file on success.
// The file is written to a temp file first; on hash verification success it is moved to destDir/<hex(fileID)>.
func RequestFile(ctx context.Context, fileID [32]byte, destDir string, rw MsgReadWriter) (string, error) {
	if err := rw.Send(&kinpb.Envelope{Payload: &kinpb.Envelope_FileRequest{
		FileRequest: &kinpb.FileRequest{FileId: fileID[:]},
	}}); err != nil {
		return "", fmt.Errorf("send file request: %w", err)
	}

	// Expect FileResponse first.
	env, err := rw.Recv()
	if err != nil {
		return "", fmt.Errorf("recv file response: %w", err)
	}
	resp := env.GetFileResponse()
	if resp == nil {
		return "", fmt.Errorf("receiver: expected FileResponse, got %T", env.Payload)
	}
	if resp.NotFound {
		return "", ErrFileNotFound
	}

	// Stream chunks to a temp file.
	tmp, err := os.CreateTemp(destDir, ".kin-tmp-")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	// Defer removes the temp file on any error path; on success the file is renamed away.
	closed := false
	defer func() {
		if !closed {
			tmp.Close()
		}
		os.Remove(tmpPath) // no-op after successful rename
	}()

	h := sha256.New()
	w := io.MultiWriter(tmp, h)

	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		env, err := rw.Recv()
		if err != nil {
			return "", fmt.Errorf("recv chunk: %w", err)
		}
		chunk := env.GetFileChunk()
		if chunk == nil {
			return "", fmt.Errorf("receiver: expected FileChunk, got %T", env.Payload)
		}
		if len(chunk.Data) > 0 {
			if _, err := w.Write(chunk.Data); err != nil {
				return "", fmt.Errorf("write chunk: %w", err)
			}
		}
		if chunk.Eof {
			break
		}
	}

	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}
	closed = true

	var got [32]byte
	copy(got[:], h.Sum(nil))
	if got != fileID {
		return "", ErrHashMismatch
	}

	destPath := filepath.Join(destDir, fmt.Sprintf("%x", fileID))
	if err := os.Rename(tmpPath, destPath); err != nil {
		return "", fmt.Errorf("rename temp file: %w", err)
	}
	return destPath, nil
}
