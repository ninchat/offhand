package relink

import (
	"crypto/tls"
	"net"
)

// Connector tells an Endpoint how to dial (and redial) a peer.
type Connector interface {
	Connect() (net.Conn, error) // dial
	Close() error               // abort the connection attempt (if possible)
}

// NetConnector adapts net.Dialer for use with an Endpoint.  The Dialer.Timeout
// field should be set since the Close method is a no-op.  (Using the
// Dialer.Deadline field will prevent reconnections.)
type NetConnector struct {
	Network string
	Address string
	Dialer  net.Dialer
}

// Connect dials the Address in the Network.
func (nc *NetConnector) Connect() (net.Conn, error) {
	return nc.Dialer.Dial(nc.Network, nc.Address)
}

// Close does nothing.
func (*NetConnector) Close() (err error) {
	return
}

// String returns the Address.
func (nc *NetConnector) String() string {
	return nc.Address
}

// TLSConnector wraps tls.DialWithDialer for use with an Endpoint.  See
// NetConnector.
type TLSConnector struct {
	NetConnector
	Config *tls.Config
}

// Connect dials the NetConnector.Address in the NetConnector.Network.
func (tc *TLSConnector) Connect() (net.Conn, error) {
	return tls.DialWithDialer(&tc.NetConnector.Dialer, tc.NetConnector.Network, tc.NetConnector.Address, tc.Config)
}
