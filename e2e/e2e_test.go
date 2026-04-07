//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type E2ESuite struct {
	suite.Suite
	cluster *Cluster
}

func (s *E2ESuite) SetupSuite() {
	s.cluster = NewCluster(s.T())
}

func (s *E2ESuite) TearDownSuite() {
	s.cluster.Cleanup()
}

func TestE2ESuite(t *testing.T) {
	suite.Run(t, new(E2ESuite))
}
