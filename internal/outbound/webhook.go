package outbound

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// PurposeWebhook is a delivery to an outbound webhook the user registered (SDD-022, SDD-007 section 10.1 path 9).
const PurposeWebhook Purpose = "webhook"

// ErrWebhookURL is an address a webhook may not have.
var ErrWebhookURL = errors.New("outbound: the webhook address is not allowed")

// Webhook is the client of one outbound webhook (SDD-022 section 5). It reaches the one address it was made for:
// every request must have that scheme, host and port, and no user name. TLS is verified, no proxy is used and no
// redirect is followed, so a public address cannot send the request anywhere else.
//
// At dial time, after the name was resolved, the address is judged again, which is what stops a name that was public
// when it was saved and is internal when it is dialled (DNS rebinding): a webhook whose host is not loopback refuses
// every address that is loopback, private, link-local, shared, unspecified, multicast or a named metadata address,
// and a webhook whose host is loopback (the person chose this machine on purpose) accepts loopback addresses only.
// The scheme is https, or http for a loopback host.
func Webhook(rawURL string, timeout time.Duration, test *TestOptions) (*http.Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" || u.User != nil {
		return nil, fmt.Errorf("%w: unreadable", ErrWebhookURL)
	}

	host := normalizeHost(u.Hostname())
	loopback := isLoopbackHost(host)

	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && loopback:
	default:
		return nil, fmt.Errorf("%w: the scheme %q", ErrWebhookURL, u.Scheme)
	}

	dialer := &net.Dialer{
		Timeout: timeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			if err := webhookDialCheck(loopback, address); err != nil {
				return err
			}

			return record(PurposeWebhook, address)
		},
	}

	dial := dialer.DialContext

	if test != nil && len(test.Resolve) > 0 {
		dial = func(ctx context.Context, network, address string) (net.Conn, error) {
			if h, port, err := net.SplitHostPort(address); err == nil {
				if ip, ok := test.Resolve[h]; ok {
					address = net.JoinHostPort(ip, port)
				}
			}

			return dialer.DialContext(ctx, network, address)
		}
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &webhookTransport{
			scheme: u.Scheme, host: host, port: effectivePort(u),
			base: &http.Transport{Proxy: nil, DialContext: dial, DisableKeepAlives: true, TLSClientConfig: testTLS(test)},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}

// webhookDialCheck is the judgement of an address about to be dialled: loopback only for a loopback webhook, a public
// address only for any other.
func webhookDialCheck(loopbackHost bool, address string) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return ErrNotPublic
	}

	ip := ap.Addr().Unmap().WithZone("")

	if loopbackHost {
		if !ip.IsLoopback() {
			return ErrNotLoopback
		}

		return nil
	}

	if NotPublicAddr(ip) {
		return ErrNotPublic
	}

	return nil
}

func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}

	if u.Scheme == "https" {
		return "443"
	}

	return "80"
}

type webhookTransport struct {
	scheme, host, port string
	base               http.RoundTripper
}

func (t *webhookTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		defer func() { _ = req.Body.Close() }()
	}

	u := req.URL
	if u.Scheme != t.scheme || normalizeHost(u.Hostname()) != t.host || effectivePort(u) != t.port || u.User != nil ||
		strings.ContainsAny(u.Host, " \t\r\n") {
		return nil, fmt.Errorf("%w: not the address of the webhook", ErrWebhookURL)
	}

	return t.base.RoundTrip(req)
}
