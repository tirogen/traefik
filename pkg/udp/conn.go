package udp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// maxDatagramSize is the maximum size of a UDP datagram.
const maxDatagramSize = 65535

const closeRetryInterval = 500 * time.Millisecond

var errClosedListener = errors.New("udp: listener closed")

type Session struct {
	// Requests FIXME
	Requests int

	// FIXME What about the 0
	// Responses limits the number of expected datagrams for a server.
	// If not set, no limitations
	// If set to 0, no response expected. However, if a response is received
	// and the session is still not finished, the response will be handled.
	Responses int

	// Timeout defines how long to wait on an idle session,
	// before releasing its related resources.
	Timeout time.Duration
}

// Listener augments a session-oriented Listener over a UDP PacketConn.
type Listener struct {
	pConn *net.UDPConn

	mu    sync.RWMutex
	conns map[string][]*Conn
	// accepting signifies whether the listener is still accepting new sessions.
	// It also serves as a sentinel for Shutdown to be idempotent.
	accepting bool

	acceptCh chan *Conn // no need for a Once, already indirectly guarded by accepting.

	// session limit datagrams from server
	session Session
}

// Listen creates a new listener.
func Listen(network string, laddr *net.UDPAddr, session Session) (*Listener, error) {
	if session.Timeout <= 0 {
		return nil, errors.New("timeout should be greater than zero")
	}

	conn, err := net.ListenUDP(network, laddr)
	if err != nil {
		return nil, err
	}

	l := &Listener{
		pConn:     conn,
		acceptCh:  make(chan *Conn),
		conns:     make(map[string][]*Conn),
		accepting: true,
		session:   session,
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

	fmt.Println("Accept")

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
	l.mu.Lock()
	defer l.mu.Unlock()

	err := l.pConn.Close()
	for k, conns := range l.conns {
		for _, conn := range conns {
			conn.close()
		}
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
	l.mu.Lock()
	if !l.accepting {
		l.mu.Unlock()
		return nil
	}
	l.accepting = false
	l.mu.Unlock()

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

		l.mu.RLock()
		if len(l.conns) == 0 {
			l.mu.RUnlock()
			break
		}
		l.mu.RUnlock()

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
			println("GET CONN NOK")
			fmt.Println(err)
			continue
		}

		println("GET CONN OK")

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
	l.mu.Lock()
	defer l.mu.Unlock()

	raddrStr := raddr.String()
	fmt.Printf("%v\n", l.conns)
	conns, ok := l.conns[raddrStr]
	if ok {
		for _, conn := range conns {
			if !conn.isClosed && conn.canRead() {
				return conn, nil
			}
		}
	}

	if !l.accepting {
		return nil, errClosedListener
	}

	newConn := l.newConn(raddr)
	l.acceptCh <- newConn

	select {
	case <-newConn.doneCh:
	case <-newConn.StartCh:
		//if !ok {
		//	l.conns[raddrStr] = []*Conn{}
		//}
		l.conns[raddrStr] = append(l.conns[raddrStr], newConn)
		go newConn.readLoop()
		return newConn, nil
	}

	if !ok || len(l.conns[raddrStr]) == 0 {
		return nil, errors.New("fail to start new connection")
	}

	for _, conn := range conns {
		if conn.target == newConn.target {
			conn.muActivity.Lock()
			conn.lastActivity = time.Now()
			conn.readCount = 0
			conn.muActivity.Unlock()

			// TODO check if we can loopback
			if conn.isClosed {
				return nil, errors.New("closed connection")
			}

			return conn, nil
		}
	}

	return nil, errors.New("fail to get connection")
}

func (l *Listener) newConn(rAddr net.Addr) *Conn {
	return &Conn{
		listener:  l,
		rAddr:     rAddr,
		receiveCh: make(chan []byte),
		readCh:    make(chan []byte),
		sizeCh:    make(chan int),
		StartCh:   make(chan struct{}),
		doneCh:    make(chan struct{}),
		session:   l.session,
	}
}

// Conn represents an on-going session with a client, over UDP packets.
type Conn struct {
	listener *Listener
	rAddr    net.Addr

	receiveCh chan []byte // to receive the data from the listener's readLoop
	readCh    chan []byte // to receive the buffer into which we should Read
	sizeCh    chan int    // to synchronize with the end of a Read
	msgs      [][]byte    // to store data from listener, to be consumed by Reads

	muActivity   sync.RWMutex
	lastActivity time.Time // the last time the session saw either read or write activity
	receiveCount int
	readCount    int

	session  Session // for timeouts
	doneOnce sync.Once
	StartCh  chan struct{}
	doneCh   chan struct{}

	target   string
	isClosed bool
}

// readLoop waits for data to come from the listener's readLoop.
// It then waits for a Read operation to be ready to consume said data,
// that is to say it waits on readCh to receive the slice of bytes that the Read operation wants to read onto.
// The Read operation receives the signal that the data has been written to the slice of bytes through the sizeCh.
func (c *Conn) readLoop() {
	ticker := time.NewTicker(c.session.Timeout / 10)
	defer ticker.Stop()

	for {
		if len(c.msgs) == 0 {
			select {
			case msg := <-c.receiveCh:
				c.msgs = append(c.msgs, msg)
			case <-ticker.C:
				c.muActivity.RLock()
				deadline := c.lastActivity.Add(c.session.Timeout)
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
			deadline := c.lastActivity.Add(c.session.Timeout)
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
func (c *Conn) Read(p []byte) (int, error) {
	select {
	case c.readCh <- p:
		fmt.Println("read")
		n := <-c.sizeCh
		c.muActivity.Lock()
		c.lastActivity = time.Now()
		c.readCount++
		c.muActivity.Unlock()
		return n, nil

	case <-c.doneCh:
		return 0, io.EOF
	}
}

// Write writes len(p) bytes from p to the underlying connection.
// Each call sends at most one datagram.
// It is an error to send a message larger than the system's max UDP datagram size.
func (c *Conn) Write(p []byte) (n int, err error) {
	if c.listener == nil {
		return 0, io.EOF
	}
	fmt.Println("write")

	c.muActivity.Lock()
	c.lastActivity = time.Now()
	c.muActivity.Unlock()
	n, err = c.listener.pConn.WriteTo(p, c.rAddr)
	if err != nil {
		return n, err
	}

	c.muActivity.Lock()
	c.receiveCount++
	c.muActivity.Unlock()

	if c.receiveCount > c.session.Responses {
		return n, c.Close()
	}

	return n, nil
}

func (c *Conn) canRead() bool {
	return c.session.Requests == 0 || c.readCount < c.session.Requests
}

func (c *Conn) close() {
	c.doneOnce.Do(func() {
		close(c.StartCh)
		close(c.doneCh)
	})
}

// Close releases resources related to the Conn.
func (c *Conn) Close() error {
	c.close()
	c.isClosed = true

	c.listener.mu.Lock()
	defer c.listener.mu.Unlock()
	delete(c.listener.conns, c.rAddr.String())
	return nil
}
