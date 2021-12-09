package udp

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/traefik/traefik/v2/pkg/log"
)

// maxDatagramSize is the maximum size of a UDP datagram.
const maxDatagramSize = 65535

const closeRetryInterval = 500 * time.Millisecond

var errClosedListener = errors.New("udp: listener closed")

// Listener augments a session-oriented Listener over a UDP PacketConn.
type Listener struct {
	pConn *net.UDPConn

	muBackendsConns sync.RWMutex
	// backendsConns holds open connections to communicate with backends,
	// for each remote address.
	backendsConns map[string]*backendsConn

	muConns sync.RWMutex
	conns   map[string]*Conn
	// accepting signifies whether the listener is still accepting new sessions.
	// It also serves as a sentinel for Shutdown to be idempotent.
	accepting bool

	acceptCh chan *Conn // no need for a Once, already indirectly guarded by accepting.

	// timeout defines how long to wait on an idle session,
	// before releasing its related resources.
	timeout  time.Duration
	requests int
}

// Listen creates a new listener.
func Listen(network string, laddr *net.UDPAddr, timeout time.Duration, requests int) (*Listener, error) {
	if timeout <= 0 {
		return nil, errors.New("timeout should be greater than zero")
	}

	conn, err := net.ListenUDP(network, laddr)
	if err != nil {
		return nil, err
	}

	l := &Listener{
		pConn:         conn,
		backendsConns: make(map[string]*backendsConn),
		acceptCh:      make(chan *Conn),
		conns:         make(map[string]*Conn),
		accepting:     true,
		timeout:       timeout,
		requests:      requests,
	}

	go l.readLoop()

	return l, nil
}

// Accept waits for and returns the next connection to the listener.
func (l *Listener) Accept() (*Conn, error) {
	c := <-l.acceptCh
	if c == nil {
		// l.acceptCh got closed
		return nil, errClosedListener
	}

	return c, nil
}

// Addr returns the listener's network address.
func (l *Listener) Addr() net.Addr {
	return l.pConn.LocalAddr()
}

// Close closes the listener.
// It is like Shutdown with a zero graceTimeout.
func (l *Listener) Close() error {
	return l.Shutdown(0)
}

// close should not be called more than once.
func (l *Listener) close() error {
	l.muConns.Lock()
	defer l.muConns.Unlock()
	err := l.pConn.Close()
	for k, v := range l.conns {
		v.close()
		delete(l.conns, k)
	}
	close(l.acceptCh)
	return err
}

// Shutdown closes the listener.
// It immediately stops accepting new sessions,
// and it waits for all existing sessions to terminate,
// and a maximum of graceTimeout.
// Then it forces close any session left.
func (l *Listener) Shutdown(graceTimeout time.Duration) error {
	l.muConns.Lock()
	if !l.accepting {
		l.muConns.Unlock()
		return nil
	}
	l.accepting = false
	l.muConns.Unlock()

	retryInterval := closeRetryInterval
	if retryInterval > graceTimeout {
		retryInterval = graceTimeout
	}
	start := time.Now()
	end := start.Add(graceTimeout)
	for {
		if time.Now().After(end) {
			break
		}

		l.muConns.RLock()
		if len(l.conns) == 0 {
			l.muConns.RUnlock()
			break
		}
		l.muConns.RUnlock()

		time.Sleep(retryInterval)
	}
	return l.close()
}

// readLoop receives all packets from all remotes.
// If a packet comes from a remote that is already known to us (i.e. a "session"),
// we find that session, and otherwise we create a new one.
// We then send the data the session's readLoop.
func (l *Listener) readLoop() {
	for {
		// Allocating a new buffer for every read avoids
		// overwriting data in c.msgs in case the next packet is received
		// before c.msgs is emptied via Read()
		buf := make([]byte, maxDatagramSize)

		n, raddr, err := l.pConn.ReadFrom(buf)
		if err != nil {
			return
		}

		conn, err := l.getConn(raddr)
		if err != nil {
			continue
		}

		select {
		case conn.receiveCh <- buf[:n]:
		case <-conn.doneCh:
			continue
		}
	}
}

// getConn returns the ongoing session with raddr if it exists, or creates a new
// one otherwise.
func (l *Listener) getConn(raddr net.Addr) (*Conn, error) {
	l.muConns.RLock()
	conn, ok := l.conns[raddr.String()]
	l.muConns.RUnlock()
	if ok && (l.requests <= 0 || conn.requests < l.requests) {
		return conn, nil
	}

	// Not reusable
	if conn != nil {
		conn.Close()
	}

	if !l.accepting {
		return nil, errClosedListener
	}

	conn = l.newConn(raddr)

	var err error
	conn.backendsConn, err = l.getBackendsConn(raddr)
	if err != nil {
		return nil, err
	}

	l.muConns.Lock()
	l.conns[raddr.String()] = conn
	l.muConns.Unlock()

	l.acceptCh <- conn
	go conn.readLoop()

	return conn, nil
}

func (l *Listener) getBackendsConn(raddr net.Addr) (*backendsConn, error) {
	l.muBackendsConns.RLock()
	conn, exists := l.backendsConns[raddr.String()]
	l.muBackendsConns.RUnlock()
	if exists {
		return conn, nil
	}

	backendsListener, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}

	bConn := newBackendsConn(backendsListener)

	l.muBackendsConns.Lock()
	l.backendsConns[raddr.String()] = bConn
	l.muBackendsConns.Unlock()

	go func() {
		ticker := time.NewTicker(l.timeout / 10)
		defer ticker.Stop()

		for {
			<-ticker.C

			// alive conn but no response yet from backend.
			if _, exist := l.conns[raddr.String()]; exist {
				return
			}

			bConn.muActivity.RLock()
			deadline := bConn.lastActivity.Add(l.timeout)
			bConn.muActivity.RUnlock()

			if time.Now().After(deadline) {
				bConn.Close()

				l.muBackendsConns.Lock()
				delete(l.backendsConns, raddr.String())
				l.muBackendsConns.Unlock()

				return
			}
		}
	}()

	go func() {
		for {
			buf := make([]byte, maxDatagramSize)
			n, addr, err := bConn.ReadFrom(buf)
			if err != nil {
				if bConn.closed {
					return
				}

				var netErr net.Error
				if errors.As(err, &netErr) && (netErr.Temporary() || netErr.Timeout()) {
					continue
				}

				log.WithoutContext().Errorf("cannot read from backend: %v", err)

				bConn.Close()
				return
			}

			bConn.muTargets.RLock()
			_, exists := bConn.targets[addr.String()]
			bConn.muTargets.RUnlock()

			if !exists {
				continue
			}

			bConn.muActivity.Lock()
			bConn.lastActivity = time.Now()
			bConn.muActivity.Unlock()

			_, err = l.pConn.WriteTo(buf[:n], raddr)
			if err != nil {
				var netErr net.Error
				if errors.As(err, &netErr) && (netErr.Temporary() || netErr.Timeout()) {
					continue
				}

				log.WithoutContext().Errorf("cannot write to backend: %v", err)
				return
			}
		}
	}()

	return bConn, nil
}

func (l *Listener) newConn(rAddr net.Addr) *Conn {
	return &Conn{
		listener:  l,
		rAddr:     rAddr,
		receiveCh: make(chan []byte),
		readCh:    make(chan []byte),
		sizeCh:    make(chan int),
		doneCh:    make(chan struct{}),
		timeout:   l.timeout,
	}
}

// Conn represents an on-going session with a client, over UDP packets.
type Conn struct {
	listener *Listener
	rAddr    net.Addr

	// backendsConn is the connection used to send and receive data from the backends.
	// It is shared across open sessions (Conn) for a given remote address.
	backendsConn *backendsConn

	receiveCh chan []byte // to receive the data from the listener's readLoop
	readCh    chan []byte // to receive the buffer into which we should Read
	sizeCh    chan int    // to synchronize with the end of a Read
	msgs      [][]byte    // to store data from listener, to be consumed by Reads

	muActivity   sync.RWMutex
	lastActivity time.Time // the last time the session saw either read or write activity
	requests     int

	timeout  time.Duration // for timeouts
	doneOnce sync.Once
	doneCh   chan struct{}
}

// readLoop waits for data to come from the listener's readLoop.
// It then waits for a Read operation to be ready to consume said data,
// that is to say it waits on readCh to receive the slice of bytes that the Read operation wants to read onto.
// The Read operation receives the signal that the data has been written to the slice of bytes through the sizeCh.
func (c *Conn) readLoop() {
	ticker := time.NewTicker(c.timeout / 10)
	defer ticker.Stop()

	for {
		if len(c.msgs) == 0 {
			select {
			case msg := <-c.receiveCh:
				c.msgs = append(c.msgs, msg)
			case <-ticker.C:
				c.muActivity.RLock()
				deadline := c.lastActivity.Add(c.timeout)
				c.muActivity.RUnlock()
				if time.Now().After(deadline) {
					c.Close()
					return
				}

				continue
			}
		}

		select {
		case cBuf := <-c.readCh:
			msg := c.msgs[0]
			c.msgs = c.msgs[1:]
			n := copy(cBuf, msg)
			c.sizeCh <- n
		case msg := <-c.receiveCh:
			c.msgs = append(c.msgs, msg)
		case <-ticker.C:
			c.muActivity.RLock()
			deadline := c.lastActivity.Add(c.timeout)
			c.muActivity.RUnlock()
			if time.Now().After(deadline) {
				c.Close()
				return
			}
		}
	}
}

// Read reads up to len(p) bytes into p from the connection.
// Each call corresponds to at most one datagram.
// If p is smaller than the datagram, the extra bytes will be discarded.
// Only returns an error if the connection has been closed.
// Thus, the error can be treated as an end of conn marker and can be discarded.
func (c *Conn) Read(p []byte) (int, error) {
	select {
	case c.readCh <- p:
		n := <-c.sizeCh
		c.muActivity.Lock()
		c.lastActivity = time.Now()
		c.requests++
		c.muActivity.Unlock()
		return n, nil

	case <-c.doneCh:
		return 0, io.EOF
	}
}

// Write writes len(p) bytes from p to the underlying connection.
// Each call sends at most one datagram.
// It is an error to send a message larger than the system's max UDP datagram size.
// Deprecated.
func (c *Conn) Write(p []byte) (n int, err error) {
	c.muActivity.Lock()
	c.lastActivity = time.Now()
	c.muActivity.Unlock()

	return c.backendsConn.Write(p)
}

func (c *Conn) close() {
	c.doneOnce.Do(func() {
		close(c.doneCh)
	})
}

// Close releases resources related to the Conn.
func (c *Conn) Close() error {
	c.close()

	c.listener.muConns.Lock()
	defer c.listener.muConns.Unlock()
	delete(c.listener.conns, c.rAddr.String())
	return nil
}

type backendsConn struct {
	*net.UDPConn

	closed bool

	muTargets sync.RWMutex
	targets   map[string]struct{}

	muActivity   sync.RWMutex
	lastActivity time.Time
}

func newBackendsConn(conn *net.UDPConn) *backendsConn {
	return &backendsConn{
		UDPConn: conn,
		targets: make(map[string]struct{}),
		// lastActivity: time.Now(),
	}
}

func (c *backendsConn) WriteTo(p []byte, target net.Addr) (int, error) {
	c.muActivity.Lock()
	c.lastActivity = time.Now()
	c.muActivity.Unlock()

	c.muTargets.Lock()
	c.targets[target.String()] = struct{}{}
	c.muTargets.Unlock()

	return c.UDPConn.WriteTo(p, target)
}

func (c *backendsConn) Close() error {
	c.closed = true

	return c.UDPConn.Close()
}
