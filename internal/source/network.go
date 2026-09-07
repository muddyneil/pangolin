package source

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// PublicHost reports whether every address currently returned for host is
// publicly routable. Rejecting mixed public/private answers avoids selecting a
// private address through DNS rotation.
func PublicHost(ctx context.Context, host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("empty host")
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		if !isPublicIP(ip) {
			return fmt.Errorf("host resolves to a private or local address")
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("failed to resolve host: %w", err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("host has no addresses")
	}
	for _, ip := range ips {
		if !isPublicIP(ip) {
			return fmt.Errorf("host resolves to a private or local address")
		}
	}
	return nil
}

func isPublicIP(ip net.IP) bool {
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified()
}

// PublicDialContext resolves the destination immediately before connecting and
// refuses private answers, covering redirects and DNS rotation for source
// downloads.
func PublicDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if !isPublicIP(ip) {
			return nil, fmt.Errorf("refusing connection to private or local address")
		}
	}
	var dialer net.Dialer
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("failed to connect to %s", host)
}
