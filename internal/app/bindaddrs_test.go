package app

import (
	"slices"
	"testing"
)

// TestBindAddrs verifies the address set the server listens on: loopback is
// always present so local tooling and the single-instance port probe keep
// working, a specific non-loopback host is added as a second listener, and a
// wildcard host binds alone (it already covers loopback).
func TestBindAddrs(t *testing.T) {
	cases := []struct {
		name string
		host string
		port int
		want []string
	}{
		{"default loopback", "127.0.0.1", 8122, []string{"127.0.0.1:8122"}},
		{"empty host", "", 8122, []string{"127.0.0.1:8122"}},
		{"localhost alias", "localhost", 8122, []string{"127.0.0.1:8122"}},
		{"ipv6 loopback", "::1", 8122, []string{"127.0.0.1:8122"}},
		{"tailscale ip adds loopback", "100.85.27.66", 8122, []string{"127.0.0.1:8122", "100.85.27.66:8122"}},
		{"lan ip adds loopback", "192.168.1.5", 9000, []string{"127.0.0.1:9000", "192.168.1.5:9000"}},
		{"wildcard binds alone", "0.0.0.0", 8122, []string{"0.0.0.0:8122"}},
		{"ipv6 wildcard binds alone", "::", 8122, []string{"[::]:8122"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := bindAddrs(c.host, c.port)
			if !slices.Equal(got, c.want) {
				t.Errorf("bindAddrs(%q, %d) = %v, want %v", c.host, c.port, got, c.want)
			}
		})
	}
}
