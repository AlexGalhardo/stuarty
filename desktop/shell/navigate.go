// Package shell holds the parts of the desktop shell that do not depend on a
// window toolkit: the child process and the navigation policy. It imports no
// GUI package and nothing of core/ or engine/ (ADR-0023, SDD-012 section 21).
package shell

import (
	"net"
	"net/url"
	"strconv"
)

// Allowed reports whether the window may navigate to rawURL: only
// http://127.0.0.1:<port> or http://localhost:<port>, the port of the server
// the shell started (SDD-012 section 21.3, RF-213). Every other scheme (file:,
// data:, blob:, https:, ws:), host, port and any URL with credentials is refused.
func Allowed(rawURL string, port int) bool {
	u, err := url.Parse(rawURL)
	if err != nil || port <= 0 || u.Scheme != "http" || u.Opaque != "" || u.User != nil {
		return false
	}
	switch u.Hostname() {
	case "127.0.0.1", "localhost":
	default:
		return false
	}
	return u.Port() == strconv.Itoa(port) && net.JoinHostPort(u.Hostname(), u.Port()) == u.Host
}
