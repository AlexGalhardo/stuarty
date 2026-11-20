package destinations

import (
	"testing"
	"time"
)

// FuzzRetryAfter feeds any header value from a receiver to the reader of Retry-After (SDD-008 section 13.2): the
// wait is never negative and never more than the cap, whatever the receiver says.
func FuzzRetryAfter(f *testing.F) {
	for _, v := range []string{"", "0", "5", "-3", " 7 ", "99999999999999999999", "Wed, 21 Oct 2026 07:28:00 GMT", "1e9", "\x00"} {
		f.Add(v)
	}

	f.Fuzz(func(t *testing.T, header string) {
		if d := retryAfter(header); d < 0 || d > maxRetryAfter {
			t.Fatalf("retryAfter(%q) = %v, outside 0 to %v", header, d, maxRetryAfter)
		}
	})
}

// T-100: a receiver that sends more seconds than a Duration holds gets the cap, not a negative wait.
func TestSecurity_T100_ReceiverRetryAfterNeverMakesAnOverflowedOrLongWait(t *testing.T) {
	t.Parallel()

	for _, v := range []string{"9227000000", "9223372036854775807", "31"} {
		if d := retryAfter(v); d != maxRetryAfter {
			t.Errorf("retryAfter(%q) = %v, want the cap %v", v, d, maxRetryAfter)
		}
	}

	if d := retryAfter("5"); d != 5*time.Second {
		t.Errorf("retryAfter(5) = %v", d)
	}
}
