package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/niklod/kin/internal/identity"
)

// Listener is a QUIC endpoint that accepts inbound connections and can also
// initiate outbound connections from the same UDP socket. Sharing the socket
// is required for UDP NAT hole punching: the NAT mapping is keyed on
// (srcIP:srcPort, dstIP:dstPort), so the dial must originate from the same
// port that sent the prime packets.
type Listener struct {
	udpConn    *net.UDPConn    // kept for LocalAddr(); all I/O goes through qTransport
	qTransport *quic.Transport
	qListener  *quic.Listener
	cert       tls.Certificate // pre-generated once; reused for all outbound dials
	port       uint16
}

// Listen binds a UDP socket on addr and starts a QUIC listener.
func Listen(addr string, id *identity.Identity) (*Listener, error) {
	cert, err := certFromIdentity(id, "listen")
	if err != nil {
		return nil, err
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", addr, err)
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("listen udp %s: %w", addr, err)
	}

	qt := &quic.Transport{Conn: udpConn}
	qLn, err := qt.Listen(serverTLSConfig(cert), defaultQUICConfig())
	if err != nil {
		_ = qt.Close()
		return nil, fmt.Errorf("quic listen: %w", err)
	}

	actualPort := uint16(udpConn.LocalAddr().(*net.UDPAddr).Port)
	return &Listener{
		udpConn:    udpConn,
		qTransport: qt,
		qListener:  qLn,
		cert:       cert,
		port:       actualPort,
	}, nil
}

// Accept waits for an incoming QUIC connection and its first stream,
// returning an authenticated Conn and the peer's NodeID.
func (l *Listener) Accept() (*Conn, [32]byte, error) {
	qConn, err := l.qListener.Accept(context.Background())
	if err != nil {
		return nil, [32]byte{}, err
	}

	stream, err := qConn.AcceptStream(context.Background())
	if err != nil {
		_ = qConn.CloseWithError(0, "accept-stream")
		return nil, [32]byte{}, fmt.Errorf("accept: accept stream: %w", err)
	}

	if err := consumeFramingPrime(stream); err != nil {
		_ = qConn.CloseWithError(0, "prime")
		return nil, [32]byte{}, fmt.Errorf("accept: framing prime: %w", err)
	}

	peerNodeID, err := NodeIDFromCert(qConn.ConnectionState().TLS.PeerCertificates)
	if err != nil {
		_ = qConn.CloseWithError(0, "peer-id")
		return nil, [32]byte{}, fmt.Errorf("accept: extract peer NodeID: %w", err)
	}

	return newConn(qConn, stream, peerNodeID), peerNodeID, nil
}

// Dial opens an outbound QUIC connection to peerAddr using this Listener's UDP
// socket (same port as the listener), verifies the peer's NodeID, and returns
// an authenticated Conn. Using the listener's shared socket is essential for
// NAT hole punching.
func (l *Listener) Dial(ctx context.Context, peerAddr string, peerNodeID [32]byte) (*Conn, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", peerAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", peerAddr, err)
	}

	qConn, err := l.qTransport.Dial(ctx, udpAddr, clientTLSConfig(l.cert, peerNodeID), defaultQUICConfig())
	if err != nil {
		return nil, fmt.Errorf("quic dial %s: %w", peerAddr, err)
	}

	stream, err := openDialStream(ctx, qConn)
	if err != nil {
		return nil, err
	}

	return newConn(qConn, stream, peerNodeID), nil
}

// Prime sends a small UDP datagram to peerAddr via the listener's transport to
// open a NAT mapping on the local router. The datagram is intentionally not a
// valid QUIC packet; the peer's QUIC stack silently discards it.
func (l *Listener) Prime(peerAddr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", peerAddr)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", peerAddr, err)
	}
	if _, err = l.qTransport.WriteTo([]byte{0}, udpAddr); err != nil {
		return fmt.Errorf("prime write: %w", err)
	}
	return nil
}

// Addr returns the listener's local UDP address.
func (l *Listener) Addr() net.Addr {
	return l.udpConn.LocalAddr()
}

// Port returns the listener's local UDP port number.
func (l *Listener) Port() uint16 {
	return l.port
}

// Close stops the listener and shuts down the underlying QUIC transport.
// Connections already accepted are not affected until their own Close is called.
func (l *Listener) Close() error {
	return errors.Join(l.qListener.Close(), l.qTransport.Close())
}

func defaultQUICConfig() *quic.Config {
	return &quic.Config{
		MaxIdleTimeout:       30 * time.Second,
		KeepAlivePeriod:      15 * time.Second,
		HandshakeIdleTimeout: 5 * time.Second,
	}
}

// writeFramingPrime writes a 4-byte zero header to the stream. quic-go only
// surfaces a client-opened stream to the server once the first byte arrives;
// without this the server's AcceptStream would block indefinitely waiting for
// the application to send its first real message.
func writeFramingPrime(w io.Writer) error {
	_, err := w.Write([]byte{0, 0, 0, 0})
	return err
}

// consumeFramingPrime reads and discards the 4-byte framing prime written by
// the dialer. It must be called once in Accept immediately after AcceptStream.
func consumeFramingPrime(r io.Reader) error {
	var buf [4]byte
	_, err := io.ReadFull(r, buf[:])
	return err
}

// openDialStream opens a QUIC stream on qConn and writes the framing prime.
// On error it sends CONNECTION_CLOSE on qConn; the caller handles any
// additional cleanup (e.g. closing an owned transport).
func openDialStream(ctx context.Context, qConn *quic.Conn) (*quic.Stream, error) {
	stream, err := qConn.OpenStreamSync(ctx)
	if err != nil {
		_ = qConn.CloseWithError(0, "open-stream")
		return nil, fmt.Errorf("open stream: %w", err)
	}
	if err := writeFramingPrime(stream); err != nil {
		_ = qConn.CloseWithError(0, "prime")
		return nil, fmt.Errorf("write framing prime: %w", err)
	}
	return stream, nil
}
