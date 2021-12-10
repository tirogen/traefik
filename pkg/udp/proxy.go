package udp

import (
	"errors"
	"io"
	"net"

	"github.com/traefik/traefik/v2/pkg/log"
)

// Proxy is a reverse-proxy implementation of the Handler interface.
type Proxy struct {
	// TODO: maybe optimize by pre-resolving it at proxy creation time
	target net.Addr
}

// NewProxy creates a new Proxy.
func NewProxy(address string) (*Proxy, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	return &Proxy{target: addr}, nil
}

// ServeUDP implements the Handler interface.
func (p *Proxy) ServeUDP(conn *Conn) {
	log.WithoutContext().Debugf("Handling connection from %s", conn.rAddr)

	// needed because of e.g. server.trackedConnection
	defer conn.Close()

	errChan := make(chan error)
	go connCopy(conn, conn.lConn, errChan)

	//go connCopy(conn.lConn, conn, errChan)
	go func() {
		for {
			buf := make([]byte, maxDatagramSize)
			n, err := conn.Read(buf)
			if err != nil {
				// FIXME really ?
				if errors.Is(err, io.EOF) {
					return
				}
				log.WithoutContext().Errorf("FIXME: %v", err)
				return
			}

			_, err = conn.lConn.WriteTo(buf[:n], p.target)
			if err != nil {
				log.WithoutContext().Errorf("FIXME: %v", err)
				return
			}
		}
	}()

	err := <-errChan
	if err != nil {
		log.WithoutContext().Errorf("Error while serving UDP: %v", err)
	}

	//<-errChan
}

func connCopy(dst io.WriteCloser, src io.Reader, errCh chan error) {
	// The buffer is initialized to the maximum UDP datagram size,
	// to make sure that the whole UDP datagram is read or written atomically (no data is discarded).
	buffer := make([]byte, maxDatagramSize)

	_, err := io.CopyBuffer(dst, src, buffer)
	errCh <- err

	if err := dst.Close(); err != nil {
		log.WithoutContext().Debugf("Error while terminating connection: %v", err)
	}
}
