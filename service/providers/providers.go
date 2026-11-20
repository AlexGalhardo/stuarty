// Package providers is the AI provider layer (SDD-016, ADR-0036): the interfaces an adapter
// implements, the registry that finds the adapter of a connection, the catalogue of models read from
// the provider at run time, and the price table the person edits. An adapter lives in its own package
// under service/providers/<name>/ and is added to the owner with one line; nothing here lists the
// providers.
//
// Every request goes through a connection (service/connections): an adapter receives a
// connections.Credential for the length of one call, whose client reaches only the hosts of that
// connection, and never opens a socket of its own.
package providers

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// Limits of SDD-016 section 3.
const (
	// MaxModels is how many models of one catalogue are kept.
	MaxModels = 2000
	// MaxModelName is the longest display name of a model, in characters.
	MaxModelName = 80
)

// Descriptor says what a provider is and how a connection to it starts (SDD-016 section 2).
type Descriptor struct {
	// ID is the connection kind of the provider; see provider.ValidProviderID.
	ID   string `json:"id"`
	Name string `json:"name"`
	// Capabilities are the provider.Capability* values the adapter implements.
	Capabilities []string `json:"capabilities"`
	// CredentialKinds are the ways to connect; epic 09 has provider.CredentialAPIKey only.
	CredentialKinds []string `json:"credential_kinds"`
	// DefaultEndpoint is the base URL a new connection starts from; its host is the default host.
	DefaultEndpoint string `json:"default_endpoint"`
	// DocsURL is where the person finds the key. It is shown and never called.
	DocsURL string `json:"docs_url"`
}

// Model is one entry of the catalogue of a provider.
type Model struct {
	// ID matches provider.ValidModel.
	ID string `json:"id"`
	// Name is a display name written by the provider: untrusted text, cleaned and cut.
	Name          string `json:"name"           untrusted:"true"`
	ContextTokens int    `json:"context_tokens"`
	OutputTokens  int    `json:"output_tokens"`
	// Price is set only when the catalogue publishes one.
	Price *provider.Price `json:"price,omitempty"`
	// Capability is a hint from the adapter (provider.Capability*), or empty: it orders what a picker shows
	// and is never a rule (SDD-016 section 2.2).
	Capability string `json:"capability,omitempty"`
}

// Provider is the part every adapter implements.
type Provider interface {
	Descriptor() Descriptor
	// Models reads the catalogue with the credential of one connection. It returns what the provider
	// says; the registry drops the ids that are not valid and caps the list.
	Models(ctx context.Context, c connections.Credential) ([]Model, error)
}

// TextResult is the answer to one text request.
type TextResult struct {
	// Text is what the model wrote: untrusted text.
	Text string
	// FinishReason is one of FinishStop, FinishLength or FinishOther.
	FinishReason string
	Usage        provider.Usage
	// Model is the model that answered, as the provider names it.
	Model string
}

// The finish reasons an adapter maps its own to.
const (
	FinishStop   = "stop"
	FinishLength = "length"
	FinishOther  = "other"
)

// TextGenerator is a provider that writes text.
type TextGenerator interface {
	Provider
	// Framing is the number of input tokens the provider adds for the framing of its messages, so the
	// estimate stays an upper bound. It needs no network.
	Framing() int
	// GenerateText makes one request and returns the text and the usage. It does not stream.
	GenerateText(ctx context.Context, c connections.Credential, req provider.TextRequest) (TextResult, error)
}

// GeneratedFile is one file a provider made: the bytes, already read (a result delivered as a URL is
// fetched through the gated client by the adapter).
type GeneratedFile struct {
	Data []byte
	// RevisedPrompt is the prompt as the provider rewrote it, when it does: untrusted text.
	RevisedPrompt string `untrusted:"true"`
}

// ImageResult is the answer to one request for images.
type ImageResult struct {
	// Model is the model that answered, as the provider names it; empty when it does not say.
	Model string
	Files []GeneratedFile
	// Failed is how many of the images asked for did not arrive, and Err is why the first did not: a partial
	// batch keeps what arrived (SDD-016 section 6.2).
	Failed int
	Err    error
}

// ImageGenerator is a provider that makes images (SDD-016 section 2.2).
type ImageGenerator interface {
	Provider
	// ImageLimits are the sizes and the count the provider documents for a model. It needs no network.
	ImageLimits(model string) provider.ImageLimits
	// GenerateImages makes one request and returns the files. No file is larger than maxBytes.
	GenerateImages(
		ctx context.Context, c connections.Credential, req provider.ImageRequest, maxBytes int64,
	) (ImageResult, error)
}

// Voice is one voice of a provider.
type Voice struct {
	ID string `json:"id"`
	// Name is a display name: untrusted text, cleaned and cut like the name of a model.
	Name     string `json:"name"               untrusted:"true"`
	Language string `json:"language,omitempty"`
	// Source is SourceProvider for a list the provider answered, SourceDocumented for one its documentation
	// gives because it has no endpoint for it.
	Source string `json:"source"`
}

// Where a list of voices came from.
const (
	SourceProvider   = "provider"
	SourceDocumented = "documented"
)

// VoiceResult is the answer to one request for speech.
type VoiceResult struct {
	Model string
	File  GeneratedFile
}

// VoiceGenerator is a provider that reads a text out (SDD-016 section 2.2).
type VoiceGenerator interface {
	Provider
	// VoiceLimits are what the provider documents for a model. It needs no network.
	VoiceLimits(model string) provider.VoiceLimits
	// Voices lists the voices of the provider.
	Voices(ctx context.Context, c connections.Credential) ([]Voice, error)
	// GenerateVoice makes one request and returns the audio. The file is no larger than maxBytes.
	GenerateVoice(
		ctx context.Context, c connections.Credential, req provider.VoiceRequest, maxBytes int64,
	) (VoiceResult, error)
}

// VideoFrame is the first frame of a clip: the bytes of an image asset, already checked.
type VideoFrame struct {
	Data   []byte
	Format string // "png", "jpeg" or "webp"
}

// The states of a video task of a provider (SDD-016 section 6.4).
const (
	VideoPending = "pending"
	VideoDone    = "done"
	VideoFailed  = "failed"
)

// VideoStatus is what a provider says of a task.
type VideoStatus struct {
	// State is VideoPending, VideoDone or VideoFailed.
	State string
	// Progress is 0 to 1 when the provider says one, else 0.
	Progress float64
	// PollAfter is how long the provider asks to wait before the next poll; zero leaves it to the caller's backoff.
	PollAfter time.Duration
	// Err explains a failed task: a *Error, such as a refusal of the prompt.
	Err error
	// Charged says, of a failed task, that the provider may have charged for it: its documentation does not say
	// that a task that ends with no clip is free. The job then records the estimate (SDD-016 section 6.4).
	Charged bool
}

// VideoGenerator is a provider that makes clips (SDD-016 section 2). Generation is asynchronous at every provider:
// Submit starts it, Poll reads it, Fetch downloads the clip.
type VideoGenerator interface {
	Provider
	// VideoLimits are what the provider documents for a model. It needs no network.
	VideoLimits(model string) provider.VideoLimits
	// Submit starts one generation and returns the task id of the provider (provider.ValidTaskID). It is the
	// only call that spends; the caller stores the id before it polls. frame is the first frame, or nil.
	Submit(
		ctx context.Context, c connections.Credential, req provider.VideoRequest, frame *VideoFrame,
	) (taskID string, err error)
	// Poll reads the state of a task. It costs nothing and may be repeated.
	Poll(ctx context.Context, c connections.Credential, taskID string) (VideoStatus, error)
	// Fetch downloads the clip of a done task through the gated client. The file is no larger than maxBytes.
	Fetch(ctx context.Context, c connections.Credential, taskID string, maxBytes int64) (GeneratedFile, error)
	// Cancel asks the provider to stop a task and says whether it confirmed that nothing will be charged.
	Cancel(ctx context.Context, c connections.Credential, taskID string) (confirmed bool, err error)
}

// The kinds of a provider failure (SDD-016 section 8).
const (
	KindAuth        = "auth"
	KindRateLimited = "rate_limited"
	KindBadRequest  = "bad_request"
	KindUnavailable = "unavailable"
	KindUnreadable  = "unreadable"
	KindRefused     = "refused"
	KindTimeout     = "timeout"
)

// Error is a failure of a provider. It is built from the status and a short code, never from the body
// of the answer, so it can be shown and logged (SDD-016 section 9).
type Error struct {
	Kind   string
	Status int
	// Code is the short error code of the provider when it matches a safe pattern; else empty.
	Code string
	// RetryAfter is the wait the provider asked for, in seconds; zero when it gave none.
	RetryAfter int
}

func (e *Error) Error() string {
	s := "provider: " + e.Kind

	if e.Status != 0 {
		s += fmt.Sprintf(" (status %d)", e.Status)
	}

	return s
}

// AsError returns the *Error in err's chain.
func AsError(err error) (*Error, bool) { return errors.AsType[*Error](err) }

// CodeNeedsSignIn is the code of an auth failure of a connection made by a sign-in (SDD-016 section 12.4):
// the person signs in again from the Connections page; a new key would not help.
const CodeNeedsSignIn = "needs_sign_in"

// SignInAware says, of an auth failure through a connection made by a sign-in (the provider refused a token
// or a key that was revoked before it expired), that the person must sign in again, instead of asking for a
// key. Any other error is returned as it is.
func SignInAware(err error, c connections.Credential) error {
	pe, ok := AsError(err)
	if !ok || !c.SignedIn || pe.Kind != KindAuth {
		return err
	}

	return &Error{Kind: KindAuth, Status: pe.Status, Code: CodeNeedsSignIn}
}

var codeRe = regexp.MustCompile(`^[a-z0-9_.-]{1,64}$`)

// SafeCode returns s when it is a short error code of the safe pattern, else empty: a code the provider
// writes is shown only when it cannot carry text.
func SafeCode(s string) string {
	if codeRe.MatchString(s) {
		return s
	}

	return ""
}

// CleanName makes a display name of the text a provider wrote: cleaned of invisible characters,
// reduced to one line and cut to MaxModelName characters.
func CleanName(s string) string {
	s = untrusted.Clean(s)
	s = strings.Join(strings.Fields(s), " ")

	if utf8.RuneCountInString(s) > MaxModelName {
		s = string([]rune(s)[:MaxModelName])
	}

	return s
}

// finish drops the models whose id is not valid, removes duplicates, sorts by id and caps the list.
func finish(in []Model) []Model {
	out := make([]Model, 0, len(in))
	seen := make(map[string]bool, len(in))

	for i := range in {
		m := in[i]
		if !provider.ValidModel(m.ID) || seen[m.ID] {
			continue
		}

		seen[m.ID] = true
		m.Name = CleanName(m.Name)
		m.ContextTokens, m.OutputTokens = max(m.ContextTokens, 0), max(m.OutputTokens, 0)

		if m.Price != nil {
			if p, ferr := provider.NormalizePrice(*m.Price); ferr == nil {
				m.Price = &p
			} else {
				m.Price = nil
			}
		}

		out = append(out, m)
	}

	slices.SortFunc(out, func(a, b Model) int { return strings.Compare(a.ID, b.ID) })

	if len(out) > MaxModels {
		out = out[:MaxModels]
	}

	return out
}
