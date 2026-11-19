package outbound

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"syscall"
	"time"
)

// PurposeConnection is a request through a destination the user connected
// (SDD-014 section 6, SDD-007 section 10.1 path 7).
const PurposeConnection Purpose = "connection"

// The refusals of the gate. A caller maps them to E_CONNECTION_NOT_FOUND and
// E_CONNECTION_DESTINATION; no socket is opened for either.
var (
	// ErrNoConnection is a request through a connection that does not exist or was removed.
	ErrNoConnection = errors.New("outbound: the connection does not exist")
	// ErrDestination is a request for a host or a scheme the connection does not allow.
	ErrDestination = errors.New("outbound: the destination is not one of the connection")
)

// Destination is what the gate needs to know about a connection: where it may go.
type Destination struct {
	ID    string
	Hosts []string
}

// Registry is the list of connections the gate asks at every request, so a
// connection removed a moment ago stops traffic at once.
type Registry interface {
	// Destination returns the connection, or an error wrapping ErrNoConnection.
	Destination(ctx context.Context, id string) (Destination, error)
	// Used records that a request went through the connection.
	Used(ctx context.Context, id string, at time.Time)
}

// metadataAddrs are the cloud metadata services that sit outside the ranges the gate refuses as a
// whole: Alibaba Cloud in the shared (CGNAT) block and AWS over IPv6 in the unique-local block. The
// blocks stay allowed (a self-hosted user reaches a service on a tailnet or a private network), so the
// two addresses are named (SDD-014 section 6). The 169.254.0.0/16 services (AWS, GCP, Azure, ...) and
// fe80::/10 are covered by the link-local rule.
var metadataAddrs = map[netip.Addr]bool{
	netip.MustParseAddr("100.100.100.200"): true,
	netip.MustParseAddr("fd00:ec2::254"):   true,
}

// Connection is the client of a destination the user connected. Every request
// and every redirect is checked against the registry at that moment: the
// connection must exist, the scheme must be https (http for a loopback host
// only), the URL carries no userinfo, and its host is in the list of the
// connection. No redirect is followed, no proxy is used, TLS is verified, and
// at dial time an address that is link-local, unspecified or multicast, or a
// named cloud metadata address, is refused, so a name that resolves to one is not reachable. Loopback
// and private addresses are allowed: a self-hosted server on the same network
// is a destination the user typed.
func Connection(reg Registry, id string, timeout time.Duration, test *TestOptions) *http.Client {
	dialer := &net.Dialer{
		Timeout: timeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return ErrNotPublic
			}

			ip := ap.Addr().Unmap()
			if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() ||
				ip.IsMulticast() || metadataAddrs[ip] {
				return ErrNotPublic
			}

			return record(PurposeConnection, address)
		},
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &connectionTransport{
			reg: reg, id: id,
			base: &http.Transport{
				Proxy: nil, DialContext: dialer.DialContext, DisableKeepAlives: true, TLSClientConfig: testTLS(test),
			},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

type connectionTransport struct {
	reg  Registry
	id   string
	base http.RoundTripper
}

func (t *connectionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	closeBody := func() {
		if req.Body != nil {
			_ = req.Body.Close()
		}
	}

	dest, err := t.reg.Destination(req.Context(), t.id)
	if err != nil {
		closeBody()

		if errors.Is(err, ErrNoConnection) {
			return nil, err
		}

		return nil, fmt.Errorf("%w: %w", ErrNoConnection, err)
	}

	if err := checkDestination(req.URL, dest.Hosts); err != nil {
		closeBody()

		return nil, err
	}

	t.reg.Used(req.Context(), t.id, time.Now())

	return t.base.RoundTrip(req)
}

// checkDestination is the scheme and host rule of the gate.
func checkDestination(u *url.URL, hosts []string) error {
	host := normalizeHost(u.Hostname())

	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && isLoopbackHost(host):
	default:
		return fmt.Errorf("%w: the scheme %q", ErrDestination, u.Scheme)
	}

	if u.User != nil || host == "" {
		return fmt.Errorf("%w: userinfo or no host", ErrDestination)
	}

	if !slices.ContainsFunc(hosts, func(h string) bool { return normalizeHost(h) == host }) {
		return fmt.Errorf("%w: %s", ErrDestination, host)
	}

	return nil
}

// NormalizeHost is the form in which hosts are compared and stored: lower
// case, without a trailing dot.
func NormalizeHost(h string) string { return normalizeHost(h) }

func normalizeHost(h string) string { return strings.TrimSuffix(strings.ToLower(h), ".") }
