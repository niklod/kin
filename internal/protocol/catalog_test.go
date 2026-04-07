package protocol_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/kin/internal/catalog"
	"github.com/niklod/kin/internal/protocol"
	"github.com/niklod/kin/internal/transfer"
	"github.com/niklod/kin/kinpb"
	"github.com/niklod/kin/testutil"
)

var (
	nodeA = [32]byte{0xAA}
	nodeB = [32]byte{0xBB}
)

type CatalogExchangeSuite struct {
	suite.Suite
	catA *catalog.Store
	catB *catalog.Store
}

func (s *CatalogExchangeSuite) SetupTest() {
	dirA := s.T().TempDir()
	dirB := s.T().TempDir()
	var err error
	s.catA, err = catalog.Open(filepath.Join(dirA, "catalog.db"), nodeA)
	s.Require().NoError(err)
	s.catB, err = catalog.Open(filepath.Join(dirB, "catalog.db"), nodeB)
	s.Require().NoError(err)
}

func (s *CatalogExchangeSuite) TearDownTest() {
	s.catA.Close()
	s.catB.Close()
}

func (s *CatalogExchangeSuite) TestCatalogOfferSentOnServe() {
	s.Require().NoError(s.catA.PutLocal(&catalog.Entry{
		FileID: [32]byte{1}, Name: "a.txt", Size: 10,
		ModTime: time.Now().UTC(), LocalPath: "/Kin/a.txt",
	}))

	idx := transfer.NewLocalIndex()
	sender := transfer.NewSender(idx)
	handler := protocol.NewHandler(sender, s.catA, nodeA, nil)

	serverConn, clientConn := testutil.NewMemConnPairWithIDs("127.0.0.1:0", 64, nodeA, nodeB)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handler.Serve(ctx, serverConn)

	env, err := clientConn.Recv()
	s.Require().NoError(err)

	offer := env.GetCatalogOffer()
	s.Require().NotNil(offer)
	s.Len(offer.Files, 1)
	s.Equal("a.txt", offer.Files[0].Name)
}

func (s *CatalogExchangeSuite) TestIncomingCatalogOfferSaved() {
	idx := transfer.NewLocalIndex()
	sender := transfer.NewSender(idx)
	handler := protocol.NewHandler(sender, s.catA, nodeA, nil)

	serverConn, clientConn := testutil.NewMemConnPairWithIDs("127.0.0.1:0", 64, nodeA, nodeB)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handler.Serve(ctx, serverConn)

	// Drain the initial catalog offer from A
	clientConn.Recv()

	// Send B's catalog to A
	clientConn.Send(&kinpb.Envelope{
		Payload: &kinpb.Envelope_CatalogOffer{
			CatalogOffer: &kinpb.CatalogOffer{
				Files: []*kinpb.CatalogFile{
					{
						FileId:      make([]byte, 32),
						Name:        "remote.txt",
						Size:        99,
						ModTimeUnix: time.Now().Unix(),
						OwnerNodeId: nodeB[:],
					},
				},
			},
		},
	})

	// Expect CatalogAck
	env, err := clientConn.Recv()
	s.Require().NoError(err)
	ack := env.GetCatalogAck()
	s.Require().NotNil(ack)
	s.Equal(uint32(1), ack.ReceivedCount)

	// Verify entry is in catalog A
	entries, err := s.catA.ListByOwner(nodeB)
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	s.Equal("remote.txt", entries[0].Name)
}

func (s *CatalogExchangeSuite) TestLoopPrevention() {
	s.Require().NoError(s.catA.PutLocal(&catalog.Entry{
		FileID: [32]byte{1}, Name: "local.txt", Size: 10,
		ModTime: time.Now().UTC(), LocalPath: "/Kin/local.txt",
	}))
	s.Require().NoError(s.catA.PutPeerEntries(nodeB, []*catalog.Entry{
		{FileID: [32]byte{2}, Name: "from_b.txt", Size: 20, ModTime: time.Now().UTC()},
	}))

	idx := transfer.NewLocalIndex()
	sender := transfer.NewSender(idx)
	handler := protocol.NewHandler(sender, s.catA, nodeA, nil)

	// Server sees peer as nodeB
	serverConn, clientConn := testutil.NewMemConnPairWithIDs("127.0.0.1:0", 64, nodeA, nodeB)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handler.Serve(ctx, serverConn)

	env, err := clientConn.Recv()
	s.Require().NoError(err)

	offer := env.GetCatalogOffer()
	s.Require().NotNil(offer)

	// Should only contain local.txt, NOT from_b.txt (loop prevention)
	s.Len(offer.Files, 1)
	s.Equal("local.txt", offer.Files[0].Name)
}

func (s *CatalogExchangeSuite) TestNilCatalogSkipsExchange() {
	idx := transfer.NewLocalIndex()
	sender := transfer.NewSender(idx)
	handler := protocol.NewHandler(sender, nil, nodeA, nil)

	serverConn, clientConn := testutil.NewMemConnPairWithIDs("127.0.0.1:0", 64, nodeA, nodeB)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handler.Serve(ctx, serverConn)

	// Send a catalog offer — should be handled gracefully (no panic)
	clientConn.Send(&kinpb.Envelope{
		Payload: &kinpb.Envelope_CatalogOffer{
			CatalogOffer: &kinpb.CatalogOffer{},
		},
	})

	// Close to end Serve
	clientConn.CloseRecv()
	serverConn.CloseRecv()
}

func TestCatalogExchangeSuite(t *testing.T) {
	suite.Run(t, new(CatalogExchangeSuite))
}
