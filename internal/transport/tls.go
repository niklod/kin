// Package transport provides QUIC-based peer-to-peer transport with Ed25519 identity.
//
// Trust model: InsecureSkipVerify=true on both sides. Peer identity is verified
// via a custom VerifyPeerCertificate callback that extracts the Ed25519 public key
// from the peer's self-signed cert and computes SHA-256 to match the expected NodeID.
package transport

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/niklod/kin/internal/identity"
)

const alpnKin = "kin/1"

// GenerateSelfSignedCert creates a TLS certificate from the given Ed25519 key pair.
// The certificate is self-signed and valid for 10 years.
func GenerateSelfSignedCert(priv ed25519.PrivateKey) (tls.Certificate, error) {
	pub := priv.Public().(ed25519.PublicKey)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "kin"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("load key pair: %w", err)
	}
	return cert, nil
}

// NodeIDFromCert extracts the Ed25519 public key from a peer's certificate
// and returns SHA-256(pubkey) as the NodeID.
func NodeIDFromCert(peerCerts []*x509.Certificate) ([32]byte, error) {
	if len(peerCerts) == 0 {
		return [32]byte{}, errors.New("transport: no peer certificates")
	}
	pub, ok := peerCerts[0].PublicKey.(ed25519.PublicKey)
	if !ok {
		return [32]byte{}, errors.New("transport: peer certificate is not Ed25519")
	}
	return sha256.Sum256(pub), nil
}

// certFromIdentity generates a self-signed TLS certificate from id's keypair.
// The op string is used to construct a descriptive error.
func certFromIdentity(id *identity.Identity, op string) (tls.Certificate, error) {
	cert, err := GenerateSelfSignedCert(id.PrivKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("%s: generate cert: %w", op, err)
	}
	return cert, nil
}

// serverTLSConfig returns a TLS config for the listening side.
// Peer certificate verification happens in VerifyPeerCertificate.
func serverTLSConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		ClientAuth:         tls.RequireAnyClientCert,
		InsecureSkipVerify: true, //nolint:gosec // intentional; peer identity checked via NodeID
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{alpnKin},
	}
}

// clientTLSConfig returns a TLS config for the dialing side that verifies
// the server's NodeID matches expectedNodeID.
func clientTLSConfig(cert tls.Certificate, expectedNodeID [32]byte) *tls.Config {
	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true, //nolint:gosec // intentional; peer identity checked below
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{alpnKin},
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			certs, err := parseCerts(rawCerts)
			if err != nil {
				return err
			}
			gotID, err := NodeIDFromCert(certs)
			if err != nil {
				return err
			}
			if gotID != expectedNodeID {
				return fmt.Errorf("transport: NodeID mismatch: got %x, want %x", gotID, expectedNodeID)
			}
			return nil
		},
	}
}

func parseCerts(rawCerts [][]byte) ([]*x509.Certificate, error) {
	certs := make([]*x509.Certificate, 0, len(rawCerts))
	for _, raw := range rawCerts {
		c, err := x509.ParseCertificate(raw)
		if err != nil {
			return nil, fmt.Errorf("transport: parse cert: %w", err)
		}
		certs = append(certs, c)
	}
	return certs, nil
}
