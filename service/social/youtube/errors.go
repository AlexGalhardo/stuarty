package youtube

import (
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/social"
)

var (
	quotaReasons = []string{
		"quotaExceeded", "rateLimitExceeded", "dailyLimitExceeded", "userRateLimitExceeded",
		"uploadRateLimitExceeded", "uploadLimitExceeded",
	}
	authReasons = []string{"authError", "insufficientPermissions", "invalid_grant", "unauthorized"}
	// eligibilityReasons are the answers that say the channel, not the request, cannot do this.
	eligibilityReasons = []string{
		"youtubeSignupRequired", "forbidden", "channelClosed", "channelSuspended", "channelForbidden",
		"forbiddenPrivacySetting", "forbiddenLicenseSetting", "accountClosed", "accountSuspended",
	}
)

type googleError struct {
	Error struct {
		Code   int `json:"code"`
		Errors []struct {
			Reason string `json:"reason"`
		} `json:"errors"`
	} `json:"error"`
}

// reasonOf reads the stable reason code of a Google error answer. The free text of the answer is not
// read at all (SDD-020 section 7.3, T-58).
func reasonOf(body []byte) string {
	var g googleError
	if json.Unmarshal(body, &g) != nil || len(g.Error.Errors) == 0 {
		return ""
	}

	return g.Error.Errors[0].Reason
}

// classify turns an error answer of the YouTube API into a connector error (SDD-020 section 7.3).
func classify(status int, body []byte, h http.Header) *social.Error {
	reason := reasonOf(body)

	mk := func(kind string) *social.Error {
		return social.NewError(kind, publish.NetworkYouTube, reason, status, nil)
	}

	switch {
	case slices.Contains(quotaReasons, reason) || status == http.StatusTooManyRequests:
		e := mk(social.KindQuota)
		e.RetryAfter = quotaResets(time.Now(), h)

		return e
	case status == http.StatusUnauthorized || slices.Contains(authReasons, reason):
		return mk(social.KindAuth)
	case slices.Contains(eligibilityReasons, reason):
		return mk(social.KindNotEligible)
	case status >= http.StatusInternalServerError:
		return mk(social.KindUnavailable)
	default:
		return mk(social.KindInvalid)
	}
}

// quotaResets is when the daily quota comes back: midnight Pacific Time (SDD-020 section 2). A
// Retry-After header, when the network sends one in seconds, is used instead.
func quotaResets(now time.Time, h http.Header) time.Time {
	if h != nil {
		if d, err := time.ParseDuration(h.Get("Retry-After") + "s"); err == nil && d > 0 {
			return now.Add(d).UTC()
		}
	}

	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.FixedZone("PST", -8*3600)
	}

	t := now.In(loc)

	return time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc).UTC()
}
