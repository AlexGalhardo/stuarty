// Package outbound is the only place the product builds network clients
// (SDD-007 section 10.2): every other package that dials or sends HTTP is a
// lint error. Each constructor names a purpose from the list of section 10.1.
package outbound

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// ErrNotLoopback is a dial to an address that is not loopback.
var ErrNotLoopback = errors.New("outbound: the address is not loopback")

// LocalServer is the client of the CLI and the stdio MCP server towards the
// owner process (SDD-007 section 10.1, path 4). It dials loopback addresses
// only, whatever name it is given, and follows no redirect.
func LocalServer(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout: timeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil || !ap.Addr().Unmap().IsLoopback() {
				return ErrNotLoopback
			}

			return nil
		},
	}

	return &http.Client{
		Timeout:       timeout,
		Transport:     &http.Transport{DialContext: dialer.DialContext, Proxy: nil, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
