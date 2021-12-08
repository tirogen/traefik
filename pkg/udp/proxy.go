package udp

import (
	"io"
	"net"
	"sync"

	"github.com/traefik/traefik/v2/pkg/log"
)

// Proxy is a reverse-proxy implementation of the Handler interface.
type Proxy struct {
	// TODO: maybe optimize by pre-resolving it at proxy creation time
	target string

	lock sync.RWMutex
	// conns stores the active/idle UDP conns for each client/remoteAddr.
	conns map[string]*Conn
}

// NewProxy creates a new Proxy.
func NewProxy(address string) (*Proxy, error) {
	return &Proxy{
		target: address,
		conns:  make(map[string]*Conn),
	}, nil
}

// ServeUDP implements the Handler interface.
func (p *Proxy) ServeUDP(conn *Conn) {
	conn.target = p.target

	raddr := conn.rAddr.String()

	p.lock.RLock()
	// FIXME think about mutex and opportunity to check if the conn is closed
	if c, ok := p.conns[raddr]; ok && !c.isClosed {
		conn.Close()
		return
	}
	p.lock.RUnlock()

	conn.StartCh <- struct{}{}

	log.WithoutContext().Debugf("Handling connection from %s", conn.rAddr)

	p.lock.Lock()
	p.conns[raddr] = conn
	p.lock.Unlock()

	connBackend, err := net.Dial("udp", p.target)
	if err != nil {
		log.WithoutContext().Errorf("Error while connecting to backend: %v", err)
		return
	}

	// maybe not needed, but just in case
	defer connBackend.Close()

	errChan := make(chan error)
	go connCopy(conn, connBackend, errChan)
	go connCopy(connBackend, conn, errChan)

	err = <-errChan
	if err != nil {
		log.WithoutContext().Errorf("Error while serving UDP: %v", err)
	}

	<-errChan

	p.lock.Lock()
	// needed because of e.g. server.trackedConnection
	conn.Close()
	delete(p.conns, raddr)
	p.lock.Unlock()
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
