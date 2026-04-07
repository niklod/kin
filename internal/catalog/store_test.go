package catalog_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/kin/internal/catalog"
)

var (
	selfID = [32]byte{0xAA}
	peerA  = [32]byte{0xBB}
	peerB  = [32]byte{0xCC}

	fileID1 = [32]byte{1}
	fileID2 = [32]byte{2}
	fileID3 = [32]byte{3}
)

type StoreSuite struct {
	suite.Suite
	dir   string
	store *catalog.Store
}

func (s *StoreSuite) SetupTest() {
	s.dir = s.T().TempDir()
	store, err := catalog.Open(filepath.Join(s.dir, "catalog.db"), selfID)
	s.Require().NoError(err)
	s.store = store
}

func (s *StoreSuite) TearDownTest() {
	s.store.Close()
}

func (s *StoreSuite) TestPutLocal_NewEntry() {
	entry := &catalog.Entry{
		FileID:    fileID1,
		Name:      "hello.txt",
		Size:      42,
		ModTime:   time.Now().UTC().Truncate(time.Second),
		LocalPath: "/home/user/Kin/hello.txt",
	}

	err := s.store.PutLocal(entry)

	s.Require().NoError(err)
	entries, err := s.store.ListLocal()
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	s.Equal("hello.txt", entries[0].Name)
	s.Equal(int64(42), entries[0].Size)
	s.Equal(selfID, entries[0].OwnerNodeID)
	s.Equal(fileID1, entries[0].FileID)
}

func (s *StoreSuite) TestPutLocal_UpdateExisting() {
	s.Require().NoError(s.store.PutLocal(&catalog.Entry{
		FileID:    fileID1,
		Name:      "hello.txt",
		Size:      42,
		LocalPath: "/home/user/Kin/hello.txt",
	}))

	s.Require().NoError(s.store.PutLocal(&catalog.Entry{
		FileID:    fileID2,
		Name:      "hello.txt",
		Size:      100,
		LocalPath: "/home/user/Kin/hello.txt",
	}))

	entries, err := s.store.ListLocal()
	s.Require().NoError(err)
	s.Require().Len(entries, 2)
}

func (s *StoreSuite) TestDeleteLocal_Tombstone() {
	s.Require().NoError(s.store.PutLocal(&catalog.Entry{
		FileID:    fileID1,
		Name:      "hello.txt",
		Size:      42,
		LocalPath: "/home/user/Kin/hello.txt",
	}))

	err := s.store.DeleteLocal(fileID1)

	s.Require().NoError(err)
	entries, err := s.store.ListLocal()
	s.Require().NoError(err)
	s.Empty(entries)
}

func (s *StoreSuite) TestDeleteLocal_NonExistent() {
	err := s.store.DeleteLocal(fileID1)
	s.Require().NoError(err)
}

func (s *StoreSuite) TestLookupLocalByPath() {
	s.Require().NoError(s.store.PutLocal(&catalog.Entry{
		FileID:    fileID1,
		Name:      "hello.txt",
		Size:      42,
		LocalPath: "/home/user/Kin/hello.txt",
	}))

	entry, err := s.store.LookupLocalByPath("/home/user/Kin/hello.txt")

	s.Require().NoError(err)
	s.Require().NotNil(entry)
	s.Equal(fileID1, entry.FileID)
}

func (s *StoreSuite) TestLookupLocalByPath_NotFound() {
	entry, err := s.store.LookupLocalByPath("/nonexistent")

	s.Require().NoError(err)
	s.Nil(entry)
}

func (s *StoreSuite) TestPutPeerEntries_BulkInsert() {
	entries := []*catalog.Entry{
		{FileID: fileID1, Name: "a.txt", Size: 10},
		{FileID: fileID2, Name: "b.txt", Size: 20},
		{FileID: fileID3, Name: "c.txt", Size: 30},
	}

	err := s.store.PutPeerEntries(peerA, entries)

	s.Require().NoError(err)
	got, err := s.store.ListByOwner(peerA)
	s.Require().NoError(err)
	s.Len(got, 3)
}

func (s *StoreSuite) TestPutPeerEntries_ReplaceOnReconnect() {
	first := []*catalog.Entry{
		{FileID: fileID1, Name: "a.txt", Size: 10},
		{FileID: fileID2, Name: "b.txt", Size: 20},
	}
	s.Require().NoError(s.store.PutPeerEntries(peerA, first))

	second := []*catalog.Entry{
		{FileID: fileID3, Name: "c.txt", Size: 30},
	}
	err := s.store.PutPeerEntries(peerA, second)

	s.Require().NoError(err)
	got, err := s.store.ListByOwner(peerA)
	s.Require().NoError(err)
	s.Len(got, 1)
	s.Equal("c.txt", got[0].Name)
}

func (s *StoreSuite) TestRemovePeer() {
	s.Require().NoError(s.store.PutPeerEntries(peerA, []*catalog.Entry{
		{FileID: fileID1, Name: "a.txt", Size: 10},
	}))

	err := s.store.RemovePeer(peerA)

	s.Require().NoError(err)
	got, err := s.store.ListByOwner(peerA)
	s.Require().NoError(err)
	s.Empty(got)
}

func (s *StoreSuite) TestGetOwners_MultipleOwners() {
	s.Require().NoError(s.store.PutLocal(&catalog.Entry{
		FileID: fileID1, Name: "shared.txt", Size: 42, LocalPath: "/home/user/Kin/shared.txt",
	}))
	s.Require().NoError(s.store.PutPeerEntries(peerA, []*catalog.Entry{
		{FileID: fileID1, Name: "shared.txt", Size: 42},
	}))
	s.Require().NoError(s.store.PutPeerEntries(peerB, []*catalog.Entry{
		{FileID: fileID1, Name: "shared.txt", Size: 42},
	}))

	owners, err := s.store.GetOwners(fileID1)

	s.Require().NoError(err)
	s.Len(owners, 3)
	nodeIDs := make(map[[32]byte]bool)
	for _, o := range owners {
		nodeIDs[o.NodeID] = true
	}
	s.True(nodeIDs[selfID])
	s.True(nodeIDs[peerA])
	s.True(nodeIDs[peerB])
}

func (s *StoreSuite) TestGetOwners_RemovedAfterDelete() {
	s.Require().NoError(s.store.PutLocal(&catalog.Entry{
		FileID: fileID1, Name: "hello.txt", Size: 42, LocalPath: "/home/user/Kin/hello.txt",
	}))

	s.Require().NoError(s.store.DeleteLocal(fileID1))

	owners, err := s.store.GetOwners(fileID1)
	s.Require().NoError(err)
	s.Empty(owners)
}

func (s *StoreSuite) TestGetEntry() {
	s.Require().NoError(s.store.PutLocal(&catalog.Entry{
		FileID: fileID1, Name: "hello.txt", Size: 42, LocalPath: "/home/user/Kin/hello.txt",
	}))

	entry, err := s.store.GetEntry(selfID, fileID1)

	s.Require().NoError(err)
	s.Require().NotNil(entry)
	s.Equal("hello.txt", entry.Name)
}

func (s *StoreSuite) TestGetEntry_NotFound() {
	entry, err := s.store.GetEntry(selfID, fileID1)

	s.Require().NoError(err)
	s.Nil(entry)
}

func (s *StoreSuite) TestListForPeer_ExcludesTarget() {
	s.Require().NoError(s.store.PutLocal(&catalog.Entry{
		FileID: fileID1, Name: "local.txt", Size: 10, LocalPath: "/home/user/Kin/local.txt",
	}))
	s.Require().NoError(s.store.PutPeerEntries(peerA, []*catalog.Entry{
		{FileID: fileID2, Name: "peerA.txt", Size: 20},
	}))
	s.Require().NoError(s.store.PutPeerEntries(peerB, []*catalog.Entry{
		{FileID: fileID3, Name: "peerB.txt", Size: 30},
	}))

	entries, err := s.store.ListForPeer(peerA)

	s.Require().NoError(err)
	s.Len(entries, 2)
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	s.True(names["local.txt"])
	s.True(names["peerB.txt"])
	s.False(names["peerA.txt"])
}

func (s *StoreSuite) TestListAll() {
	s.Require().NoError(s.store.PutLocal(&catalog.Entry{
		FileID: fileID1, Name: "local.txt", Size: 10, LocalPath: "/home/user/Kin/local.txt",
	}))
	s.Require().NoError(s.store.PutPeerEntries(peerA, []*catalog.Entry{
		{FileID: fileID2, Name: "remote.txt", Size: 20},
	}))

	entries, err := s.store.ListAll()

	s.Require().NoError(err)
	s.Len(entries, 2)
}

func (s *StoreSuite) TestPersistence() {
	path := filepath.Join(s.dir, "persist.db")
	s1, err := catalog.Open(path, selfID)
	s.Require().NoError(err)
	s.Require().NoError(s1.PutLocal(&catalog.Entry{
		FileID: fileID1, Name: "hello.txt", Size: 42, LocalPath: "/home/user/Kin/hello.txt",
	}))
	s1.Close()

	s2, err := catalog.Open(path, selfID)
	s.Require().NoError(err)
	defer s2.Close()

	entries, err := s2.ListLocal()
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	s.Equal("hello.txt", entries[0].Name)
}

func TestStoreSuite(t *testing.T) {
	suite.Run(t, new(StoreSuite))
}
