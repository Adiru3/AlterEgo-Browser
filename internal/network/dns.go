package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// dohResponse is the JSON shape returned by Cloudflare's DNS-over-HTTPS API.
type dohResponse struct {
	Status int `json:"Status"`
	Answer []struct {
		Name string `json:"name"`
		Type int    `json:"type"`
		TTL  int    `json:"TTL"`
		Data string `json:"data"`
	} `json:"Answer"`
}

// DoHResolver resolves DNS queries over HTTPS so that all DNS traffic is
// tunnelled through the profile's proxy and never leaks to the local resolver.
type DoHResolver struct {
	client   *http.Client
	provider string // DoH provider base URL, e.g. "https://cloudflare-dns.com/dns-query"
}

// NewDoHResolver constructs a DoHResolver backed by the given HTTP client and
// DoH provider URL. The client should already route through the proxy.
func NewDoHResolver(client *http.Client, provider string) *DoHResolver {
	if provider == "" {
		provider = "https://cloudflare-dns.com/dns-query"
	}
	return &DoHResolver{client: client, provider: provider}
}

// LookupHost resolves hostname to a slice of IPv4/IPv6 address strings using
// the DNS-over-HTTPS API.
func (r *DoHResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	// If the caller already supplied an IP address, return it immediately.
	if ip := net.ParseIP(host); ip != nil {
		return []string{ip.String()}, nil
	}

	reqURL := fmt.Sprintf("%s?name=%s&type=A", r.provider, host)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("doh: build request: %w", err)
	}
	req.Header.Set("Accept", "application/dns-json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doh: query %s: %w", host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("doh: unexpected status %d for %s", resp.StatusCode, host)
	}

	var doh dohResponse
	if err := json.NewDecoder(resp.Body).Decode(&doh); err != nil {
		return nil, fmt.Errorf("doh: decode response: %w", err)
	}

	// DNS status 0 == NOERROR.
	if doh.Status != 0 {
		return nil, fmt.Errorf("doh: DNS error status %d for %s", doh.Status, host)
	}

	// Type 1 == A record (IPv4).  Collect all A-record answers.
	var addrs []string
	for _, ans := range doh.Answer {
		if ans.Type == 1 {
			addrs = append(addrs, ans.Data)
		}
	}

	if len(addrs) == 0 {
		return nil, fmt.Errorf("doh: no A records for %s", host)
	}

	return addrs, nil
}

// ToDialContext returns a DialContext function suitable for use in
// http.Transport.DialContext. It resolves hostnames via DoH before dialling so
// that the system resolver is never used and no DNS leaks occur through the
// proxy.
func (r *DoHResolver) ToDialContext() func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 30 * time.Second}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("doh dialer: parse addr %q: %w", addr, err)
		}

		// If host is already an IP skip the DoH lookup.
		resolvedHost := host
		if net.ParseIP(host) == nil {
			ips, err := r.LookupHost(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("doh dialer: resolve %s: %w", host, err)
			}
			resolvedHost = ips[0]
		}

		return dialer.DialContext(ctx, network, net.JoinHostPort(resolvedHost, port))
	}
}
