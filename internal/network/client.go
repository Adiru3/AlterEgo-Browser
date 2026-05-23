package network

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/alterego/browser/internal/profile"
	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/proxy"
)

// -----------------------------------------------------------------------------
// profileTransport – custom RoundTripper
// -----------------------------------------------------------------------------

// profileTransport wraps http.Transport and overrides DialTLSContext so that
// HTTPS connections go through: proxy → uTLS handshake.
//
// The embedded *http.Transport handles all plaintext HTTP (port 80) as well as
// connection pooling, keep-alives, timeouts, and header canonicalisation.
type profileTransport struct {
	base        *http.Transport
	proxyDialer proxy.Dialer
	tlsPreset   string
}

// RoundTrip satisfies http.RoundTripper. All requests pass through the base
// transport which has DialContext and DialTLSContext set up correctly.
func (t *profileTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.base.RoundTrip(req)
}

// dialTLSContext is assigned to http.Transport.DialTLSContext. It:
//  1. Uses the proxy dialer to establish a TCP connection (SOCKS5/HTTP CONNECT).
//  2. Wraps that raw TCP connection with uTLS so the ClientHello matches the
//     chosen browser fingerprint.
//  3. Performs the TLS handshake and returns the encrypted connection.
func (t *profileTransport) dialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("transport: parse addr %q: %w", addr, err)
	}

	// Dial TCP through the proxy (or directly if proxy.Direct).
	tcpConn, err := t.proxyDialer.Dial(network, addr)
	if err != nil {
		return nil, fmt.Errorf("transport: proxy dial %s: %w", addr, err)
	}

	// Wrap with uTLS.
	helloID := GetUTLSHelloID(t.tlsPreset)
	tlsCfg := &utls.Config{
		ServerName:         host,
		InsecureSkipVerify: false, //nolint:gosec
	}
	uconn := utls.UClient(tcpConn, tlsCfg, helloID)

	if err := uconn.HandshakeContext(ctx); err != nil {
		uconn.Close()
		return nil, fmt.Errorf("transport: utls handshake %s: %w", host, err)
	}

	return uconn, nil
}

// -----------------------------------------------------------------------------
// ProfileClient
// -----------------------------------------------------------------------------

// ProfileClient is an isolated HTTP client for a single browser profile.
// Every profile gets its own:
//   - TCP socket pool (via a dedicated http.Transport)
//   - TLS stack using uTLS to spoof the JA3 fingerprint
//   - SOCKS5/HTTP proxy binding
//   - DoH DNS resolver (no local resolver leaks)
//   - Cookie jar
type ProfileClient struct {
	Profile *profile.Config
	http    *http.Client
}

// NewClient constructs a fully configured ProfileClient from the given profile
// configuration.
func NewClient(cfg *profile.Config) (*ProfileClient, error) {
	// -----------------------------------------------------------------
	// 1. Proxy dialer
	// -----------------------------------------------------------------
	proxyDialer, err := BuildDialer(cfg)
	if err != nil {
		return nil, fmt.Errorf("network: build proxy dialer: %w", err)
	}

	// -----------------------------------------------------------------
	// 2. Bootstrap HTTP client for DoH (uses proxy if set).
	//    We need a plain http.Transport here because DoH itself is HTTP,
	//    and we have not wired up the full uTLS transport yet.
	// -----------------------------------------------------------------
	dohBootstrapTransport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return proxyDialer.Dial(network, addr)
		},
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       60 * time.Second,
	}
	dohHTTPClient := &http.Client{
		Transport: dohBootstrapTransport,
		Timeout:   20 * time.Second,
	}
	dohResolver := NewDoHResolver(dohHTTPClient, "https://cloudflare-dns.com/dns-query")

	// -----------------------------------------------------------------
	// 3. Build the profileTransport
	// -----------------------------------------------------------------
	pt := &profileTransport{
		proxyDialer: proxyDialer,
		tlsPreset:   cfg.Fingerprint.TLSPreset,
	}

	// The DialContext on the base transport routes plaintext TCP through the
	// proxy *and* resolves hostnames via DoH.
	baseDialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("dial: parse addr %q: %w", addr, err)
		}

		// Resolve hostname via DoH (skips system resolver → no leaks).
		resolvedHost := host
		if net.ParseIP(host) == nil {
			ips, err := dohResolver.LookupHost(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("dial: doh resolve %s: %w", host, err)
			}
			resolvedHost = ips[0]
		}

		return proxyDialer.Dial(network, net.JoinHostPort(resolvedHost, port))
	}

	base := &http.Transport{
		DialContext:           baseDialContext,
		DialTLSContext:        pt.dialTLSContext,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		ForceAttemptHTTP2:     false, // HTTP/2 fingerprinting is separate
	}
	pt.base = base

	// -----------------------------------------------------------------
	// 4. Cookie jar (isolated per profile)
	// -----------------------------------------------------------------
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("network: create cookie jar: %w", err)
	}

	httpClient := &http.Client{
		Transport: pt,
		Jar:       jar,
		Timeout:   90 * time.Second,
	}

	return &ProfileClient{
		Profile: cfg,
		http:    httpClient,
	}, nil
}

// Get performs a GET request with all profile-derived headers injected.
func (c *ProfileClient) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("client: build GET %s: %w", url, err)
	}
	return c.Do(req)
}

// Do performs req after injecting profile headers.
func (c *ProfileClient) Do(req *http.Request) (*http.Response, error) {
	c.injectHeaders(req)
	return c.http.Do(req)
}

// injectHeaders sets standard browser headers derived from the profile
// fingerprint so every request looks like it came from the spoofed browser.
func (c *ProfileClient) injectHeaders(req *http.Request) {
	fp := c.Profile.Fingerprint

	// User-Agent
	if fp.UserAgent != "" {
		req.Header.Set("User-Agent", fp.UserAgent)
	}

	// Accept-Language – build from language list, e.g. "en-US,en;q=0.9"
	if len(fp.Languages) > 0 {
		req.Header.Set("Accept-Language", buildAcceptLanguage(fp.Languages))
	}

	// Standard browser Accept header
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")

	// Compression support
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")

	// Sec-Ch-Ua (only set for Chromium-based presets)
	if secChUa := buildSecChUa(fp.TLSPreset); secChUa != "" {
		req.Header.Set("Sec-Ch-Ua", secChUa)
		req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
		req.Header.Set("Sec-Ch-Ua-Platform", `"`+fp.Platform+`"`)
	}
}

// buildAcceptLanguage converts a language slice like ["en-US", "en"] into the
// Accept-Language header value "en-US,en;q=0.9".
func buildAcceptLanguage(langs []string) string {
	if len(langs) == 0 {
		return "en-US,en;q=0.9"
	}
	if len(langs) == 1 {
		return langs[0]
	}

	parts := make([]string, len(langs))
	parts[0] = langs[0]
	for i, lang := range langs[1:] {
		// Decrement quality by 0.1 for each subsequent tag, minimum 0.1.
		q := 1.0 - float64(i+1)*0.1
		if q < 0.1 {
			q = 0.1
		}
		parts[i+1] = fmt.Sprintf("%s;q=%.1f", lang, q)
	}
	return strings.Join(parts, ",")
}

// buildSecChUa returns the Sec-Ch-Ua header value for Chromium presets.
// Returns "" for Firefox/Safari where this header is absent.
func buildSecChUa(preset string) string {
	switch preset {
	case "chrome_120":
		return `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`
	default:
		return ""
	}
}
