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

// The IPv6 forms that carry an IPv4 address a gateway or a stack turns into that IPv4 one, so the
// embedded address is the one judged (SDD-014 section 6): the well-known NAT64 prefix (RFC 6052,
// 64:ff9b::a.b.c.d), 6to4 (RFC 3056, 2002:aabb:ccdd::, the IPv4 in bits 16 to 47) and the deprecated
// IPv4-compatible form (::a.b.c.d). The local-use NAT64 prefix (RFC 8215) embeds the IPv4 at a
// position that depends on the deployment, so the whole prefix is refused. Teredo (2001::/32) and an
// ISATAP interface identifier (::5efe:a.b.c.d) are not decoded: they are tunnels, and the IPv4 address
// they carry receives an encapsulated packet, never the connection, so no service there is reached.
var (
	nat64      = netip.MustParsePrefix("64:ff9b::/96")
	nat64Local = netip.MustParsePrefix("64:ff9b:1::/48")
	sixToFour  = netip.MustParsePrefix("2002::/16")
	v4Compat   = netip.MustParsePrefix("::/96")
)

// embeddedIPv4 returns the IPv4 address an IPv6 one carries, when it is one of the forms above.
func embeddedIPv4(ip netip.Addr) (netip.Addr, bool) {
	b := ip.As16()

	switch {
	case nat64.Contains(ip), v4Compat.Contains(ip):
		return netip.AddrFrom4([4]byte(b[12:])), true
	case sixToFour.Contains(ip):
		return netip.AddrFrom4([4]byte(b[2:6])), true
	}

	return netip.Addr{}, false
}

// refusedAtDial says whether the gate refuses an address it is about to dial. The zone is dropped
// first: fd00:ec2::254%eth0 is still the metadata address.
func refusedAtDial(addr netip.Addr) bool {
	ip := addr.Unmap().WithZone("")
	if ip.Is6() && nat64Local.Contains(ip) {
		return true
	}

	if ip.Is6() {
		if v4, ok := embeddedIPv4(ip); ok {
			ip = v4
		}
	}

	return ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() ||
		ip.IsMulticast() || metadataAddrs[ip]
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

			if refusedAtDial(ap.Addr()) {
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
