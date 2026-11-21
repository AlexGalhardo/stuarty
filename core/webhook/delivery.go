package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// The states of a delivery (SDD-022 section 4).
const (
	StatePending   = "pending"
	StateSucceeded = "succeeded"
	StateFailed    = "failed"
)

// The numbers of a delivery.
const (
	// Timeout is how long one request may take.
	Timeout = 10 * time.Second
	// MaxRetries is how many times a delivery is tried again after the first attempt.
	MaxRetries = 5
	// MaxAttempts is the first attempt and the retries.
	MaxAttempts = 1 + MaxRetries
	// VaultLockedWait is how long a delivery waits when the vault cannot be opened; the wait is not an attempt.
	VaultLockedWait = 30 * time.Second
	// MaxAnswerBytes is how much of an answer is read, only to count it.
	MaxAnswerBytes = 4 << 10
	// ReplayWindow is how far from its own clock a receiver should accept the time of a signature.
	ReplayWindow = 5 * time.Minute
	// KeepAttempts is how many attempts of a webhook are kept.
	KeepAttempts = 50
	// KeepFinished is how long a finished delivery is kept.
	KeepFinished = 30 * 24 * time.Hour
)

// The headers of a delivery.
const (
	HeaderSignature = "Stuarty-Signature"
	HeaderEvent     = "Stuarty-Event"
	HeaderDelivery  = "Stuarty-Delivery"
	UserAgent       = "Stuarty-Webhooks/1"
)

// Backoff is the wait before retry number n (1 to MaxRetries): 10 s, 30 s, 90 s, 270 s and 810 s.
func Backoff(retry int) time.Duration {
	if retry < 1 || retry > MaxRetries {
		return 0
	}

	d := 10 * time.Second
	for i := 1; i < retry; i++ {
		d *= 3
	}

	return d
}

// Outcome is what one attempt means for the delivery.
type Outcome int

// The outcomes of an attempt.
const (
	// OutcomeDelivered is a 2xx answer.
	OutcomeDelivered Outcome = iota
	// OutcomeRetry is a failure that may pass: a network error, a timeout, a 5xx, a 408 or a 429.
	OutcomeRetry
	// OutcomeRefused is an answer that says no: the receiver is not asked again.
	OutcomeRefused
)

// Classify says what an attempt means. status is the HTTP status, 0 when there was none (a network error).
func Classify(status int) Outcome {
	switch {
	case status >= 200 && status < 300:
		return OutcomeDelivered
	case status == 0, status >= 500, status == 408, status == 429:
		return OutcomeRetry
	}

	return OutcomeRefused
}

// Next says what follows an attempt: the state of the delivery and, when it stays pending, the wait before the
// next one. attempts counts the attempt that just ended; max is the most the delivery may make.
func Next(outcome Outcome, attempts, max int) (state string, wait time.Duration) {
	switch {
	case outcome == OutcomeDelivered:
		return StateSucceeded, 0
	case outcome == OutcomeRetry && attempts < max:
		return StatePending, Backoff(attempts)
	}

	return StateFailed, 0
}

// Secret-related rules.

// SecretPrefix starts every signing secret.
const SecretPrefix = "whsec_"

// NewSecret makes a signing secret: whsec_ and 43 URL-safe characters of 32 random bytes. r is the source of the
// bytes (crypto/rand in production).
func NewSecret(r io.Reader) (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}

	return SecretPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// Sign is the value of the header Stuarty-Signature: t=<unix seconds>,v1=<hex HMAC-SHA256 of "<t>.<body>">, with the
// secret as the key.
func Sign(secret string, at time.Time, body []byte) string {
	t := at.Unix()

	return "t=" + strconv.FormatInt(t, 10) + ",v1=" + mac(secret, t, body)
}

func mac(secret string, t int64, body []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(strconv.FormatInt(t, 10)))
	h.Write([]byte("."))
	h.Write(body)

	return hex.EncodeToString(h.Sum(nil))
}

// ErrSignature is a signature that does not verify.
var ErrSignature = errors.New("webhook: the signature does not verify")

// Verify checks a Stuarty-Signature header against the secret and the exact bytes of the body, and refuses a time
// that is more than tolerance from now. It is what a receiver does; the product uses it in its tests and a person can
// read it as the reference of SDD-022 section 4.
func Verify(secret, header string, body []byte, now time.Time, tolerance time.Duration) error {
	var (
		t   int64
		sig string
		ok  bool
	)

	for _, part := range strings.Split(header, ",") {
		k, v, found := strings.Cut(strings.TrimSpace(part), "=")
		switch {
		case !found:
			return ErrSignature
		case k == "t":
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return ErrSignature
			}

			t, ok = n, true
		case k == "v1":
			sig = v
		}
	}

	if !ok || sig == "" {
		return ErrSignature
	}

	want := mac(secret, t, body)
	if !hmac.Equal([]byte(want), []byte(strings.ToLower(sig))) {
		return ErrSignature
	}

	if d := now.Sub(time.Unix(t, 0)); d > tolerance || d < -tolerance {
		return fmt.Errorf("%w: the time is outside the window", ErrSignature)
	}

	return nil
}

// The short codes an attempt records as its error (SDD-022 section 4). Never the message of a receiver.
const (
	ErrCodeTimeout        = "timeout"
	ErrCodeConnect        = "connect"
	ErrCodeRefusedAddress = "refused_address"
	ErrCodeStatus         = "status"
	ErrCodeVaultLocked    = "vault_locked"
	ErrCodeInvalidURL     = "invalid_url"
)
