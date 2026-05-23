package network

import (
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/alterego/browser/internal/profile"
	"golang.org/x/net/proxy"
)

// BuildDialer creates a proxy.Dialer that routes connections through the
// proxy described in cfg. If no proxy is configured, proxy.Direct is returned.
func BuildDialer(cfg *profile.Config) (proxy.Dialer, error) {
	if cfg.Proxy.Type == "" || cfg.Proxy.Host == "" {
		return proxy.Direct, nil
	}

	proxyURL, err := parseProxyURL(cfg.Proxy)
	if err != nil {
		return nil, fmt.Errorf("proxy: build dialer: %w", err)
	}

	// golang.org/x/net/proxy supports both SOCKS5 and HTTP CONNECT via
	// proxy.FromURL, so we can use the same code path for both proxy types.
	dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("proxy: from url %s: %w", proxyURL.Redacted(), err)
	}

	return dialer, nil
}

// parseProxyURL converts a ProxyConfig into a *url.URL suitable for
// proxy.FromURL.
func parseProxyURL(pc profile.ProxyConfig) (*url.URL, error) {
	if pc.Host == "" {
		return nil, fmt.Errorf("proxy: host is empty")
	}

	proxyType := pc.Type
	if proxyType == "" {
		proxyType = "socks5"
	}

	raw := fmt.Sprintf("%s://%s:%d", proxyType, pc.Host, pc.Port)

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("proxy: parse url %q: %w", raw, err)
	}

	if pc.User != "" || pc.Pass != "" {
		u.User = url.UserPassword(pc.User, pc.Pass)
	}

	return u, nil
}

// DirectDialer returns a plain *net.Dialer with a 30-second timeout.
// Use this when no proxy is configured and you need a concrete *net.Dialer.
func DirectDialer() *net.Dialer {
	return &net.Dialer{
		Timeout: 30 * time.Second,
	}
}
