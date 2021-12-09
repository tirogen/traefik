package udp

import (
	"crypto/rand"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxy_ServeUDP(t *testing.T) {
	backendAddr := "127.0.0.1:8081"
	backendConn := newEchoBackend(t, backendAddr, 1024*1024)
	defer func() {
		require.NoError(t, backendConn.Close())
	}()

	proxy, err := NewProxy(backendAddr)
	require.NoError(t, err)

	proxyAddr := ":8080"
	listener := newServer(t, proxyAddr, proxy)
	defer listener.Close()

	time.Sleep(time.Second)

	udpConn, err := net.Dial("udp", proxyAddr)
	require.NoError(t, err)

	_, err = udpConn.Write([]byte("DATAWRITE"))
	require.NoError(t, err)

	b := make([]byte, 1024*1024)
	n, err := udpConn.Read(b)
	require.NoError(t, err)

	assert.Equal(t, "DATAWRITE", string(b[:n]))
}

func TestProxy_ServeUDP_MaxDataSize(t *testing.T) {
	if runtime.GOOS == "darwin" {
		// sudo sysctl -w net.inet.udp.maxdgram=65507
		t.Skip("Skip test on darwin as the maximum dgram size is set to 9216 bytes by default")
	}

	// Theoretical maximum size of data in a UDP datagram.
	// 65535 − 8 (UDP header) − 20 (IP header).
	dataSize := 65507

	backendAddr := "127.0.0.1:8083"
	backendConn := newEchoBackend(t, backendAddr, dataSize)
	defer func() {
		require.NoError(t, backendConn.Close())
	}()

	proxy, err := NewProxy(backendAddr)
	require.NoError(t, err)

	listenerAddr := ":8082"
	listener := newServer(t, listenerAddr, proxy)
	defer listener.Close()

	time.Sleep(time.Second)

	udpConn, err := net.Dial("udp", listenerAddr)
	require.NoError(t, err)

	want := make([]byte, dataSize)

	_, err = rand.Read(want)
	require.NoError(t, err)

	_, err = udpConn.Write(want)
	require.NoError(t, err)

	got := make([]byte, dataSize)

	_, err = udpConn.Read(got)
	require.NoError(t, err)

	assert.Equal(t, want, got)
}

func newServer(t *testing.T, addr string, handler Handler) *Listener {
	t.Helper()

	return newServerWithOptions(t, addr, 3*time.Second, 0, handler)
}

func newServerWithOptions(t *testing.T, addr string, timeout time.Duration, requests int, handler Handler) *Listener {
	t.Helper()

	addrL, err := net.ResolveUDPAddr("udp", addr)
	require.NoError(t, err)

	listener, err := Listen("udp", addrL, timeout, requests)
	require.NoError(t, err)

	go func() {
		for {
			if !listener.accepting {
				return
			}

			conn, err := listener.Accept()
			if err != nil {
				return
			}

			go handler.ServeUDP(conn)
		}
	}()

	return listener
}

func newEchoBackend(t *testing.T, addr string, dataSize int) *net.UDPConn {
	t.Helper()

	backendAddr, err := net.ResolveUDPAddr("udp", addr)
	require.NoError(t, err)

	backendConn, err := net.ListenUDP("udp", backendAddr)
	require.NoError(t, err)

	go func() {
		for {
			buffer := make([]byte, dataSize)

			n, addr, err := backendConn.ReadFrom(buffer)
			if err != nil {
				return
			}

			_, err = backendConn.WriteTo(buffer[:n], addr)
			if err != nil {
				return
			}
		}
	}()

	return backendConn
}
