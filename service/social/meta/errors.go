package meta

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// The reasons a Meta error is given (SDD-020 section 19). They are stable codes: the text Meta writes is
// never kept, so it cannot reach a person or an agent.
const (
	ReasonTokenInvalid      = "token_invalid"
	ReasonTokenExpired      = "token_expired"
	ReasonPasswordChanged   = "password_changed"
	ReasonAppRemoved        = "app_removed"
	ReasonPermissionMissing = "permission_missing"
	ReasonRateLimit         = "rate_limit"
	ReasonPublishingLimit   = "publishing_limit"
	ReasonAccountRestricted = "account_restricted"
	ReasonSpam              = "spam"
	ReasonBlocked           = "blocked"
	ReasonUnsupportedFormat = "unsupported_format"
	ReasonAspectRatio       = "aspect_ratio"
	ReasonResolution        = "resolution"
	ReasonDuration          = "duration"
	ReasonFrameRate         = "frame_rate"
	ReasonCaptionTooLong    = "caption_too_long"
	ReasonInvalidParameter  = "invalid_parameter"
	ReasonProcessingFailed  = "processing_failed"
	ReasonUploadFailed      = "upload_failed"
	ReasonMediaExpired      = "media_expired"
	ReasonNotReady          = "not_ready"
	ReasonStillProcessing   = "still_processing"
	ReasonNoAccounts        = "no_accounts"
	ReasonBadSecret         = "bad_secret"
	ReasonNeedsSignIn       = "needs_sign_in"
)

// The subcodes of Instagram that decide a kind (SDD-020 section 15).
const (
	subServer          = 2207001
	subDownloadTimeout = 2207003
	subMediaNotFound   = 2207006
	subContainerGone   = 2207008
	subCaptionTooLong  = 2207010
	subMediaExpired    = 2207020
	subNotReady        = 2207027
	subCreationFailed  = 2207032
	subPublishLimit    = 2207042
	subAccountLimited  = 2207050
	subSpam            = 2207051
	subFetchFailed     = 2207052
	subUploadUnknown   = 2207053
	subUnsupported     = 2207026
)

type graphErrorBody struct {
	Error *struct {
		Code    int    `json:"code"`
		Subcode int    `json:"error_subcode"`
		Type    string `json:"type"`
	} `json:"error"`
	// DebugInfo is the error of the upload host, which does not use the Graph envelope.
	DebugInfo *struct {
		Type string `json:"type"`
	} `json:"debug_info"`
}

// GateError says whether err is the refusal of the outbound gate, which the service maps to its own code and
// which a connector never rewrites.
func GateError(err error) bool {
	return errors.Is(err, outbound.ErrNoConnection) || errors.Is(err, outbound.ErrDestination) ||
		errors.Is(err, outbound.ErrNotPublic)
}

// TransportError turns a failure to reach Meta into a connector error; the refusal of the gate and a
// cancelled context pass through.
func TransportError(network string, err error) error {
	if GateError(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	return social.NewError(social.KindUnavailable, network, "", 0, err)
}

// Classify maps an answer of Meta that was not a success to a connector error. Only the numbers of the
// answer are read; none of its text is kept.
func Classify(network string, status int, body []byte) error {
	var env graphErrorBody

	_ = json.Unmarshal(body, &env)

	var kind, reason string

	switch {
	case env.Error != nil:
		kind, reason = byCode(env.Error.Code, env.Error.Subcode, status)
	case env.DebugInfo != nil:
		kind, reason = byUploadType(env.DebugInfo.Type)
	default:
		kind, reason = byStatus(status)
	}

	return social.NewError(kind, network, reason, status, nil)
}

func byStatus(status int) (kind, reason string) {
	switch {
	case status >= http.StatusInternalServerError:
		return social.KindUnavailable, ""
	case status == http.StatusUnauthorized:
		return social.KindAuth, ReasonTokenInvalid
	case status == http.StatusTooManyRequests:
		return social.KindQuota, ReasonRateLimit
	}

	return social.KindInvalid, "http_" + strconv.Itoa(status)
}

// byUploadType maps the error type the upload host names.
func byUploadType(t string) (kind, reason string) {
	switch t {
	case "ProcessingFailedError":
		return social.KindRejected, ReasonProcessingFailed
	case "OffsetInvalidError", "PartialRequestError":
		return social.KindUnavailable, ReasonUploadFailed
	}

	return social.KindUnavailable, ReasonUploadFailed
}

//nolint:cyclop,gocyclo // one table of the codes of SDD-020 section 19, flat on purpose
func byCode(code, sub, status int) (kind, reason string) {
	switch {
	case code == 190:
		return social.KindAuth, tokenReason(sub)
	case code == 10 || code >= 200 && code <= 299:
		return social.KindAuth, ReasonPermissionMissing
	case sub == subSpam:
		return social.KindNotEligible, ReasonSpam
	case code == 9 || sub == subPublishLimit:
		return social.KindQuota, ReasonPublishingLimit
	case code == 4 || code == 17 || code == 32 || code == 341 || code == 613:
		return social.KindQuota, ReasonRateLimit
	case code == 25 || sub == subAccountLimited:
		return social.KindNotEligible, ReasonAccountRestricted
	case code == 368:
		return social.KindNotEligible, ReasonBlocked
	case code == 352 || sub == subUnsupported:
		return social.KindInvalid, ReasonUnsupportedFormat
	case code == 1363040 || sub == 1363040:
		return social.KindInvalid, ReasonAspectRatio
	case code == 1363127 || sub == 1363127:
		return social.KindInvalid, ReasonResolution
	case code == 1363128 || sub == 1363128:
		return social.KindInvalid, ReasonDuration
	case code == 1363129 || sub == 1363129:
		return social.KindInvalid, ReasonFrameRate
	case code == 36004 || sub == subCaptionTooLong:
		return social.KindInvalid, ReasonCaptionTooLong
	case code == 9007 || sub == subNotReady:
		return social.KindUnavailable, ReasonNotReady
	case sub == subMediaExpired || sub == subContainerGone || sub == subMediaNotFound || code == 24:
		return social.KindUnavailable, ReasonMediaExpired
	case sub == subUploadUnknown || sub == subCreationFailed:
		return social.KindUnavailable, ReasonUploadFailed
	case code == 6000:
		return social.KindRejected, ReasonUploadFailed
	case sub == subFetchFailed:
		return social.KindRejected, ReasonUploadFailed
	case code == 1 || code == 2 || code == -1 || code == -2 || sub == subServer || sub == subDownloadTimeout ||
		status >= http.StatusInternalServerError:
		return social.KindUnavailable, ""
	case code == 100:
		return social.KindInvalid, ReasonInvalidParameter
	}

	return social.KindInvalid, "code_" + strconv.Itoa(code)
}

func tokenReason(sub int) string {
	switch sub {
	case 463:
		return ReasonTokenExpired
	case 460:
		return ReasonPasswordChanged
	case 458:
		return ReasonAppRemoved
	}

	return ReasonTokenInvalid
}

// TokenError turns a refusal of the token endpoint into a connector error.
func TokenError(network string, err error) error {
	te, ok := errors.AsType[*oauth.TokenError](err)
	if !ok {
		return TransportError(network, err)
	}

	switch {
	case te.Status >= http.StatusInternalServerError:
		return social.NewError(social.KindUnavailable, network, "", te.Status, err)
	case te.Status == http.StatusTooManyRequests:
		return social.NewError(social.KindQuota, network, ReasonRateLimit, te.Status, err)
	}

	// A wrong app id or secret, a code used twice or one that expired: Meta answers 400 for each.
	return social.NewError(social.KindAuth, network, "refused", te.Status, err)
}
