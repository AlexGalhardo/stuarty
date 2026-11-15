package outbound

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/AlexGalhardo/vukem/httpclient"
)

// Purpose names why a connection is opened (SDD-007 section 10.1).
type Purpose string

// The purposes that reach a host chosen by the operator or by the embedded manifest.
const (
	PurposeHelperDownload Purpose = "helper_download"
	PurposeTranscribeAPI  Purpose = "transcribe_api"
)

// Errors of the purposes below.
var (
	ErrScheme       = errors.New("outbound: the URL scheme is not allowed")
	ErrHost         = errors.New("outbound: the host is not allowed")
	ErrRedirect     = errors.New("outbound: redirect refused")
	ErrBlockedDial  = errors.New("outbound: the address is not a public address")
	ErrInvalidURL   = errors.New("outbound: the endpoint is not valid")
	maxRedirectsCap = 5
)

// DialRecorder sees every connection attempt of a purpose, with the resolved address, before it is
// made; an error stops the attempt. It exists for TestSecurity_T10 and the tests of the paths of
// section 10.1 (zero dials when nothing is configured).
type DialRecorder func(p Purpose, address string) error

var recorder atomic.Pointer[DialRecorder]

// SetDialRecorder installs the recorder and returns the function that removes it.
func SetDialRecorder(r DialRecorder) (restore func()) {
	prev := recorder.Swap(&r)

	return func() { recorder.Store(prev) }
}

func record(p Purpose, address string) error {
	if r := recorder.Load(); r != nil && *r != nil {
		return (*r)(p, address)
	}

	return nil
}

// TestOptions are for tests with a local server only: a CA to trust and permission to reach loopback.
// Production code never sets them.
type TestOptions struct {
	RootCAs       *x509.CertPool
	AllowLoopback bool
}

// HelperOptions configure the client of the helper download.
type HelperOptions struct {
	// Hosts is the allowlist of host names, from the manifest. Every request and every redirect hop is
	// checked against it.
	Hosts []string
	// Timeout bounds the whole download.
	Timeout time.Duration
	// MaxRedirects is at most 5.
	MaxRedirects int
	Test         *TestOptions
}

// HelperDownload is the client of the install step (purpose helper_download): https only, public
// addresses only, hosts from the manifest, at most 5 redirects each checked, no proxy. The caller caps
// the body at the size of the manifest.
func HelperDownload(o HelperOptions) *http.Client {
	maxRedirects := maxRedirectsCap
	if o.MaxRedirects > 0 {
		maxRedirects = min(o.MaxRedirects, maxRedirectsCap)
	}
	hosts := slices.Clone(o.Hosts)

	dialer := &net.Dialer{
		Timeout: o.Timeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			if err := record(PurposeHelperDownload, address); err != nil {
				return err
			}

			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return fmt.Errorf("%w: %s", ErrBlockedDial, address)
			}

			allow := []netip.Prefix(nil)
			if o.Test != nil && o.Test.AllowLoopback {
				allow = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")}
			}

			if !httpclient.Allowed(ap.Addr(), allow) {
				return fmt.Errorf("%w: %s", ErrBlockedDial, address)
			}

			return nil
		},
	}

	transport := &http.Transport{
		Proxy: nil, DialContext: dialer.DialContext, DisableKeepAlives: true,
		TLSClientConfig: testTLS(o.Test),
	}

	return &http.Client{
		Timeout: o.Timeout,
		Transport: &checkedTransport{
			rt:      transport,
			allowed: func(u *url.URL) error { return checkHost(u, hosts, false) },
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return fmt.Errorf("%w: more than %d redirects", ErrRedirect, maxRedirects)
			}

			return checkHost(req.URL, hosts, false)
		},
	}
}

// TranscribeAPI is the client of the transcription API provider (purpose transcribe_api). It reaches the
// host of the endpoint and no other, over https (http only for a loopback host), verifies TLS and
// follows no redirect. The operator chose the endpoint, so any address is allowed for that host.
func TranscribeAPI(endpoint string, timeout time.Duration, test *TestOptions) (*http.Client, error) {
	u, err := ParseEndpoint(endpoint)
	if err != nil {
		return nil, err
	}

	hosts := []string{u.Hostname()}
	dialer := &net.Dialer{
		Timeout: timeout,
		Control: func(_, address string, _ syscall.RawConn) error { return record(PurposeTranscribeAPI, address) },
	}
	transport := &http.Transport{
		Proxy: nil, DialContext: dialer.DialContext, DisableKeepAlives: true, TLSClientConfig: testTLS(test),
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &checkedTransport{
			rt: transport, allowed: func(r *url.URL) error { return checkHost(r, hosts, true) },
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}

// ParseEndpoint checks VUKEM_VIDEO_TRANSCRIBE_URL: https, or http for a loopback host, no userinfo, no
// query or fragment (SDD-007 section 4.5).
func ParseEndpoint(endpoint string) (*url.URL, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return nil, fmt.Errorf("%w: %q", ErrInvalidURL, "unreadable")
	}

	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("%w: userinfo, query and fragment are not allowed", ErrInvalidURL)
	}

	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopbackHost(u.Hostname()) {
			return nil, fmt.Errorf("%w: http is only for a loopback host", ErrScheme)
		}
	default:
		return nil, fmt.Errorf("%w: %q", ErrScheme, u.Scheme)
	}

	return u, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip, err := netip.ParseAddr(host)

	return err == nil && ip.Unmap().IsLoopback()
}

// checkHost refuses a URL whose scheme or host is not allowed. allowLoopbackHTTP lets http through for
// a loopback host (the API provider).
func checkHost(u *url.URL, hosts []string, allowLoopbackHTTP bool) error {
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && allowLoopbackHTTP && isLoopbackHost(u.Hostname()):
	default:
		return fmt.Errorf("%w: %q", ErrScheme, u.Scheme)
	}

	if u.User != nil {
		return fmt.Errorf("%w: userinfo", ErrHost)
	}

	if !slices.ContainsFunc(hosts, func(h string) bool { return strings.EqualFold(h, u.Hostname()) }) {
		return fmt.Errorf("%w: %s", ErrHost, u.Hostname())
	}

	return nil
}

func testTLS(t *TestOptions) *tls.Config {
	if t == nil || t.RootCAs == nil {
		return nil
	}

	return &tls.Config{RootCAs: t.RootCAs, MinVersion: tls.VersionTLS12}
}

// checkedTransport applies the host and scheme rules to every request before it reaches the dialer.
type checkedTransport struct {
	rt      http.RoundTripper
	allowed func(*url.URL) error
}

func (t *checkedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.allowed(req.URL); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}

		return nil, err
	}

	return t.rt.RoundTrip(req)
}
