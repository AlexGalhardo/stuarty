// Package webhook holds the pure rules of the outbound webhooks (SDD-022, ADR-0032, ADR-0033): the record and its
// limits, the events and the sentence that explains each, the address rules, the payload of each event built from an
// allow-list, the signature, and the schedule of the retries. It does no I/O and opens no connection: the service
// stores the webhooks and sends the deliveries, and no surface computes a rule of its own.
package webhook

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The limits of SDD-022 section 8.
const (
	MaxWebhooks     = 20
	MaxEvents       = 5
	MaxNameRunes    = 60
	MaxURLBytes     = 2048
	MaxBodyBytes    = 8 << 10
	MaxPostURLBytes = 2048
)

// The events (SDD-022 section 3).
const (
	PublicationPublished = "publication.published"
	PublicationFailed    = "publication.failed"
	ExportReady          = "export.ready"
	ApprovalRequested    = "approval.requested"
	JobFailed            = "job.failed"
	// Test is the event of the "send test" action. A webhook cannot choose it.
	Test = "webhook.test"
)

// EventInfo is an event a webhook may choose, with the sentence that tells a person when it fires and what it carries.
type EventInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Events lists the events a webhook may choose, in the order a page shows them.
func Events() []EventInfo {
	return []EventInfo{
		{PublicationPublished, "A post went out to a network or a blog. It carries the publication, the network, the " +
			"account connection and, when the post is public, its address."},
		{PublicationFailed, "A post failed for good and will not be tried again by itself. It carries the publication, " +
			"the network and the kind of failure, never the text of the error."},
		{ExportReady, "An export finished. It carries the job, the project, the format, the size and the picture size, " +
			"never the file or its path."},
		{ApprovalRequested, "Something waits for a person to approve it. It carries the request, its kind and when it " +
			"expires, never what it says."},
		{JobFailed, "A job failed. It carries the job, the project, the kind and the error code, never the message."},
	}
}

// ValidEvent reports whether name is an event a webhook may choose.
func ValidEvent(name string) bool {
	return slices.ContainsFunc(Events(), func(e EventInfo) bool { return e.Name == name })
}

// FieldError is a refusal of one member of a request: the path of the member and a sentence for a person.
type FieldError struct {
	Path    string
	Message string
}

func (e *FieldError) Error() string { return e.Path + ": " + e.Message }

func bad(path, format string, args ...any) *FieldError {
	return &FieldError{Path: path, Message: fmt.Sprintf(format, args...)}
}

// NormalizeName trims the name and checks it: 1 to 60 characters, no control character.
func NormalizeName(name string) (string, *FieldError) {
	name = strings.TrimSpace(name)

	switch n := utf8.RuneCountInString(name); {
	case n == 0 || n > MaxNameRunes:
		return "", bad("name", "the name is 1 to %d characters.", MaxNameRunes)
	case strings.ContainsFunc(name, unicode.IsControl):
		return "", bad("name", "the name has a control character.")
	}

	return name, nil
}

// NormalizeEvents checks the list of events and returns it in the order of Events, without duplicates.
func NormalizeEvents(events []string) ([]string, *FieldError) {
	if len(events) == 0 {
		return nil, bad("events", "choose at least one event.")
	}

	for _, e := range events {
		if !ValidEvent(e) {
			return nil, bad("events", "%q is not an event of a webhook.", clip(e))
		}
	}

	out := make([]string, 0, len(events))

	for _, e := range Events() {
		if slices.Contains(events, e.Name) {
			out = append(out, e.Name)
		}
	}

	if len(out) != len(events) {
		return nil, bad("events", "an event is listed twice.")
	}

	if len(out) > MaxEvents {
		return nil, bad("events", "a webhook has at most %d events.", MaxEvents)
	}

	return out, nil
}

// Subscribed reports whether a webhook with these events and this state receives an event.
func Subscribed(events []string, enabled bool, event string) bool {
	return enabled && slices.Contains(events, event)
}

func clip(s string) string {
	r := []rune(s)
	if len(r) > 40 {
		r = append(r[:40], '.', '.', '.')
	}

	return strings.Map(func(c rune) rune {
		if unicode.IsControl(c) {
			return '?'
		}

		return c
	}, string(r))
}
