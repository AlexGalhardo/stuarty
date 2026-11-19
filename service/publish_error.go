package service

import (
	"time"

	"github.com/AlexGalhardo/stuarty/service/social"
)

// socialError maps what a connector returned. A failure of the network becomes E_PUBLISH; the refusal of
// the outbound gate and the failures of the vault keep their own codes (SDD-014 section 7).
func (s *svc) socialError(err error) error {
	if se, ok := social.AsError(err); ok {
		return publishError(se)
	}

	return connectionError(err)
}

// publishError writes the message of a connector error. The message is written here from the kind and
// the reason code, and never copied from the answer of the network (SDD-020 section 7.3, T-58).
func publishError(se *social.Error) *Error {
	e := newError(CodePublish, "%s", publishMessage(se)).
		withDetail("kind", se.Kind).
		withDetail("network", se.Network)

	if se.Reason != "" {
		e = e.withDetail("reason", se.Reason)
	}

	switch se.Kind {
	case social.KindQuota:
		if !se.RetryAfter.IsZero() {
			e = e.withDetail("retry_after", se.RetryAfter.UTC().Format(time.RFC3339))
		}

		e = e.withHint(
			"the limit of the day was used and it comes back at midnight Pacific Time; publish the same post " +
				"again then, and the upload resumes where it stopped",
		)
	case social.KindAuth:
		e = e.withHint("sign in to the account again from the Connections page; if your Google project is in the " +
			"Testing status, its sign-in expires after 7 days")
	case social.KindNotEligible:
		e = e.withHint("check the channel in YouTube Studio: verification, restrictions and the privacy setting; " +
			"then publish again")
	case social.KindInvalid:
		e = e.withHint("fix the title, the description, the tags or the files named in the reason and publish again")
	case social.KindRejected:
		e = e.withHint("the network refused the video itself; read its reason in YouTube Studio before trying again")
	default:
		e = e.withHint("try again later; publishing the same post again resumes the upload")
	}

	e.Retryable = se.Retryable()

	return e
}

func publishMessage(se *social.Error) string {
	suffix := ""
	if se.Reason != "" {
		suffix = " (" + se.Reason + ")"
	}

	switch se.Kind {
	case social.KindQuota:
		return "the quota or the upload limit of " + se.Network + " was reached" + suffix + "."
	case social.KindAuth:
		return "the sign-in to " + se.Network + " does not work" + suffix + "."
	case social.KindNotEligible:
		return "the account cannot publish this to " + se.Network + suffix + "."
	case social.KindInvalid:
		return se.Network + " refused the post" + suffix + "."
	case social.KindRejected:
		return se.Network + " rejected the video" + suffix + "."
	default:
		return se.Network + " could not be reached or failed" + suffix + "."
	}
}
