package network

import (
	"context"
	"fmt"
	"net"

	utls "github.com/refraction-networking/utls"
)

// tlsDialer wraps a plain net.Dialer and upgrades connections to uTLS
// so the TLS ClientHello matches the spoofed browser fingerprint.
type tlsDialer struct {
	preset string
	inner  net.Dialer
}

// newTLSDialer constructs a tlsDialer for the given TLS preset name.
func newTLSDialer(preset string) *tlsDialer {
	return &tlsDialer{preset: preset}
}

// GetUTLSHelloID maps a human-readable preset name to the corresponding
// utls.ClientHelloID. Falls back to HelloChrome_120 for unknown presets.
func GetUTLSHelloID(preset string) utls.ClientHelloID {
	switch preset {
	case "chrome_120":
		return utls.HelloChrome_120
	case "firefox_121":
		// Closest available Firefox preset in the utls library.
		return utls.HelloFirefox_105
	case "safari_17":
		return utls.HelloSafari_16_0
	default:
		return utls.HelloChrome_120
	}
}

// newUTLSConn wraps an existing TCP connection with a uTLS UClient configured
// for the given hostname (SNI) and ClientHelloID preset.
func newUTLSConn(tcpConn net.Conn, hostname string, preset string) (*utls.UConn, error) {
	helloID := GetUTLSHelloID(preset)

	tlsCfg := &utls.Config{
		ServerName:         hostname,
		InsecureSkipVerify: false, //nolint:gosec // controlled by caller
	}

	uconn := utls.UClient(tcpConn, tlsCfg, helloID)
	return uconn, nil
}

// DialTLS establishes a TLS connection to addr using the spoofed ClientHello.
//
// Flow:
//  1. Parse addr to separate hostname and port.
//  2. Dial TCP via inner net.Dialer (honours any proxy set on the context).
//  3. Wrap the raw connection with uTLS and the chosen ClientHelloID.
//  4. Perform the TLS handshake.
//  5. Return the uTLS connection (implements net.Conn).
func (d *tlsDialer) DialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("tls dialer: parse addr %q: %w", addr, err)
	}

	// Step 1: Dial TCP.
	tcpConn, err := d.inner.DialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("tls dialer: tcp dial %s: %w", addr, err)
	}

	// Step 2: Wrap with uTLS.
	uconn, err := newUTLSConn(tcpConn, host, d.preset)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("tls dialer: create utls conn: %w", err)
	}

	// Step 3: Perform the TLS handshake using the spoofed ClientHello.
	if err := uconn.HandshakeContext(ctx); err != nil {
		uconn.Close()
		return nil, fmt.Errorf("tls dialer: tls handshake with %s: %w", host, err)
	}

	return uconn, nil
}
