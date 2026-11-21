package webhook

import (
	"net/netip"
	"net/url"
	"strings"
	"unicode"
)

// Host kinds of an address.
const (
	// HostLoopback is localhost or an address of the loopback block: the person chose this machine on purpose.
	HostLoopback = "loopback"
	// HostPublic is any other host: an IP address that is not in a refused range, or a name.
	HostPublic = "public"
)

// IsLoopbackHost reports whether a host names this machine on purpose: "localhost" or an IP literal of the loopback
// block. A name under .localhost or any other name is judged at dial time, as a public one.
func IsLoopbackHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" {
		return true
	}

	ip, err := netip.ParseAddr(host)

	return err == nil && ip.Unmap().IsLoopback()
}

// CheckURL applies the address rules of SDD-022 section 5 to the URL of a webhook when it is saved and returns the
// kind of its host. notPublic says whether an IP address is one a public destination may never be (private,
// link-local, shared, unspecified, multicast, metadata, or an IPv6 form that carries one): it is a parameter so that
// the ranges live in one place, internal/outbound, which also judges the address at dial time. A name is not
// resolved here.
func CheckURL(raw string, notPublic func(netip.Addr) bool) (string, *FieldError) {
	if raw == "" || len(raw) > MaxURLBytes {
		return "", bad("url", "the address is 1 to %d characters.", MaxURLBytes)
	}

	if strings.ContainsFunc(raw, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) {
		return "", bad("url", "the address has a space or a control character.")
	}

	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", bad("url", "the address is not a URL with a host.")
	}

	if u.User != nil {
		return "", bad("url", "the address has a user name or a password in it.")
	}

	if strings.Contains(raw, "#") {
		return "", bad("url", "the address has a fragment.")
	}

	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	loopback := IsLoopbackHost(host)

	switch u.Scheme {
	case "https":
	case "http":
		if !loopback {
			return "", bad("url", "http is only for this machine (localhost or 127.0.0.1): use an https address.")
		}
	default:
		return "", bad("url", "the address starts with https.")
	}

	if loopback {
		return HostLoopback, nil
	}

	ip, perr := netip.ParseAddr(host)
	if perr == nil && notPublic != nil && notPublic(ip) {
		return "", bad("url", "the address is on a private or internal network, which a webhook may not reach.")
	}

	if perr != nil && endsInNumber(host) {
		return "", bad("url", "the address is a number and not a name: write the host in full.")
	}

	for _, suffix := range []string{".localhost", ".internal", ".local"} {
		if strings.HasSuffix(host, suffix) {
			return "", bad("url", "the address is a name of this machine or of a private network.")
		}
	}

	return HostPublic, nil
}

// endsInNumber reports whether the last label of a host is a number, decimal, octal or 0x hexadecimal. No public name
// ends so, and a resolver or a browser reads such a host as an IPv4 address (2130706433, 0x7f.1, 127.1, 0177.0.0.1),
// so it is one of the forms of an address the range check would not see when the webhook is saved.
func endsInNumber(host string) bool {
	last := host[strings.LastIndexByte(host, '.')+1:]
	if digits, ok := strings.CutPrefix(last, "0x"); ok {
		return !strings.ContainsFunc(digits, func(r rune) bool { return !strings.ContainsRune("0123456789abcdef", r) })
	}

	return last != "" && !strings.ContainsFunc(last, func(r rune) bool { return r < '0' || r > '9' })
}

// SafeURL is the scheme and the host of an address, with no path, query or port: the path and the query of a
// receiver often are its token, so a surface that is not the admin one shows this form.
func SafeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return ""
	}

	host := u.Hostname()
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}

	return u.Scheme + "://" + host
}
