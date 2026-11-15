package httpapi

import (
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/AlexGalhardo/vukem-video/service/config"
)

// Network is the part of the guard that network mode replaces (SDD-007 section
// 4.3): the effective scheme, and the Host and Origin of APP_URL in place of the
// loopback ones. A nil *Network is local mode.
type Network struct {
	host   string // lower-case host, with the port unless it is the default of https
	origin string // scheme://host[:port], as a browser sends it
	// loopbackHTTP is the container on the user's own machine (4.3 item 4):
	// plain HTTP is accepted, and only as the loopback name of APP_URL.
	loopbackHTTP bool
	trustProto   bool
	proxies      []netip.Prefix
	log          *slog.Logger
}

// NewNetwork builds the network guard from a valid configuration (config.Validate).
func NewNetwork(res config.Resolved, log *slog.Logger) (*Network, error) {
	u, err := url.Parse(res.AppURL)
	if err != nil {
		return nil, err
	}

	if log == nil {
		log = slog.Default()
	}

	host := strings.ToLower(u.Host)
	if u.Scheme == "https" {
		host = strings.TrimSuffix(host, ":443")
	}

	return &Network{
		host:         host,
		origin:       u.Scheme + "://" + host,
		loopbackHTTP: res.LoopbackHTTP,
		trustProto:   res.TrustForwardedProto,
		proxies:      res.TrustedProxies,
		log:          log,
	}, nil
}

// logStartup writes the lines an operator needs: the container rule, and that
// no user exists yet.
func (n *Network) logStartup(noUsers bool) {
	if n.loopbackHTTP {
		n.log.Info("network mode over plain HTTP: the server is reachable only as "+n.origin+
			"; any other use needs TLS (set an https APP_URL and TLS_CERT_FILE and TLS_KEY_FILE, or a proxy)",
			"app_url", n.origin)
	}

	if !noUsers {
		return
	}

	n.log.Warn("network mode has no users yet: every API request is answered 401; " +
		"create the first administrator with `vukem-video user:create <email> --admin` while the server is " +
		"stopped, or start it with VUKEM_VIDEO_ADMIN_EMAIL and VUKEM_VIDEO_ADMIN_PASSWORD")
}

// allowed is steps 1 to 3 of local mode replaced (4.3 item 3) plus steps 4 and
// 5 with the origin of APP_URL.
func (n *Network) allowed(r *http.Request) bool {
	if r.URL.Path == healthPath && peerAllowed(r) && !hasRelay(r) {
		return fetchSiteAllowed(r)
	}

	return n.schemeAllowed(r) && strings.EqualFold(r.Host, n.host) && fetchSiteAllowed(r) && n.originAllowed(r)
}

func (n *Network) originAllowed(r *http.Request) bool {
	origin, present := r.Header["Origin"]
	if !present {
		return true
	}

	return len(origin) == 1 && origin[0] == n.origin
}

// schemeAllowed refuses a request whose effective scheme is not https, unless
// the container rule admits plain HTTP. A forwarded scheme counts only from a
// trusted proxy with TRUST_FORWARDED_PROTO, and a request that claims http
// through the header is refused whoever sends it.
func (n *Network) schemeAllowed(r *http.Request) bool {
	fwd := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")))
	if fwd != "" && fwd != "https" {
		return false
	}

	scheme := "http"

	switch {
	case r.TLS != nil:
		scheme = "https"
	case n.trustProto && n.fromProxy(r) && fwd == "https":
		scheme = "https"
	}

	return scheme == "https" || n.loopbackHTTP
}

func (n *Network) fromProxy(r *http.Request) bool {
	a, ok := peerAddr(r)
	if !ok {
		return false
	}

	for _, p := range n.proxies {
		if p.Contains(a) {
			return true
		}
	}

	return false
}
