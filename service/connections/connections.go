// Package connections is the registry of the destinations the user connected
// (ADR-0032, SDD-014): the record, its validation, the vault entry that holds
// its credential and the connector kinds that use it. It is the only package
// that reads a credential, and it hands one to a closure that makes the
// outbound call (Manager.With); no result carries one.
package connections

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
	"github.com/AlexGalhardo/stuarty/service/vault"
)

// Limits of SDD-014 section 2.
const (
	MaxConnections = 50
	MaxLabel       = 60
	MaxHosts       = 16
	MaxEndpoint    = 2048
	// MaxSecret is the largest credential, the size of a vault entry.
	MaxSecret = vault.MaxSecretSize
)

// The data a connection may receive (SDD-014 section 2).
const (
	AcceptText     = "text"
	AcceptImage    = "image"
	AcceptVideo    = "video"
	AcceptAudio    = "audio"
	AcceptMetadata = "metadata"
	AcceptDocument = "document"
)

// Accepts lists every value of the accepts member.
var Accepts = []string{AcceptText, AcceptImage, AcceptVideo, AcceptAudio, AcceptMetadata, AcceptDocument}

// KindTest is the connector kind of the tests and of the Connections page.
const KindTest = "test"

// Errors of the registry. The service maps them to codes (SDD-014 section 7).
var (
	// ErrNotFound is a connection that does not exist or was removed.
	ErrNotFound = errors.New("connections: no such connection")
	// ErrNoSecret is a connection whose credential is not in the vault.
	ErrNoSecret = errors.New("connections: the credential of the connection is not in the vault")
)

// Connection is what a surface sees of a connection: never its secret, only
// whether one is set.
type Connection struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Label      string     `json:"label"        untrusted:"true"`
	Endpoint   string     `json:"endpoint"`
	Hosts      []string   `json:"hosts"`
	Accepts    []string   `json:"accepts"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	SecretSet  bool       `json:"secret_set"`
}

// Row is a connection as the table holds it.
type Row struct {
	ID         string
	Kind       string
	Label      string
	Endpoint   string
	Hosts      []string
	Accepts    []string
	SecretRef  string
	CreatedMs  int64
	CreatedBy  string
	LastUsedMs *int64
}

// Store is where the rows live (the table connections of SDD-014 section 8).
type Store interface {
	// List returns every connection, oldest first.
	List(ctx context.Context) ([]Row, error)
	// Get returns one, or ErrNotFound.
	Get(ctx context.Context, id string) (Row, error)
	// Count is the number of connections.
	Count(ctx context.Context) (int, error)
	// Insert adds a row.
	Insert(ctx context.Context, row Row) error
	// Delete removes a row; ErrNotFound when there is none.
	Delete(ctx context.Context, id string) error
	// Touch records a use; a row that is gone is not an error.
	Touch(ctx context.Context, id string, atMs int64) error
}

// Spec is what ConnectionCreate asks for, before validation.
type Spec struct {
	Kind     string
	Label    string
	Endpoint string
	Hosts    []string
	Accepts  []string
}

// FieldError is a validation failure; Path names the member of the request.
type FieldError struct {
	Path    string
	Message string
}

func (e *FieldError) Error() string { return e.Path + ": " + e.Message }

func bad(path, format string, args ...any) *FieldError {
	return &FieldError{Path: path, Message: fmt.Sprintf(format, args...)}
}

// Normalize checks a spec and returns it in stored form: the host list
// lower-cased and without duplicates, defaulting to the host of the endpoint,
// and the accepts list in the order of Accepts.
func Normalize(in Spec, kinds []string) (Spec, *FieldError) {
	out := Spec{Kind: strings.TrimSpace(in.Kind), Label: strings.TrimSpace(in.Label)}

	if !slices.Contains(kinds, out.Kind) {
		return out, bad("kind", "must be one of %s", strings.Join(kinds, ", "))
	}

	if n := len([]rune(out.Label)); n < 1 || n > MaxLabel || strings.ContainsFunc(out.Label, unicode.IsControl) {
		return out, bad("label", "must be 1 to %d characters with no control character", MaxLabel)
	}

	u, ferr := parseEndpoint(in.Endpoint)
	if ferr != nil {
		return out, ferr
	}

	out.Endpoint = u.String()

	hosts := make([]string, 0, len(in.Hosts)+1)
	hosts = append(hosts, outbound.NormalizeHost(u.Hostname()))

	for i, h := range in.Hosts {
		h = outbound.NormalizeHost(strings.TrimSpace(h))
		if !validHost(h) {
			return out, bad(
				fmt.Sprintf("hosts[%d]", i),
				"must be a host name or an IP address, with no scheme, port, path or wildcard",
			)
		}

		if !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}

	if len(hosts) > MaxHosts {
		return out, bad("hosts", "at most %d hosts", MaxHosts)
	}

	out.Hosts = hosts

	if len(in.Accepts) == 0 {
		return out, bad("accepts", "must name at least one of %s", strings.Join(Accepts, ", "))
	}

	for i, a := range in.Accepts {
		if !slices.Contains(Accepts, a) {
			return out, bad(fmt.Sprintf("accepts[%d]", i), "must be one of %s", strings.Join(Accepts, ", "))
		}
	}

	for _, a := range Accepts {
		if slices.Contains(in.Accepts, a) {
			out.Accepts = append(out.Accepts, a)
		}
	}

	return out, nil
}

// parseEndpoint reads the URL the connector calls first: https, or http for a
// loopback host, with no userinfo, query or fragment.
func parseEndpoint(raw string) (*url.URL, *FieldError) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > MaxEndpoint {
		return nil, bad("endpoint", "must be a URL of at most %d characters", MaxEndpoint)
	}

	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil, bad("endpoint", "must be a URL with a host")
	}

	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, bad("endpoint", "must have no user, query or fragment")
	}

	switch u.Scheme {
	case "https":
	case "http":
		if !loopback(u.Hostname()) {
			return nil, bad("endpoint", "http is only for a loopback host; use https")
		}
	default:
		return nil, bad("endpoint", "the scheme must be https")
	}

	host := outbound.NormalizeHost(u.Hostname())
	if !validHost(host) {
		return nil, bad("endpoint", "the host is not valid")
	}

	switch port := u.Port(); {
	case port != "":
		u.Host = net.JoinHostPort(host, port)
	case strings.Contains(host, ":"):
		u.Host = "[" + host + "]"
	default:
		u.Host = host
	}

	return u, nil
}

func loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip, err := netip.ParseAddr(host)

	return err == nil && ip.Unmap().IsLoopback()
}

// validHost accepts a lower-case DNS name or an IP literal.
func validHost(h string) bool {
	if h == "" || len(h) > 253 {
		return false
	}

	if _, err := netip.ParseAddr(h); err == nil {
		return true
	}

	for label := range strings.SplitSeq(h, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}

		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}

	return true
}

// View turns a row into the record a surface sees.
func View(r Row, secretSet bool) Connection {
	c := Connection{
		ID: r.ID, Kind: r.Kind, Label: r.Label, Endpoint: r.Endpoint,
		Hosts: slices.Clone(r.Hosts), Accepts: slices.Clone(r.Accepts),
		CreatedAt: time.UnixMilli(r.CreatedMs).UTC(), SecretSet: secretSet,
	}

	if r.LastUsedMs != nil {
		t := time.UnixMilli(*r.LastUsedMs).UTC()
		c.LastUsedAt = &t
	}

	return c
}

// Summary is the connection as one line of text for the CLI and MCP text views. The label is text
// a person typed, so it is delimited as untrusted text; the secret is never printed, only whether
// one is set.
func (c *Connection) Summary() string {
	used := "never used"
	if c.LastUsedAt != nil {
		used = "last used " + c.LastUsedAt.Format("2006-01-02 15:04")
	}

	secret := "secret set"
	if !c.SecretSet {
		secret = "NO SECRET"
	}

	return fmt.Sprintf("%s  %s  %s  hosts: %s  accepts: %s  %s  %s", c.ID, c.Kind,
		untrusted.Inline(untrusted.SourceProjectText, nil, c.Label, MaxLabel),
		strings.Join(c.Hosts, ", "), strings.Join(c.Accepts, ", "), used, secret)
}
