package app

import (
	"net"
	"slices"
	"sync"
)

// The addresses this process is really answering on.
//
// The server may listen on more than one address (loopback, plus a Tailscale or
// LAN address - see bindAddrs), and an extra listener is best-effort: an
// interface that is down leaves that address unbound while the server carries on
// serving locally. So the browser cannot work the list out for itself, and a
// guess would name an address nothing answers on. Each listener that binds
// records itself here, and GET /api/health hands the list to whoever asks - the
// research brief in the page uses it to tell an assistant which address to use
// from where.
var (
	servedMu   sync.RWMutex
	servedList []string
)

// recordServed notes one TCP address that has just been bound successfully, as
// the base URLs a client can reach it at. Called once per listener, only after
// net.Listen returned without error, so the list holds what is being served and
// never what was merely attempted.
func recordServed(addr string) {
	servedMu.Lock()
	defer servedMu.Unlock()
	for _, u := range reachableURLs(addr) {
		if !slices.Contains(servedList, u) {
			servedList = append(servedList, u)
		}
	}
}

// servedAddresses returns the base URLs this server answers on, in the order
// their listeners bound - loopback first, because that listener is the required
// one. The result is a copy, so a caller may hold or encode it freely. It is
// empty in a process that never started a listener (a test, or a data
// subcommand), which is the honest answer there.
func servedAddresses() []string {
	servedMu.RLock()
	defer servedMu.RUnlock()
	out := make([]string, len(servedList))
	copy(out, servedList)
	return out
}

// reachableURLs turns one bound TCP address into the base URLs a client can
// actually type. A specific address is itself. A WILDCARD address (0.0.0.0 or
// ::) is not a destination anybody can send to, so it expands to the addresses
// that bind really answers on: loopback, plus every address of this machine's
// interfaces. Loopback and link-local addresses from the interface list are left
// out - loopback is already there, and a link-local address needs a zone nobody
// reading a brief can supply.
func reachableURLs(addr string) []string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsUnspecified() {
		return []string{"http://" + net.JoinHostPort(host, port)}
	}

	urls := []string{"http://" + net.JoinHostPort("127.0.0.1", port)}
	ipv4Only := ip.To4() != nil
	ifaceAddrs, err := net.InterfaceAddrs()
	if err != nil {
		return urls
	}
	for _, a := range ifaceAddrs {
		n, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if n.IP.IsLoopback() || n.IP.IsLinkLocalUnicast() || n.IP.IsLinkLocalMulticast() || n.IP.IsUnspecified() {
			continue
		}
		if ipv4Only && n.IP.To4() == nil {
			continue
		}
		u := "http://" + net.JoinHostPort(n.IP.String(), port)
		if !slices.Contains(urls, u) {
			urls = append(urls, u)
		}
	}
	return urls
}
