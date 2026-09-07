package source

import (
	"context"
	"testing"
)

func TestPublicHostRejectsLocalAddresses(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "10.0.0.1", "::1", "169.254.169.254", "localhost"} {
		t.Run(host, func(t *testing.T) {
			if err := PublicHost(context.Background(), host); err == nil {
				t.Fatalf("expected local host %q to be rejected", host)
			}
		})
	}
}

func TestPublicHostAcceptsPublicLiteral(t *testing.T) {
	if err := PublicHost(context.Background(), "203.0.113.1"); err != nil {
		t.Fatalf("expected public literal to be accepted: %v", err)
	}
}
