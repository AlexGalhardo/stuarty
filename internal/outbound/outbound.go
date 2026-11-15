// Package outbound is the only place the product builds network clients
// (SDD-007 section 10.2): every other package that dials or sends HTTP is a
// lint error. Each constructor names a purpose from the list of section 10.1.
package outbound

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
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

// RemoteServer is the client of the CLI and the stdio MCP server towards the
// server named by VUKEM_VIDEO_URL (SDD-007 section 10.1, path 4). The operator
// chose that address, so it may be any host; TLS verification stays on, no proxy
// is used and no redirect is followed, so the credential goes to that host only.
func RemoteServer(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// ErrHostNotAllowed is a helper download that leaves the allowlist or is not HTTPS.
var ErrHostNotAllowed = errors.New("outbound: the host is not on the download allowlist")

// ErrNotPublic is a dial to an address that is not a public one.
var ErrNotPublic = errors.New("outbound: the address is not public")

const maxDownloadRedirects = 5

// HelperDownload is the client of the install step (purpose helper_download,
// SDD-007 section 10.1 and 6.8 item 4). It speaks HTTPS only, to the hosts of
// the manifest only, at every redirect hop, follows at most 5 redirects, uses
// no proxy and dials public addresses only. Nothing but the explicit install
// step builds it.
func HelperDownload(hosts []string, timeout time.Duration) *http.Client {
	allowed := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		allowed[strings.ToLower(h)] = true
	}

	check := func(r *http.Request) error {
		if r.URL.Scheme != "https" || !allowed[strings.ToLower(r.URL.Hostname())] {
			return ErrHostNotAllowed
		}

		return nil
	}

	dialer := &net.Dialer{
		Timeout: timeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return ErrNotPublic
			}

			ip := ap.Addr().Unmap()
			if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				return ErrNotPublic
			}

			return nil
		},
	}

	return &http.Client{
		Timeout: timeout,
		Transport: checkedTransport{
			base:  &http.Transport{DialContext: dialer.DialContext, Proxy: nil, DisableKeepAlives: true},
			check: check,
		},
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) > maxDownloadRedirects {
				return http.ErrUseLastResponse
			}

			return check(r)
		},
	}
}

// checkedTransport applies the allowlist to the first request as well as to
// every redirect.
type checkedTransport struct {
	base  http.RoundTripper
	check func(*http.Request) error
}

func (t checkedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := t.check(r); err != nil {
		return nil, err
	}

	return t.base.RoundTrip(r)
}
