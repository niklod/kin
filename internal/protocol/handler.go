package protocol

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/niklod/kin/internal/transfer"
	"github.com/niklod/kin/kinpb"
)

// Conn is the interface the handler uses to communicate with a peer.
type Conn interface {
	Send(*kinpb.Envelope) error
	Recv() (*kinpb.Envelope, error)
	RemoteAddr() string
}

// Handler dispatches incoming protobuf messages for a single peer connection.
type Handler struct {
	sender *transfer.Sender
	logger *slog.Logger
}

// NewHandler creates a Handler backed by the given file sender.
func NewHandler(sender *transfer.Sender, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{sender: sender, logger: logger}
}

// Serve reads messages from conn in a loop and dispatches them until the
// connection closes or ctx is cancelled.
func (h *Handler) Serve(ctx context.Context, conn Conn) {
	for {
		if ctx.Err() != nil {
			return
		}
		env, err := conn.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			h.logger.Debug("recv error", "peer", conn.RemoteAddr(), "err", err)
			return
		}
		if err := h.dispatch(ctx, env, conn); err != nil {
			h.logger.Warn("dispatch error", "peer", conn.RemoteAddr(), "err", err)
		}
	}
}

func (h *Handler) dispatch(ctx context.Context, env *kinpb.Envelope, conn Conn) error {
	switch p := env.Payload.(type) {
	case *kinpb.Envelope_FileRequest:
		return h.handleFileRequest(ctx, p.FileRequest, conn)
	default:
		return fmt.Errorf("handler: unhandled message type %T", env.Payload)
	}
}

func (h *Handler) handleFileRequest(ctx context.Context, req *kinpb.FileRequest, conn Conn) error {
	if len(req.FileId) != 32 {
		return conn.Send(&kinpb.Envelope{Payload: &kinpb.Envelope_Error{
			Error: &kinpb.Error{Code: "bad_request", Message: "file_id must be 32 bytes"},
		}})
	}
	var fileID [32]byte
	copy(fileID[:], req.FileId)

	h.logger.Debug("file request", "peer", conn.RemoteAddr(), "file_id", fmt.Sprintf("%x", fileID[:8]))
	return h.sender.HandleRequest(ctx, fileID, conn)
}
