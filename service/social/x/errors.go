package x

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/social"
)

const maxBody = 1 << 20

// The reasons of an error of X: stable codes, never the text of the answer (SDD-020 section 36, T-58).
const (
	ReasonTokenInvalid     = "token_invalid"
	ReasonForbidden        = "forbidden"
	ReasonDuplicate        = "duplicate"
	ReasonDuration         = "duration"
	ReasonCredits          = "credits_exhausted"
	ReasonRateLimit        = "rate_limit"
	ReasonInvalidRequest   = "invalid_request"
	ReasonMediaExpired     = "media_expired"
	ReasonProcessingFailed = "processing_failed"
	ReasonStillProcessing  = "still_processing"
	ReasonOutcomeUnknown   = "outcome_unknown"
)

// gateError says whether err is the refusal of the outbound gate, which the service maps to its own code and
// which a connector never rewrites.
func gateError(err error) bool {
	return errors.Is(err, outbound.ErrNoConnection) || errors.Is(err, outbound.ErrDestination) ||
		errors.Is(err, outbound.ErrNotPublic)
}

// transportError turns a failure to reach X into a connector error; the refusal of the gate passes through.
func transportError(err error) error {
	if gateError(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	return social.NewError(social.KindUnavailable, publish.NetworkX, "", 0, err)
}

// tokenError turns a refusal of the token endpoint into a connector error.
func tokenError(err error) error {
	te, ok := errors.AsType[*oauth.TokenError](err)
	if !ok {
		return transportError(err)
	}

	switch {
	case te.Status >= http.StatusInternalServerError:
		return social.NewError(social.KindUnavailable, publish.NetworkX, "", te.Status, err)
	case te.Status == http.StatusTooManyRequests:
		return social.NewError(social.KindQuota, publish.NetworkX, ReasonRateLimit, te.Status, err)
	default:
		reason := te.Code
		if reason == "" {
			reason = "refused"
		}

		return social.NewError(social.KindAuth, publish.NetworkX, reason, te.Status, err)
	}
}

// problem is the answer of X to a request that failed: an Error (code, message) or a Problem (type, title,
// detail, status). The text is read to choose a row of the table of section 36 and is never copied.
type problem struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Type   string `json:"type"`
	Code   int    `json:"code"`
	Errors []struct {
		Code int `json:"code"`
	} `json:"errors"`
}

func (p *problem) text() string { return strings.ToLower(p.Title + " " + p.Detail) }

func parseProblem(body []byte) problem {
	var p problem

	_ = json.Unmarshal(body, &p)

	return p
}

// resetAt is when a rate limit comes back: x-rate-limit-reset is a Unix time.
func resetAt(h http.Header, now time.Time) time.Time {
	if n, err := strconv.ParseInt(h.Get("x-rate-limit-reset"), 10, 64); err == nil && n > now.Unix() {
		return time.Unix(n, 0).UTC()
	}

	return now.Add(15 * time.Minute).UTC()
}

// classify turns an error answer of the X API into a connector error (SDD-020 section 36).
func classify(status int, body []byte, h http.Header, now time.Time) *social.Error {
	p := parseProblem(body)
	txt := p.text()

	mk := func(kind, reason string) *social.Error {
		return social.NewError(kind, publish.NetworkX, reason, status, nil)
	}

	switch {
	case status == http.StatusTooManyRequests:
		e := mk(social.KindQuota, ReasonRateLimit)
		e.RetryAfter = resetAt(h, now)

		return e
	case status == http.StatusPaymentRequired:
		return mk(social.KindQuota, ReasonCredits)
	case status == http.StatusUnauthorized:
		return mk(social.KindAuth, ReasonTokenInvalid)
	case status == http.StatusForbidden && strings.Contains(txt, "duplicate"):
		return mk(social.KindRejected, ReasonDuplicate)
	case status == http.StatusForbidden && strings.Contains(txt, "video") && strings.Contains(txt, "longer than"):
		return mk(social.KindInvalid, ReasonDuration)
	case status == http.StatusForbidden:
		return mk(social.KindNotEligible, ReasonForbidden)
	case status == http.StatusNotFound:
		return mk(social.KindInvalid, ReasonMediaExpired)
	case status >= http.StatusInternalServerError:
		return mk(social.KindUnavailable, "")
	default:
		return mk(social.KindInvalid, ReasonInvalidRequest)
	}
}
