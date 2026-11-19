package service

import (
	"time"

	"github.com/AlexGalhardo/stuarty/service/social"
)

// socialError maps what a connector returned. A failure of the network becomes E_PUBLISH; the refusal of
// the outbound gate and the failures of the vault keep their own codes (SDD-014 section 7).
func (s *svc) socialError(err error) error {
	if se, ok := social.AsError(err); ok {
		return publishError(se, s.networkHints(se.Network))
	}

	return connectionError(err)
}

// networkHints are the hints the connector of a network wrote for its errors (SDD-020 section 22); nil for a
// network the build does not know.
func (s *svc) networkHints(network string) map[string]string {
	if s.social == nil {
		return nil
	}

	conn, err := s.social.Get(network)
	if err != nil {
		return nil
	}

	return conn.Describe().Hints
}

// publishError writes the message of a connector error. The message is written here from the kind and
// the reason code, and never copied from the answer of the network (SDD-020 section 7.3, T-58). The hint is
// the one the connector of the network wrote for that kind, so that an error of one network never sends the
// person to another's screen; a kind it wrote none for gets a hint that names no network.
func publishError(se *social.Error, hints map[string]string) *Error {
	e := newError(CodePublish, "%s", publishMessage(se)).
		withDetail("kind", se.Kind).
		withDetail("network", se.Network)

	if se.Reason != "" {
		e = e.withDetail("reason", se.Reason)
	}

	if se.Kind == social.KindQuota && !se.RetryAfter.IsZero() {
		e = e.withDetail("retry_after", se.RetryAfter.UTC().Format(time.RFC3339))
	}

	hint := hints[se.Kind]
	if hint == "" {
		hint = genericHint(se.Kind)
	}

	e = e.withHint("%s", hint)
	e.Retryable = se.Retryable()

	return e
}

// genericHint is what a person does about an error of a kind when the connector wrote no hint of its own. It
// names no network.
func genericHint(kind string) string {
	switch kind {
	case social.KindQuota:
		return "a limit of the network was used; wait, then publish the same post again"
	case social.KindAuth:
		return "sign in to the account again from the Connections page"
	case social.KindNotEligible:
		return "check the account on the network itself: its verification, restrictions and settings; then publish again"
	case social.KindInvalid:
		return "fix the fields or the files named in the reason and publish again"
	case social.KindRejected:
		return "the network refused the video itself; read its reason on the network before trying again"
	}

	return "try again later; publishing the same post again resumes the upload"
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
