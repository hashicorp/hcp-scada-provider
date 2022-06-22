package tcp

import (
	"crypto/tls"
	"net"
	"time"
)

const (
	// timeout is the dial timeout to the SCADA brokers.
	timeout = 10 * time.Second
)

// Dialer dials the passed address.
type Dialer struct {
	// TLSConfig is the TLS configuration to use when dialing.
	TLSConfig *tls.Config
}

func (d *Dialer) Dial(addr string) (net.Conn, error) {
	if d.TLSConfig != nil {
		// Create a dialer with a timeout
		dialer := &net.Dialer{Timeout: timeout}
		return tls.DialWithDialer(dialer, "tcp", addr, d.TLSConfig)
	}

	return net.DialTimeout("tcp", addr, timeout)
}
