package bidirpc

import (
	"crypto/tls"
	"net"
	"time"
)

// Transport is an alias for net.Conn.
type Transport = net.Conn

// Dialer abstracts the transport layer. It can dial with or without TLS.
type Dialer struct {
	Timeout   time.Duration
	UseTLS    bool
	TLSConfig *tls.Config
	ALPN      string
}

// Dial creates a Transport connection to the given address.
// If UseTLS is true, it performs a TLS handshake and sets NextProtos to ALPN.
func (d *Dialer) Dial(addr string) (Transport, error) {
	conn, err := net.DialTimeout("tcp", addr, d.Timeout)
	if err != nil {
		return nil, err
	}
	if d.UseTLS {
		if d.TLSConfig == nil {
			d.TLSConfig = &tls.Config{}
		}
		d.TLSConfig.NextProtos = []string{d.ALPN}
		tlsConn := tls.Client(conn, d.TLSConfig)
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			return nil, err
		}
		return tlsConn, nil
	}
	return conn, nil
}
