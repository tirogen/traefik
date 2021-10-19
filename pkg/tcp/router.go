package tcp

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/traefik/traefik/v2/pkg/log"
)

const defaultBufSize = 4096

// Router is a TCP router.
type Router struct {
	tcpMuxer    Muxer
	tcpMuxerTLS Muxer

	// Forwarder handlers.
	httpForwarder  Handler
	httpsForwarder Handler

	// HTTP(S) handlers.
	httpHandler  http.Handler
	httpsHandler http.Handler

	// TLS configs.
	httpsTLSConfig    *tls.Config            // default TLS config
	hostHTTPTLSConfig map[string]*tls.Config // TLS configs keyed by SNI
}

// NewRouter returns a new TCP router.
func NewRouter() (*Router, error) {
	mux, err := NewMuxer()
	if err != nil {
		return nil, err
	}

	muxTLS, err := NewMuxer()
	if err != nil {
		return nil, err
	}

	return &Router{
		tcpMuxer:    *mux,
		tcpMuxerTLS: *muxTLS,
	}, nil
}

// GetTLSGetClientInfo is called after a ClientHello is received from a client.
func (r *Router) GetTLSGetClientInfo() func(info *tls.ClientHelloInfo) (*tls.Config, error) {
	return func(info *tls.ClientHelloInfo) (*tls.Config, error) {
		if tlsConfig, ok := r.hostHTTPTLSConfig[info.ServerName]; ok {
			return tlsConfig, nil
		}
		return r.httpsTLSConfig, nil
	}
}

// ServeTCP forwards the connection to the right TCP/HTTP handler.
func (r *Router) ServeTCP(conn WriteCloser) {
	fmt.Printf("SERVE TCP on %s\n", conn.LocalAddr().String())
	// FIXME -- Check if ProxyProtocol changes the first bytes of the request

	// if !r.tcpMuxer.HasRoutes() && !r.tcpMuxerTLS.HasRoutes() {
	//	r.catchAllNoTLS.ServeTCP(conn)
	//	return
	// }

	if r.tcpMuxer.hasRoutes() && !r.tcpMuxerTLS.hasRoutes() {
		fmt.Printf("NEW READER on %s\n", conn.LocalAddr().String())
		connData, err := NewConnData("", conn)
		if err != nil {
			// TODO
			log.WithoutContext().Errorf("Error while : %v", err)
			conn.Close()
			return
		}

		handler := r.tcpMuxer.Match(connData)
		switch {
		case handler != nil:
			handler.ServeTCP(conn)
		default:
			conn.Close()
		}
		return
	}

	br := bufio.NewReader(conn)
	fmt.Printf("NEW READER on %s\n", conn.LocalAddr().String())
	serverName, tls, peeked, err := clientHelloServerName(br)
	if err != nil {
		fmt.Printf("TCP READER ERROR on %s: %v\n", conn.LocalAddr().String(), err)
		conn.Close()
		return
	}
	fmt.Printf("PEEKED BYTES %s on %s\n", peeked, conn.LocalAddr().String())

	// Remove read/write deadline and delegate this to underlying tcp server (for now only handled by HTTP Server)
	err = conn.SetReadDeadline(time.Time{})
	if err != nil {
		log.WithoutContext().Errorf("Error while setting read deadline: %v", err)
	}

	err = conn.SetWriteDeadline(time.Time{})
	if err != nil {
		log.WithoutContext().Errorf("Error while setting write deadline: %v", err)
	}

	connData, err := NewConnData(serverName, conn)
	if err != nil {
		// TODO
		log.WithoutContext().Errorf("Error while : %v", err)
		conn.Close()
		return
	}

	if !tls {

		fmt.Printf("Try to match with routes length:%v\n", len(r.tcpMuxer.routes))
		// TODO priority (between ClientIP and HostSNI(`*`) for instance)
		handler := r.tcpMuxer.Match(connData)
		switch {
		case handler != nil:
			fmt.Printf("FOUND HANDLER TCP on %s\n", conn.LocalAddr().String())
			handler.ServeTCP(r.GetConn(conn, peeked))
		case r.httpForwarder != nil:
			fmt.Printf("SERVE HTTP on %s\n", conn.LocalAddr().String())
			r.httpForwarder.ServeTCP(r.GetConn(conn, peeked))
		default:
			conn.Close()
		}
		return
	}

	handler := r.tcpMuxerTLS.Match(connData)
	if handler != nil {
		handler.ServeTCP(r.GetConn(conn, peeked))
		return
	}

	if r.httpsForwarder != nil {
		r.httpsForwarder.ServeTCP(r.GetConn(conn, peeked))
		return
	}

	conn.Close()
}

// AddRoute defines a handler for the given rule.
func (r *Router) AddRoute(rule string, target Handler) error {
	return r.tcpMuxer.AddRoute(rule, target)
}

// AddRouteTLS defines a handler for a given rule and sets the matching tlsConfig.
func (r *Router) AddRouteTLS(rule string, target Handler, config *tls.Config) error {
	return r.tcpMuxerTLS.AddRoute(rule, &TLSHandler{
		Next:   target,
		Config: config,
	})
}

// AddHTTPTLSConfig defines a handler for a given sniHost and sets the matching tlsConfig.
func (r *Router) AddHTTPTLSConfig(sniHost string, config *tls.Config) {
	if r.hostHTTPTLSConfig == nil {
		r.hostHTTPTLSConfig = map[string]*tls.Config{}
	}
	r.hostHTTPTLSConfig[sniHost] = config
	fmt.Printf("Config for host %s: %v\n", sniHost, spew.Sprint(config))
}

// GetConn creates a connection proxy with a peeked string.
func (r *Router) GetConn(conn WriteCloser, peeked string) WriteCloser {
	// FIXME should it really be on Router ?
	conn = &Conn{
		Peeked:      []byte(peeked),
		WriteCloser: conn,
	}
	return conn
}

// GetHTTPHandler gets the attached http handler.
func (r *Router) GetHTTPHandler() http.Handler {
	return r.httpHandler
}

// GetHTTPSHandler gets the attached https handler.
func (r *Router) GetHTTPSHandler() http.Handler {
	return r.httpsHandler
}

// HTTPForwarder sets the tcp handler that will forward the connections to an http handler.
func (r *Router) HTTPForwarder(handler Handler) {
	r.httpForwarder = handler
}

// HTTPSForwarder sets the tcp handler that will forward the TLS connections to an http handler.
func (r *Router) HTTPSForwarder(handler Handler) {
	for sniHost, tlsConf := range r.hostHTTPTLSConfig {
		log.WithoutContext().Errorf("ADD ROUTE TLS: %s", sniHost)
		// TODO check if we ignore the error
		err := r.AddRouteTLS("HostSNI(`"+sniHost+"`)", handler, tlsConf)
		if err != nil {
			log.WithoutContext().Errorf("Error while adding route for host: %w", err)
		}
	}

	r.httpsForwarder = &TLSHandler{
		Next:   handler,
		Config: r.httpsTLSConfig,
	}
}

// HTTPHandler attaches http handlers on the router.
func (r *Router) HTTPHandler(handler http.Handler) {
	r.httpHandler = handler
}

// HTTPSHandler attaches https handlers on the router.
func (r *Router) HTTPSHandler(handler http.Handler, config *tls.Config) {
	r.httpsHandler = handler
	r.httpsTLSConfig = config
}

// Conn is a connection proxy that handles Peeked bytes.
type Conn struct {
	// Peeked are the bytes that have been read from Conn for the
	// purposes of route matching, but have not yet been consumed
	// by Read calls. It set to nil by Read when fully consumed.
	Peeked []byte

	// Conn is the underlying connection.
	// It can be type asserted against *net.TCPConn or other types
	// as needed. It should not be read from directly unless
	// Peeked is nil.
	WriteCloser
}

// Read reads bytes from the connection (using the buffer prior to actually reading).
func (c *Conn) Read(p []byte) (n int, err error) {
	if len(c.Peeked) > 0 {
		n = copy(p, c.Peeked)
		c.Peeked = c.Peeked[n:]
		if len(c.Peeked) == 0 {
			c.Peeked = nil
		}
		return n, nil
	}
	return c.WriteCloser.Read(p)
}

// clientHelloServerName returns the SNI server name inside the TLS ClientHello,
// without consuming any bytes from br.
// On any error, the empty string is returned.
func clientHelloServerName(br *bufio.Reader) (string, bool, string, error) {
	hdr, err := br.Peek(1)
	if err != nil {
		var opErr *net.OpError
		if !errors.Is(err, io.EOF) && (!errors.As(err, &opErr) || opErr.Timeout()) {
			log.WithoutContext().Debugf("Error while Peeking first byte: %s", err)
		}

		return "", false, "", err
	}

	// No valid TLS record has a type of 0x80, however SSLv2 handshakes
	// start with a uint16 length where the MSB is set and the first record
	// is always < 256 bytes long. Therefore typ == 0x80 strongly suggests
	// an SSLv2 client.
	const recordTypeSSLv2 = 0x80
	const recordTypeHandshake = 0x16
	if hdr[0] != recordTypeHandshake {
		if hdr[0] == recordTypeSSLv2 {
			// we consider SSLv2 as TLS and it will be refuse by real TLS handshake.
			return "", true, getPeeked(br), nil
		}
		return "", false, getPeeked(br), nil // Not TLS.
	}

	const recordHeaderLen = 5
	hdr, err = br.Peek(recordHeaderLen)
	if err != nil {
		log.Errorf("Error while Peeking hello: %s", err)
		return "", false, getPeeked(br), nil
	}

	recLen := int(hdr[3])<<8 | int(hdr[4]) // ignoring version in hdr[1:3]

	if recordHeaderLen+recLen > defaultBufSize {
		br = bufio.NewReaderSize(br, recordHeaderLen+recLen)
	}

	helloBytes, err := br.Peek(recordHeaderLen + recLen)
	if err != nil {
		log.Errorf("Error while Hello: %s", err)
		return "", true, getPeeked(br), nil
	}

	sni := ""
	server := tls.Server(sniSniffConn{r: bytes.NewReader(helloBytes)}, &tls.Config{
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			sni = hello.ServerName
			return nil, nil
		},
	})
	_ = server.Handshake()

	return sni, true, getPeeked(br), nil
}

func getPeeked(br *bufio.Reader) string {
	peeked, err := br.Peek(br.Buffered())
	if err != nil {
		log.Errorf("Could not get anything: %s", err)
		return ""
	}
	return string(peeked)
}

// sniSniffConn is a net.Conn that reads from r, fails on Writes,
// and crashes otherwise.
type sniSniffConn struct {
	r        io.Reader
	net.Conn // nil; crash on any unexpected use
}

// Read reads from the underlying reader.
func (c sniSniffConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// Write crashes all the time.
func (sniSniffConn) Write(p []byte) (int, error) { return 0, io.EOF }
