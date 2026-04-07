package download_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/kin/internal/catalog"
	"github.com/niklod/kin/internal/download"
	"github.com/niklod/kin/internal/transfer"
	"github.com/niklod/kin/kinpb"
	"github.com/niklod/kin/testutil"
)

var (
	selfID = [32]byte{0xAA}
	peerA  = [32]byte{0xBB}
	peerB  = [32]byte{0xCC}
)

type stubCatalog struct {
	owners  map[[32]byte][]catalog.OwnerHint
	entries map[[64]byte]*catalog.Entry
}

func newStubCatalog() *stubCatalog {
	return &stubCatalog{
		owners:  make(map[[32]byte][]catalog.OwnerHint),
		entries: make(map[[64]byte]*catalog.Entry),
	}
}

func (s *stubCatalog) GetOwners(fileID [32]byte) ([]catalog.OwnerHint, error) {
	return s.owners[fileID], nil
}

func (s *stubCatalog) GetEntry(ownerNodeID, fileID [32]byte) (*catalog.Entry, error) {
	var key [64]byte
	copy(key[:32], ownerNodeID[:])
	copy(key[32:], fileID[:])
	return s.entries[key], nil
}

func (s *stubCatalog) addOwner(fileID [32]byte, nodeID [32]byte) {
	s.owners[fileID] = append(s.owners[fileID], catalog.OwnerHint{
		NodeID: nodeID, SeenAt: time.Now().UTC(),
	})
}

func (s *stubCatalog) addEntry(ownerNodeID, fileID [32]byte, name string) {
	var key [64]byte
	copy(key[:32], ownerNodeID[:])
	copy(key[32:], fileID[:])
	s.entries[key] = &catalog.Entry{
		FileID:      fileID,
		Name:        name,
		OwnerNodeID: ownerNodeID,
	}
}

type stubDialer struct {
	conns map[[32]byte]transfer.MsgReadWriter
	err   error
}

func (s *stubDialer) Dial(_ context.Context, peerNodeID [32]byte) (transfer.MsgReadWriter, error) {
	if s.err != nil {
		return nil, s.err
	}
	conn, ok := s.conns[peerNodeID]
	if !ok {
		return nil, errors.New("peer unreachable")
	}
	return conn, nil
}

type stubIndex struct {
	added map[string]bool
}

func (s *stubIndex) Add(path string) error {
	s.added[path] = true
	return nil
}

type DownloadSuite struct {
	suite.Suite
	sharedDir string
	cat       *stubCatalog
	dialer    *stubDialer
	index     *stubIndex
}

func (s *DownloadSuite) SetupTest() {
	s.sharedDir = s.T().TempDir()
	s.cat = newStubCatalog()
	s.dialer = &stubDialer{conns: make(map[[32]byte]transfer.MsgReadWriter)}
	s.index = &stubIndex{added: make(map[string]bool)}
}

func (s *DownloadSuite) TestDownload_Success() {
	content := "hello world"
	fileID := sha256.Sum256([]byte(content))
	s.cat.addOwner(fileID, peerA)
	s.cat.addEntry(peerA, fileID, "hello.txt")

	serverConn, clientConn := testutil.NewMemConnPair("test", 64)
	s.dialer.conns[peerA] = clientConn

	// Simulate sender in background
	go func() {
		env, _ := serverConn.Recv()
		req := env.GetFileRequest()
		serverConn.Send(&kinpb.Envelope{
			Payload: &kinpb.Envelope_FileResponse{
				FileResponse: &kinpb.FileResponse{FileId: req.FileId, Size: int64(len(content))},
			},
		})
		serverConn.Send(&kinpb.Envelope{
			Payload: &kinpb.Envelope_FileChunk{
				FileChunk: &kinpb.FileChunk{FileId: req.FileId, Data: []byte(content), Eof: true},
			},
		})
	}()

	dl := download.New(s.cat, s.dialer, s.index, s.sharedDir, selfID, nil)

	path, err := dl.Download(context.Background(), fileID)

	s.Require().NoError(err)
	s.Require().NotEmpty(path)

	data, err := os.ReadFile(path)
	s.Require().NoError(err)
	s.Equal(content, string(data))
	s.Equal("hello.txt", filepath.Base(path))
	s.True(s.index.added[path])
}

func (s *DownloadSuite) TestDownload_FallbackToSecondOwner() {
	content := "fallback test"
	fileID := sha256.Sum256([]byte(content))
	s.cat.addOwner(fileID, peerA)
	s.cat.addOwner(fileID, peerB)
	s.cat.addEntry(peerB, fileID, "fallback.txt")

	// peerA is unreachable, peerB serves the file
	serverConn, clientConn := testutil.NewMemConnPair("test", 64)
	s.dialer.conns[peerB] = clientConn

	go func() {
		env, _ := serverConn.Recv()
		req := env.GetFileRequest()
		serverConn.Send(&kinpb.Envelope{
			Payload: &kinpb.Envelope_FileResponse{
				FileResponse: &kinpb.FileResponse{FileId: req.FileId, Size: int64(len(content))},
			},
		})
		serverConn.Send(&kinpb.Envelope{
			Payload: &kinpb.Envelope_FileChunk{
				FileChunk: &kinpb.FileChunk{FileId: req.FileId, Data: []byte(content), Eof: true},
			},
		})
	}()

	dl := download.New(s.cat, s.dialer, s.index, s.sharedDir, selfID, nil)

	path, err := dl.Download(context.Background(), fileID)

	s.Require().NoError(err)
	data, _ := os.ReadFile(path)
	s.Equal(content, string(data))
}

func (s *DownloadSuite) TestDownload_AllOwnersOffline() {
	fileID := sha256.Sum256([]byte("unreachable"))
	s.cat.addOwner(fileID, peerA)

	dl := download.New(s.cat, s.dialer, s.index, s.sharedDir, selfID, nil)

	_, err := dl.Download(context.Background(), fileID)

	s.Require().Error(err)
	s.True(errors.Is(err, download.ErrNoAvailableOwner))
}

func (s *DownloadSuite) TestDownload_NoOwners() {
	fileID := [32]byte{0xFF}

	dl := download.New(s.cat, s.dialer, s.index, s.sharedDir, selfID, nil)

	_, err := dl.Download(context.Background(), fileID)

	s.Require().Error(err)
	s.True(errors.Is(err, download.ErrNoAvailableOwner))
}

func (s *DownloadSuite) TestDownload_SelfExcluded() {
	fileID := sha256.Sum256([]byte("self"))
	s.cat.addOwner(fileID, selfID)

	dl := download.New(s.cat, s.dialer, s.index, s.sharedDir, selfID, nil)

	_, err := dl.Download(context.Background(), fileID)

	s.Require().Error(err)
	s.True(errors.Is(err, download.ErrNoAvailableOwner))
}

func TestDownloadSuite(t *testing.T) {
	suite.Run(t, new(DownloadSuite))
}
