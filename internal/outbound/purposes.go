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
)

// Purpose names why a connection is opened (SDD-007 section 10.1).
type Purpose string

// The purposes that reach a host chosen by the operator or by the embedded manifest.
const (
	PurposeHelperDownload Purpose = "helper_download"
	PurposeTranscribeAPI  Purpose = "transcribe_api"
	PurposeDesktopUpdate  Purpose = "desktop_update"
)

// Errors of the purposes below.
var (
	ErrScheme     = errors.New("outbound: the URL scheme is not allowed")
	ErrHost       = errors.New("outbound: the host is not allowed")
	ErrRedirect   = errors.New("outbound: redirect refused")
	ErrInvalidURL = errors.New("outbound: the endpoint is not valid")
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

// TestOptions are for tests with a local server only: a CA to trust. Production code never sets them.
type TestOptions struct {
	RootCAs *x509.CertPool
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
		Transport: &hostCheckedTransport{
			rt: transport, allowed: func(r *url.URL) error { return checkHost(r, hosts, true) },
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}

// DesktopUpdate is the client of the desktop update check (purpose desktop_update, SDD-009 section 10.5).
// It reaches the host of the feed URL and no other, over https only (no loopback http exception), verifies
// TLS, uses no proxy and follows no redirect. Only the desktop update client builds it, and only on a
// request of the person or with the setting on.
func DesktopUpdate(feedURL string, timeout time.Duration, test *TestOptions) (*http.Client, error) {
	u, err := url.Parse(feedURL)
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return nil, fmt.Errorf("%w: %q", ErrInvalidURL, "unreadable")
	}

	if err := checkHost(u, []string{u.Hostname()}, false); err != nil {
		return nil, err
	}

	hosts := []string{u.Hostname()}
	dialer := &net.Dialer{
		Timeout: timeout,
		Control: func(_, address string, _ syscall.RawConn) error { return record(PurposeDesktopUpdate, address) },
	}
	transport := &http.Transport{
		Proxy: nil, DialContext: dialer.DialContext, DisableKeepAlives: true, TLSClientConfig: testTLS(test),
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &hostCheckedTransport{
			rt: transport, allowed: func(r *url.URL) error { return checkHost(r, hosts, false) },
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

// hostCheckedTransport applies the host and scheme rules to every request before it reaches the dialer.
type hostCheckedTransport struct {
	rt      http.RoundTripper
	allowed func(*url.URL) error
}

func (t *hostCheckedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.allowed(req.URL); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}

		return nil, err
	}

	return t.rt.RoundTrip(req)
}
