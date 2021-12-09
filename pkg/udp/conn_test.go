package udp

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsecutiveWrites(t *testing.T) {
	addr, err := net.ResolveUDPAddr("udp", ":0")
	require.NoError(t, err)

	ln, err := Listen("udp", addr, 3*time.Second, 0)
	require.NoError(t, err)
	defer func() {
		err := ln.Close()
		require.NoError(t, err)
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if errors.Is(err, errClosedListener) {
				return
			}
			require.NoError(t, err)

			go func() {
				b := make([]byte, 2048)
				b2 := make([]byte, 2048)
				var n int
				var n2 int

				n, err = conn.Read(b)
				require.NoError(t, err)
				// Wait to make sure that the second packet is received
				time.Sleep(10 * time.Millisecond)
				n2, err = conn.Read(b2)
				require.NoError(t, err)

				_, err = conn.listener.pConn.WriteTo(b[:n], conn.rAddr)
				require.NoError(t, err)
				_, err = conn.listener.pConn.WriteTo(b2[:n2], conn.rAddr)
				require.NoError(t, err)
			}()
		}
	}()

	udpConn, err := net.Dial("udp", ln.Addr().String())
	require.NoError(t, err)

	// Send multiple packets of different content and length consecutively
	// Read back packets afterwards and make sure that content matches
	// This checks if any buffers are overwritten while the receiver is enqueuing multiple packets
	b := make([]byte, 2048)
	var n int
	_, err = udpConn.Write([]byte("TESTLONG0"))
	require.NoError(t, err)
	_, err = udpConn.Write([]byte("1TEST"))
	require.NoError(t, err)

	n, err = udpConn.Read(b)
	require.NoError(t, err)
	require.Equal(t, "TESTLONG0", string(b[:n]))
	n, err = udpConn.Read(b)
	require.NoError(t, err)
	require.Equal(t, "1TEST", string(b[:n]))
}

func TestListenNotBlocking(t *testing.T) {
	addr, err := net.ResolveUDPAddr("udp", ":0")

	require.NoError(t, err)

	ln, err := Listen("udp", addr, 3*time.Second, 0)
	require.NoError(t, err)
	defer func() {
		err := ln.Close()
		require.NoError(t, err)
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if errors.Is(err, errClosedListener) {
				return
			}
			require.NoError(t, err)

			go func() {
				b := make([]byte, 2048)
				n, err := conn.Read(b)
				require.NoError(t, err)
				_, err = conn.listener.pConn.WriteTo(b[:n], conn.rAddr)
				require.NoError(t, err)

				n, err = conn.Read(b)
				require.NoError(t, err)
				_, err = conn.listener.pConn.WriteTo(b[:n], conn.rAddr)
				require.NoError(t, err)

				// This should not block second call
				time.Sleep(10 * time.Second)
			}()
		}
	}()

	udpConn, err := net.Dial("udp", ln.Addr().String())
	require.NoError(t, err)

	_, err = udpConn.Write([]byte("TEST"))
	require.NoError(t, err)

	b := make([]byte, 2048)
	n, err := udpConn.Read(b)
	require.NoError(t, err)
	require.Equal(t, "TEST", string(b[:n]))

	_, err = udpConn.Write([]byte("TEST2"))
	require.NoError(t, err)

	n, err = udpConn.Read(b)
	require.NoError(t, err)
	require.Equal(t, "TEST2", string(b[:n]))

	_, err = udpConn.Write([]byte("TEST"))
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		udpConn2, err := net.Dial("udp", ln.Addr().String())
		require.NoError(t, err)

		_, err = udpConn2.Write([]byte("TEST"))
		require.NoError(t, err)

		n, err = udpConn2.Read(b)
		require.NoError(t, err)

		assert.Equal(t, "TEST", string(b[:n]))

		_, err = udpConn2.Write([]byte("TEST2"))
		require.NoError(t, err)

		n, err = udpConn2.Read(b)
		require.NoError(t, err)

		assert.Equal(t, "TEST2", string(b[:n]))

		close(done)
	}()

	select {
	case <-time.Tick(time.Second):
		t.Error("Timeout")
	case <-done:
	}
}

func TestListenWithZeroTimeout(t *testing.T) {
	addr, err := net.ResolveUDPAddr("udp", ":0")
	require.NoError(t, err)

	_, err = Listen("udp", addr, 0, 0)
	assert.Error(t, err)
}

func TestTimeoutWithRead(t *testing.T) {
	testTimeout(t, true)
}

func TestTimeoutWithoutRead(t *testing.T) {
	testTimeout(t, false)
}

func testTimeout(t *testing.T, withRead bool) {
	t.Helper()

	addr, err := net.ResolveUDPAddr("udp", ":0")
	require.NoError(t, err)

	ln, err := Listen("udp", addr, 3*time.Second, 0)
	require.NoError(t, err)
	defer func() {
		err := ln.Close()
		require.NoError(t, err)
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if errors.Is(err, errClosedListener) {
				return
			}
			require.NoError(t, err)

			if withRead {
				buf := make([]byte, 1024)
				_, err = conn.Read(buf)

				require.NoError(t, err)
			}
		}
	}()

	for i := 0; i < 10; i++ {
		udpConn2, err := net.Dial("udp", ln.Addr().String())
		require.NoError(t, err)

		_, err = udpConn2.Write([]byte("TEST"))
		require.NoError(t, err)
	}

	time.Sleep(10 * time.Millisecond)

	assert.Equal(t, 10, len(ln.conns))

	time.Sleep(ln.timeout + time.Second)
	assert.Equal(t, 0, len(ln.conns))
}

func TestShutdown(t *testing.T) {
	addr, err := net.ResolveUDPAddr("udp", ":0")
	require.NoError(t, err)

	l, err := Listen("udp", addr, 3*time.Second, 0)
	require.NoError(t, err)

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}

			go func() {
				conn := conn
				for {
					b := make([]byte, 1024*1024)
					n, err := conn.Read(b)
					require.NoError(t, err)
					// We control the termination,
					// otherwise we would block on the Read above,
					// until conn is closed by a timeout.
					// Which means we would get an error,
					// and even though we are in a goroutine and the current test might be over,
					// go test would still yell at us if this happens while other tests are still running.
					if string(b[:n]) == "CLOSE" {
						return
					}
					_, err = conn.listener.pConn.WriteTo(b[:n], conn.rAddr)
					require.NoError(t, err)
				}
			}()
		}
	}()

	conn, err := net.Dial("udp", l.Addr().String())
	require.NoError(t, err)

	// Start sending packets, to create a "session" with the server.
	requireEcho(t, "TEST", conn, time.Second)

	doneChan := make(chan struct{})
	go func() {
		err := l.Shutdown(5 * time.Second)
		require.NoError(t, err)
		close(doneChan)
	}()

	// Make sure that our session is still live even after the shutdown.
	requireEcho(t, "TEST2", conn, time.Second)

	// And make sure that on the other hand, opening new sessions is not possible anymore.
	conn2, err := net.Dial("udp", l.Addr().String())
	require.NoError(t, err)

	_, err = conn2.Write([]byte("TEST"))
	// Packet is accepted, but dropped
	require.NoError(t, err)

	// Make sure that our session is yet again still live.
	// This is specifically to make sure we don't create a regression in listener's readLoop,
	// i.e. that we only terminate the listener's readLoop goroutine by closing its pConn.
	requireEcho(t, "TEST3", conn, time.Second)

	done := make(chan bool)
	go func() {
		defer close(done)
		b := make([]byte, 1024*1024)
		n, err := conn2.Read(b)
		require.Error(t, err)
		assert.Equal(t, 0, n)
	}()

	conn2.Close()

	select {
	case <-done:
	case <-time.Tick(time.Second):
		t.Fatal("Timeout")
	}

	_, err = conn.Write([]byte("CLOSE"))
	require.NoError(t, err)

	select {
	case <-doneChan:
	case <-time.Tick(5 * time.Second):
		// In case we introduce a regression that would make the test wait forever.
		t.Fatal("Timeout during shutdown")
	}
}

func TestReadLoopMaxDataSize(t *testing.T) {
	if runtime.GOOS == "darwin" {
		// sudo sysctl -w net.inet.udp.maxdgram=65507
		t.Skip("Skip test on darwin as the maximum dgram size is set to 9216 bytes by default")
	}

	// Theoretical maximum size of data in a UDP datagram.
	// 65535 − 8 (UDP header) − 20 (IP header).
	dataSize := 65507

	doneCh := make(chan struct{})

	addr, err := net.ResolveUDPAddr("udp", ":0")
	require.NoError(t, err)

	l, err := Listen("udp", addr, 3*time.Second, 0)
	require.NoError(t, err)

	defer func() {
		err := l.Close()
		require.NoError(t, err)
	}()

	go func() {
		defer close(doneCh)

		conn, err := l.Accept()
		require.NoError(t, err)

		buffer := make([]byte, dataSize)

		n, err := conn.Read(buffer)
		require.NoError(t, err)

		assert.Equal(t, dataSize, n)
	}()

	c, err := net.Dial("udp", l.Addr().String())
	require.NoError(t, err)

	data := make([]byte, dataSize)

	_, err = rand.Read(data)
	require.NoError(t, err)

	_, err = c.Write(data)
	require.NoError(t, err)

	select {
	case <-doneCh:
	case <-time.Tick(5 * time.Second):
		t.Fatal("Timeout waiting for datagram read")
	}
}

func Test_RequestsLimit(t *testing.T) {
	requests := 42
	testCases := []struct {
		desc        string
		requests    int
		wantLBCalls int
		wantErr     bool
	}{
		{
			desc:        "Empty",
			wantLBCalls: 1,
		},
		{
			desc:        "Requests limit is lower than the total of requests",
			requests:    requests / 4,
			wantLBCalls: 5,
		},
		{
			desc:        "Requests limit equals the total of requests",
			requests:    requests,
			wantLBCalls: 1,
		},
		{
			desc:        "Requests limit is greater than the total of requests",
			requests:    requests * 2,
			wantLBCalls: 1,
		},
	}

	for i, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			backendAddr := fmt.Sprintf("127.0.0.1:808%d", 2*i)
			backendConn := newEchoBackend(t, backendAddr, 2048)
			defer func() {
				require.NoError(t, backendConn.Close())
			}()

			proxy, err := NewProxy(backendAddr)
			require.NoError(t, err)

			lbCalls := 0
			proxyHandler := HandlerFunc(func(conn *Conn) {
				lbCalls++
				proxy.ServeUDP(conn)
			})

			listenerAddr := fmt.Sprintf(":808%d", 2*i+1)
			listener := newServerWithOptions(t, listenerAddr, time.Second, test.requests, proxyHandler)
			defer listener.Close()

			udpConn, err := net.Dial("udp", listenerAddr)
			require.NoError(t, err)

			var gotErr bool
			for i := 0; i < requests; i++ {
				_, err = udpConn.Write([]byte("DATAWRITE"))
				if err != nil {
					gotErr = true
				}

				b := make([]byte, 2048)
				n, err := udpConn.Read(b)
				if err != nil {
					gotErr = true
				}
				assert.Equal(t, "DATAWRITE", string(b[:n]))
			}

			assert.Equal(t, test.wantErr, gotErr)
			assert.Equal(t, test.wantLBCalls, lbCalls)
		})
	}
}

func Test_UnknownBackend(t *testing.T) {
	backendAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:8080")
	require.NoError(t, err)

	backendConn, err := net.ListenUDP("udp", backendAddr)
	require.NoError(t, err)
	defer backendConn.Close()

	backend2Addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:8081")
	require.NoError(t, err)

	backend2, err := net.ListenUDP("udp", backend2Addr)
	require.NoError(t, err)
	defer backend2.Close()

	go func() {
		for {
			b := make([]byte, 2048)
			_, from, err := backendConn.ReadFrom(b)
			if err != nil {
				return
			}

			_, err = backendConn.WriteTo([]byte("ACK"), from)
			if err != nil {
				return
			}

			_, err = backend2.WriteTo([]byte("FAKE"), from)
			if err != nil {
				return
			}
		}
	}()

	proxy, err := NewProxy(backendAddr.String())
	require.NoError(t, err)

	listener := newServerWithOptions(t, ":8083", time.Second, 0, proxy)
	defer listener.Close()

	udpConn, err := net.Dial("udp", ":8083")
	require.NoError(t, err)

	for i := 0; i < 42; i++ {
		_, err = udpConn.Write([]byte("DATAWRITE"))
		require.NoError(t, err)

		b := make([]byte, 2048)
		n, err := udpConn.Read(b)
		require.NoError(t, err)
		assert.Equal(t, "ACK", string(b[:n]))
	}
}

// requireEcho tests that the conn session is live and functional,
// by writing data through it, and expecting the same data as a response when reading on it.
// It fatals if the read blocks longer than timeout,
// which is useful to detect regressions that would make a test wait forever.
func requireEcho(t *testing.T, data string, conn io.ReadWriter, timeout time.Duration) {
	t.Helper()

	_, err := conn.Write([]byte(data))
	require.NoError(t, err)

	doneChan := make(chan struct{})
	go func() {
		b := make([]byte, 1024*1024)
		n, err := conn.Read(b)
		require.NoError(t, err)
		assert.Equal(t, data, string(b[:n]))
		close(doneChan)
	}()

	select {
	case <-doneChan:
	case <-time.Tick(timeout):
		t.Fatalf("Timeout during echo for: %s", data)
	}
}
