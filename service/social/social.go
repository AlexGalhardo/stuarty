// Package social is the frame every social network connector plugs into (ADR-0035, SDD-020): the
// Connector interface, its registry, the error a connector returns and the store of upload sessions.
// A network lives in one package of its own and nothing outside that package knows the network's API
// (NFR-063). A connector reaches the network only through the credential and the gated client of one
// connection (connections.Manager.With), so it can reach the hosts of that connection and nothing else.
package social

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"time"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
)

// RedirectPath is where the sign-in of every network lands on the local server (SDD-020 section 6).
const RedirectPath = "/social/callback"

// Description says what a network can do, where its rules were read and which hosts its connection
// reaches.
type Description struct {
	// Network is the stable name and the kind of the connections of the network, for example youtube.
	Network string `json:"network"`
	// Name is the name shown to a person.
	Name string `json:"name"`
	// Publishes lists what the connector can send: video, thumbnail.
	Publishes []string `json:"publishes"`
	// Limits are the limits of a post, with the official page and the date they were read.
	Limits publish.Limits `json:"limits"`
	// PostCostMicros and PostCostCurrency are what the network charges for one post, in millionths of the
	// currency; zero for a network that charges nothing. A scheduled post shows it in its approval and counts
	// it against the ceilings (SDD-020 section 26, RF-408).
	PostCostMicros   int64  `json:"post_cost_micros,omitempty"`
	PostCostCurrency string `json:"post_cost_currency,omitempty"`
	// Endpoint and Hosts are what a connection of the network is created with.
	Endpoint string   `json:"endpoint"`
	Hosts    []string `json:"hosts"`
	// Scopes are the permissions the sign-in asks for.
	Scopes []string `json:"scopes"`
	// Guide is the official page the setup guide links to, and GuideRead the date it was read.
	Guide     string `json:"guide"`
	GuideRead string `json:"guide_read"`
	// Hints say what a person does about an error of this network, by the kind of the error (KindQuota...).
	// Each connector writes its own, so an error of one network never sends the person to another's screen;
	// a kind with no hint gets the generic one of the service.
	Hints map[string]string `json:"hints,omitempty"`
	// AIDisclosure is true when the network takes a flag that says the video has synthetic content.
	AIDisclosure bool `json:"ai_disclosure"`
}

// AuthConfig is what the generic sign-in needs from a network (SDD-020 section 6).
type AuthConfig struct {
	AuthURL  string
	TokenURL string
	Scopes   []string
	// Extra are fixed parameters of the authorization address, for example access_type=offline.
	Extra url.Values
}

// Account is who a credential is on the network.
type Account struct {
	ID    string
	Title string
}

// Remote is what the network says of a video it holds.
type Remote struct {
	State publish.State
	// Reason is a stable code of the network (for example duplicate) when the state is failed.
	Reason string
}

// Published is the result of an upload.
type Published struct {
	RemoteID string
	URL      string
	State    publish.State
	// ThumbnailError is the kind of the error of the thumbnail when the video was published and the
	// thumbnail was not; empty otherwise.
	ThumbnailError string
}

// Upload is what Publish is given: the post, the files and where to keep the upload session.
type Upload struct {
	Post          publish.Post
	Path          string
	ThumbnailPath string
	// Key identifies this upload for resuming; Sessions keeps the address of its session.
	Key      string
	Sessions Sessions
	// Report tells the progress, 0 to 1, with a short message.
	Report func(progress float64, message string)
}

// Connector is one network.
type Connector interface {
	// Describe says what the network can do.
	Describe() Description
	// Validate checks a post with the limits of the network and the rules only that network has.
	Validate(p publish.Post) []publish.Issue
	// Publish sends the post. It reaches the network only through c.Client.
	Publish(ctx context.Context, c connections.Credential, u Upload) (Published, error)
	// Status reads the state of a video that was published.
	Status(ctx context.Context, c connections.Credential, remoteID string) (Remote, error)
	// AuthConfig is the sign-in of the network.
	AuthConfig() AuthConfig
	// Account says who the credential is.
	Account(ctx context.Context, c connections.Credential) (Account, error)
	// SignedIn says whether the secret of a connection holds a refresh token and who it names;
	// it never returns the token.
	SignedIn(secret []byte) (Account, bool)
	// ClientID is the client id kept in the secret of a connection, for the authorization address; false
	// when the secret holds none.
	ClientID(secret []byte) (string, bool)
	// Begin makes the secret of a new connection from the client id and secret of the person's own app.
	Begin(clientID, clientSecret string) ([]byte, error)
	// Exchange trades an authorization code for the tokens and returns the new secret of the connection.
	// It reaches the network only through c.Client.
	Exchange(ctx context.Context, c connections.Credential, code, verifier, redirectURI string) ([]byte, error)
	// Test is the harmless request of the Connections page.
	connections.Connector
}

// Chooser is implemented by a connector whose sign-in reaches several accounts, such as the Pages and the
// Instagram accounts of one Meta login (SDD-020 section 17). The sign-in keeps what it reached and a person
// chooses which account the connection is for; until then SignedIn says false. A connector that signs in as
// one account (YouTube) does not implement it.
type Chooser interface {
	// Choices lists the accounts the sign-in reached and none is chosen of; empty once an account is chosen.
	// It never returns a token.
	Choices(secret []byte) []Account
	// Choose returns the new secret of the connection with the one account chosen, and only its credential
	// kept. An id that is not among the choices is an error.
	Choose(secret []byte, accountID string) ([]byte, error)
}

// The kinds of a connector error.
const (
	KindQuota       = "quota"
	KindNotEligible = "not_eligible"
	KindAuth        = "auth"
	KindInvalid     = "invalid"
	KindRejected    = "rejected"
	KindUnavailable = "unavailable"
)

var reasonPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,60}$`)

// Error is a failure of the network that a person can act on. The text of the network is never in it:
// Reason is a stable code that passed reasonPattern, and the message is written by the service.
type Error struct {
	Kind    string
	Network string
	// Reason is the code the network gave (quotaExceeded, duplicate...), or empty.
	Reason string
	// RetryAfter is when a quota comes back, when known.
	RetryAfter time.Time
	// Status is the HTTP status the network answered, or 0.
	Status int
	Err    error
}

// NewError builds a connector error; a reason that is not a plain code is dropped, so the text of the
// network cannot travel in it.
func NewError(kind, network, reason string, status int, cause error) *Error {
	if !reasonPattern.MatchString(reason) {
		reason = ""
	}

	return &Error{Kind: kind, Network: network, Reason: reason, Status: status, Err: cause}
}

func (e *Error) Error() string {
	s := "social: " + e.Network + ": " + e.Kind

	if e.Reason != "" {
		s += " (" + e.Reason + ")"
	}

	return s
}

func (e *Error) Unwrap() error { return e.Err }

// Retryable says whether the same publication may succeed later.
func (e *Error) Retryable() bool {
	return e.Kind == KindQuota || e.Kind == KindUnavailable || e.Kind == KindAuth
}

// AsError finds a connector error in err.
func AsError(err error) (*Error, bool) { return errors.AsType[*Error](err) }
