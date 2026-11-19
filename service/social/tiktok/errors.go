package tiktok

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/social"
)

const maxBody = 1 << 20

// The reasons of an error of TikTok: stable codes, never the text of the answer (SDD-020 section 37, T-58).
const (
	ReasonTokenInvalid      = "token_invalid"
	ReasonScopeMissing      = "scope_missing"
	ReasonAuthRemoved       = "auth_removed"
	ReasonRateLimit         = "rate_limit"
	ReasonPendingShareLimit = "pending_share_limit"
	ReasonPostingLimit      = "posting_limit"
	ReasonBanned            = "banned"
	ReasonSpamRisk          = "spam_risk"
	ReasonInvalidParam      = "invalid_param"
	ReasonInvalidPublishID  = "invalid_publish_id"
	ReasonURLOwnership      = "url_ownership"
	ReasonUnsupportedFormat = "unsupported_format"
	ReasonDuration          = "duration"
	ReasonFrameRate         = "frame_rate"
	ReasonResolution        = "resolution"
	ReasonPublishCancelled  = "publish_cancelled"
	ReasonStillProcessing   = "still_processing"
	ReasonUploadExpired     = "upload_expired"
	ReasonUploadIncomplete  = "upload_incomplete"
)

// gateError says whether err is the refusal of the outbound gate, which the service maps to its own code and
// which a connector never rewrites.
func gateError(err error) bool {
	return errors.Is(err, outbound.ErrNoConnection) || errors.Is(err, outbound.ErrDestination) ||
		errors.Is(err, outbound.ErrNotPublic)
}

// transportError turns a failure to reach TikTok into a connector error; the refusal of the gate passes
// through.
func transportError(err error) error {
	if gateError(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	return social.NewError(social.KindUnavailable, publish.NetworkTikTok, "", 0, err)
}

// tokenError turns a refusal of the token endpoint into a connector error.
func tokenError(err error) error {
	te, ok := errors.AsType[*oauth.TokenError](err)
	if !ok {
		return transportError(err)
	}

	switch {
	case te.Status >= http.StatusInternalServerError:
		return social.NewError(social.KindUnavailable, publish.NetworkTikTok, "", te.Status, err)
	case te.Status == http.StatusTooManyRequests:
		return social.NewError(social.KindQuota, publish.NetworkTikTok, ReasonRateLimit, te.Status, err)
	default:
		reason := te.Code
		if reason == "" {
			reason = "refused"
		}

		return social.NewError(social.KindAuth, publish.NetworkTikTok, reason, te.Status, err)
	}
}

// codeOf reads the stable code of a TikTok answer: {"error": {"code": "..."}}. The message is not read.
func codeOf(body []byte) string {
	var a struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}

	if json.Unmarshal(body, &a) != nil {
		return ""
	}

	return a.Error.Code
}

// classify turns an error answer of the TikTok API into a connector error (SDD-020 section 37).
func classify(status int, body []byte) *social.Error {
	code := codeOf(body)

	mk := func(kind, reason string) *social.Error {
		return social.NewError(kind, publish.NetworkTikTok, reason, status, nil)
	}

	switch {
	case code == "spam_risk_too_many_pending_share":
		return mk(social.KindQuota, ReasonPendingShareLimit)
	case code == "rate_limit_exceeded" || status == http.StatusTooManyRequests:
		return mk(social.KindQuota, ReasonRateLimit)
	case code == "spam_risk_user_banned_from_posting":
		return mk(social.KindNotEligible, ReasonBanned)
	case code == "access_token_invalid" || (status == http.StatusUnauthorized && code != "scope_not_authorized"):
		return mk(social.KindAuth, ReasonTokenInvalid)
	case code == "scope_not_authorized":
		return mk(social.KindAuth, ReasonScopeMissing)
	case code == "url_ownership_unverified":
		return mk(social.KindInvalid, ReasonURLOwnership)
	case code == "invalid_publish_id":
		return mk(social.KindInvalid, ReasonInvalidPublishID)
	case status >= http.StatusInternalServerError:
		return mk(social.KindUnavailable, "")
	default:
		return mk(social.KindInvalid, ReasonInvalidParam)
	}
}

// failReason maps the fail_reason of a status that says FAILED (SDD-020 section 37).
func failReason(reason string) *social.Error {
	mk := func(kind, why string) *social.Error {
		return social.NewError(kind, publish.NetworkTikTok, why, http.StatusOK, nil)
	}

	switch reason {
	case "file_format_check_failed":
		return mk(social.KindInvalid, ReasonUnsupportedFormat)
	case "duration_check_failed":
		return mk(social.KindInvalid, ReasonDuration)
	case "frame_rate_check_failed":
		return mk(social.KindInvalid, ReasonFrameRate)
	case "picture_size_check_failed":
		return mk(social.KindInvalid, ReasonResolution)
	case "auth_removed":
		return mk(social.KindAuth, ReasonAuthRemoved)
	case "spam_risk_too_many_posts":
		return mk(social.KindQuota, ReasonPostingLimit)
	case "spam_risk_user_banned_from_posting":
		return mk(social.KindNotEligible, ReasonBanned)
	case "spam_risk", "spam_risk_text":
		return mk(social.KindNotEligible, ReasonSpamRisk)
	case "publish_cancelled":
		return mk(social.KindRejected, ReasonPublishCancelled)
	case "internal":
		return mk(social.KindUnavailable, "")
	default:
		return mk(social.KindRejected, "failed")
	}
}
