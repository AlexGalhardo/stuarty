package webhook

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/AlexGalhardo/stuarty/core/publish"
)

// Data is the subject of an event: only the members of its struct reach the body, and each struct holds
// identifiers, states, counts and the public address of a post (SDD-022 section 3). A text the product holds (a
// title, a caption, a summary, a prompt, an error message) is a member of none of them, and a new member needs a
// change of the table Allowed in the same commit.
type Data interface {
	// EventName is the event of the subject.
	EventName() string
	// key is what makes the id of the event: the same subject is the same event.
	key() string
}

// PublicationPublishedData is the data of publication.published.
type PublicationPublishedData struct {
	PublicationID string `json:"publication_id"`
	TargetID      string `json:"target_id"`
	ProjectID     string `json:"project_id"`
	Network       string `json:"network"`
	ConnectionID  string `json:"connection_id"`
	// URL is the address of the post, only when it is public (PublicPostURL).
	URL         string `json:"url,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
}

// EventName implements Data.
func (PublicationPublishedData) EventName() string { return PublicationPublished }
func (d PublicationPublishedData) key() string     { return d.TargetID }

// PublicationFailedData is the data of publication.failed.
type PublicationFailedData struct {
	PublicationID string `json:"publication_id"`
	TargetID      string `json:"target_id"`
	ProjectID     string `json:"project_id"`
	Network       string `json:"network"`
	ConnectionID  string `json:"connection_id"`
	ErrorKind     string `json:"error_kind,omitempty"`
	// ApprovalID is the request of the send that failed; it makes the id of the event and is not in the body. A target
	// that is retried gets a new request, so its next failure is a new event and not a duplicate of the first.
	ApprovalID string `json:"-"`
}

// EventName implements Data.
func (PublicationFailedData) EventName() string { return PublicationFailed }
func (d PublicationFailedData) key() string     { return d.TargetID + "\x00" + d.ApprovalID }

// ExportReadyData is the data of export.ready.
type ExportReadyData struct {
	JobID string `json:"job_id"`
	// ProjectID is empty for the export of an image, which belongs to no project.
	ProjectID string `json:"project_id,omitempty"`
	Kind      string `json:"kind"`
	Format    string `json:"format,omitempty"`
	Bytes     int64  `json:"bytes"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

// EventName implements Data.
func (ExportReadyData) EventName() string { return ExportReady }
func (d ExportReadyData) key() string     { return d.JobID }

// ApprovalRequestedData is the data of approval.requested.
type ApprovalRequestedData struct {
	ApprovalID   string `json:"approval_id"`
	Kind         string `json:"kind"`
	ProjectID    string `json:"project_id,omitempty"`
	JobID        string `json:"job_id,omitempty"`
	ConnectionID string `json:"connection_id,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
}

// EventName implements Data.
func (ApprovalRequestedData) EventName() string { return ApprovalRequested }
func (d ApprovalRequestedData) key() string     { return d.ApprovalID }

// JobFailedData is the data of job.failed.
type JobFailedData struct {
	JobID     string `json:"job_id"`
	ProjectID string `json:"project_id,omitempty"`
	Kind      string `json:"kind"`
	State     string `json:"state"`
	ErrorCode string `json:"error_code,omitempty"`
}

// EventName implements Data.
func (JobFailedData) EventName() string { return JobFailed }
func (d JobFailedData) key() string     { return d.JobID }

// TestData is the data of webhook.test. Nonce makes the id of each test its own and is not in the body.
type TestData struct {
	WebhookID string `json:"webhook_id"`
	Nonce     string `json:"-"`
}

// EventName implements Data.
func (TestData) EventName() string { return Test }
func (d TestData) key() string     { return d.WebhookID + "\x00" + d.Nonce }

// Allowed is the allow-list of the members of data, by event (SDD-022 section 3). Check refuses any other member.
func Allowed(event string) []string {
	switch event {
	case PublicationPublished:
		return []string{
			"publication_id", "target_id", "project_id", "network", "connection_id", "url", "published_at",
		}
	case PublicationFailed:
		return []string{"publication_id", "target_id", "project_id", "network", "connection_id", "error_kind"}
	case ExportReady:
		return []string{"job_id", "project_id", "kind", "format", "bytes", "width", "height"}
	case ApprovalRequested:
		return []string{"approval_id", "kind", "project_id", "job_id", "connection_id", "expires_at"}
	case JobFailed:
		return []string{"job_id", "project_id", "kind", "state", "error_code"}
	case Test:
		return []string{"webhook_id"}
	}

	return nil
}

// Envelope is the body of a delivery (SDD-022 section 3).
type Envelope struct {
	Event      string          `json:"event"`
	ID         string          `json:"id"`
	OccurredAt string          `json:"occurred_at"`
	Workspace  string          `json:"workspace"`
	Data       json.RawMessage `json:"data"`
}

// EventID is the id of an event: evt_ and 20 hexadecimal characters of the SHA-256 of the event name and the key of
// its subject.
func EventID(d Data) string {
	sum := sha256.Sum256([]byte(d.EventName() + "\x00" + d.key()))

	return "evt_" + hex.EncodeToString(sum[:10])
}

// WorkspaceID is the opaque, stable id of a workspace in a body: wsp_ and 10 hexadecimal characters of the SHA-256 of
// its path, which the body never carries.
func WorkspaceID(path string) string {
	sum := sha256.Sum256([]byte("stuarty-workspace\x00" + path))

	return "wsp_" + hex.EncodeToString(sum[:5])
}

// Build makes the body of an event and its id. It checks the body before it returns it.
func Build(d Data, at time.Time, workspace string) (body []byte, id string, err error) {
	data, err := json.Marshal(d)
	if err != nil {
		return nil, "", err
	}

	id = EventID(d)

	body, err = json.Marshal(Envelope{
		Event: d.EventName(), ID: id, OccurredAt: at.UTC().Format(time.RFC3339), Workspace: workspace, Data: data,
	})
	if err != nil {
		return nil, "", err
	}

	if err := Check(body); err != nil {
		return nil, "", err
	}

	return body, id, nil
}

// PublicPostURL returns the address of a post when it is one a receiver may be given: the post is public, and the
// address is https, with a host, no user name and at most 2048 characters. Anything else is the empty string, so no
// member is sent. The link of an unlisted post is what keeps it private, so it is never handed to a third party, and
// a post that names no visibility is private (core/publish).
func PublicPostURL(raw string, visibility publish.Visibility) string {
	if visibility != publish.Public {
		return ""
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || len(raw) > MaxPostURLBytes ||
		strings.ContainsFunc(raw, unicode.IsControl) {
		return ""
	}

	if IsLoopbackHost(u.Hostname()) {
		return ""
	}

	return raw
}

// ErrBody is a body that breaks the rule of SDD-022 section 3.
var ErrBody = errors.New("webhook: the body breaks the payload rule")

// forbidden are the beginnings of a value that is a credential or a key: a body must never carry one, whatever
// member holds it. A private key block is looked for anywhere in a value.
var forbidden = []string{"whsec_", "bearer ", "sk-", "basic ", "ghp_", "xoxb-", "eyj"}

// Check verifies a body: the five members of the envelope and nothing else, a known event, the members of data on
// the allow-list of its event, values that are strings or numbers, no value that looks like a credential, no control
// character, and at most 8 KiB.
func Check(body []byte) error {
	if len(body) == 0 || len(body) > MaxBodyBytes {
		return fmt.Errorf("%w: %d bytes", ErrBody, len(body))
	}

	top, err := members(body)
	if err != nil {
		return err
	}

	if len(top) != 5 {
		return fmt.Errorf("%w: the envelope has %d members", ErrBody, len(top))
	}

	for _, k := range []string{"event", "id", "occurred_at", "workspace", "data"} {
		if _, ok := top[k]; !ok {
			return fmt.Errorf("%w: no member %s", ErrBody, k)
		}
	}

	var event string
	if err := json.Unmarshal(top["event"], &event); err != nil {
		return fmt.Errorf("%w: event", ErrBody)
	}

	allowed := Allowed(event)
	if allowed == nil {
		return fmt.Errorf("%w: the event %q", ErrBody, clip(event))
	}

	data, err := members(top["data"])
	if err != nil {
		return err
	}

	for k, v := range data {
		if !slices.Contains(allowed, k) {
			return fmt.Errorf("%w: the member %s is not on the list of %s", ErrBody, clip(k), event)
		}

		if err := checkValue(v); err != nil {
			return err
		}
	}

	for k, v := range top {
		if k != "data" {
			if err := checkValue(v); err != nil {
				return err
			}
		}
	}

	return nil
}

func members(raw []byte) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var m map[string]json.RawMessage
	if err := dec.Decode(&m); err != nil || m == nil {
		return nil, fmt.Errorf("%w: not an object", ErrBody)
	}

	return m, nil
}

func checkValue(raw json.RawMessage) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return fmt.Errorf("%w: an empty value", ErrBody)
	}

	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return fmt.Errorf("%w: a string", ErrBody)
		}

		if len(s) > MaxPostURLBytes || strings.ContainsFunc(s, unicode.IsControl) {
			return fmt.Errorf("%w: a string that is long or has a control character", ErrBody)
		}

		low := strings.ToLower(s)
		if strings.Contains(low, "-----begin") || slices.ContainsFunc(forbidden, func(f string) bool {
			return strings.HasPrefix(low, f)
		}) {
			return fmt.Errorf("%w: a value that looks like a credential", ErrBody)
		}
	case '{', '[':
		return fmt.Errorf("%w: a nested value", ErrBody)
	}

	return nil
}
