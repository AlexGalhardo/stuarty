package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
)

// The methods of video of the shared maker: they count as calls, so a Prepare that reaches the provider fails.

func (*maker) VideoLimits(string) provider.VideoLimits {
	return provider.VideoLimits{MinSeconds: 2, MaxSeconds: 10, Aspects: []string{"16:9", "9:16"}}
}

func (m *maker) Submit(
	context.Context, connections.Credential, provider.VideoRequest, *providers.VideoFrame,
) (string, error) {
	m.calls.Add(1)

	return "task-1", nil
}

func (m *maker) Poll(context.Context, connections.Credential, string) (providers.VideoStatus, error) {
	m.calls.Add(1)

	return providers.VideoStatus{State: providers.VideoDone}, nil
}

func (m *maker) Fetch(context.Context, connections.Credential, string, int64) (providers.GeneratedFile, error) {
	m.calls.Add(1)

	return providers.GeneratedFile{}, nil
}

func (m *maker) Cancel(context.Context, connections.Credential, string) (bool, error) {
	m.calls.Add(1)

	return true, nil
}

func videoStart(args string) JobStartRequest {
	return JobStartRequest{Kind: JobVideoGenerate, Args: json.RawMessage(args)}
}

func newVideoRig(t *testing.T) *generateRig {
	t.Helper()

	return newGenerateRig(t, provider.CapabilityText, provider.CapabilityVideo)
}

func TestVideoGenerate_PrepareEstimatesAndNeverCallsTheProvider(t *testing.T) {
	t.Parallel()

	r := newVideoRig(t)
	r.setPrice(t, provider.Price{Currency: "USD", Unit: provider.UnitSecond, UnitMicros: 400_000})

	prep, err := r.s.prepareVideo(context.Background(), videoStart(
		`{"connection_id":"`+r.conn+`","model":"m1","prompt":"A red fox in the snow","seconds":6,"aspect":"9:16"}`))
	if err != nil {
		t.Fatal(err)
	}

	if prep.Estimate != (core.Money{Micros: 2_400_000, Currency: "USD"}) || prep.ConnectionID != r.conn {
		t.Fatalf("prep = %+v", prep)
	}

	for _, want := range []string{"6 seconds", "9:16", "Maker", "m1", "A red fox in the snow"} {
		if !strings.Contains(prep.Summary, want) {
			t.Errorf("the summary lacks %q: %q", want, prep.Summary)
		}
	}

	if !core.ValidSummary(prep.Summary) {
		t.Errorf("the summary is not valid for a person: %q", prep.Summary)
	}

	var in generateInput
	if err := json.Unmarshal(prep.Input, &in); err != nil || in.Video == nil || in.Video.Seconds != 6 ||
		in.TaskID != "" || in.Price.UnitMicros != 400_000 || in.PriceSource != provider.SourceUser ||
		in.Estimate != prep.Estimate {
		t.Fatalf("the input of the job keeps the price the person saw and no task id yet: %+v, %v", in, err)
	}

	if n := r.maker.calls.Load(); n != 0 {
		t.Fatalf("Prepare made %d calls to the provider", n)
	}
}

func TestVideoGenerate_DefaultsAreApplied(t *testing.T) {
	t.Parallel()

	r := newVideoRig(t)
	r.setPrice(t, provider.Price{Currency: "USD", Unit: provider.UnitSecond, UnitMicros: 10})

	prep, err := r.s.prepareVideo(context.Background(),
		videoStart(`{"connection_id":"`+r.conn+`","model":"m1","prompt":"p"}`))
	if err != nil || prep.Estimate.Micros != 50 || !strings.Contains(prep.Summary, "5 seconds, aspect 16:9") {
		t.Fatalf("got %+v, %v", prep, err)
	}
}

func TestVideoGenerate_UnknownPriceIsRefusedNeverZero(t *testing.T) {
	t.Parallel()

	r := newVideoRig(t)
	args := `{"connection_id":"` + r.conn + `","model":"m1","prompt":"p"}`

	check := func(name string) {
		t.Helper()

		_, err := r.s.prepareVideo(context.Background(), videoStart(args))

		e, ok := errors.AsType[*Error](err)
		if !ok || e.Code != CodePriceUnknown || e.Details["unit"] != provider.UnitSecond {
			t.Fatalf("%s: got %v", name, err)
		}
	}

	check("no price at all")

	r.setPrice(t, provider.Price{Currency: "USD", InputMicrosPerMTok: 1, OutputMicrosPerMTok: 1})
	check("a price of tokens")

	r.setPrice(t, provider.Price{Currency: "USD", Unit: provider.UnitImage, UnitMicros: 1})
	check("a price per image")

	if n := r.maker.calls.Load(); n != 0 {
		t.Fatalf("the provider was called %d times", n)
	}
}

func TestVideoGenerate_ArgsAreStrictAndLimitsComeFromTheAdapter(t *testing.T) {
	t.Parallel()

	r := newVideoRig(t)
	r.setPrice(t, provider.Price{Currency: "USD", Unit: provider.UnitSecond, UnitMicros: 1})

	for name, raw := range map[string]string{
		"empty":          ``,
		"unknown member": `{"connection_id":"` + r.conn + `","model":"m1","prompt":"p","url":"http://x"}`,
		"wrong type":     `{"connection_id":"` + r.conn + `","model":"m1","prompt":3}`,
	} {
		_, err := r.s.prepareVideo(context.Background(), videoStart(raw))

		if e, ok := errors.AsType[*Error](err); !ok || e.Code != "E_VALIDATION" || e.Path != "args" {
			t.Errorf("%s: got %v", name, err)
		}
	}

	for name, tc := range map[string]struct{ args, path string }{
		"shorter than the model makes": {`"seconds":1`, "args.seconds"},
		"longer than the model makes":  {`"seconds":11`, "args.seconds"},
		"an aspect the model lacks":    {`"aspect":"1:1"`, "args.aspect"},
		"an id that is not an asset":   {`"first_frame":"../x"`, "args.first_frame"},
	} {
		body := `{"connection_id":"` + r.conn + `","model":"m1","prompt":"p",` + tc.args + `}`

		_, err := r.s.prepareVideo(context.Background(), videoStart(body))

		if e, ok := errors.AsType[*Error](err); !ok || e.Code != "E_VALIDATION" || e.Path != tc.path {
			t.Errorf("%s: got %v", name, err)
		}
	}

	// A start with no connection names every argument in its hint, since job_start has no room for them.
	_, err := r.s.prepareVideo(context.Background(), videoStart(`{"model":"m1","prompt":"p"}`))
	if e, ok := errors.AsType[*Error](err); !ok || e.Path != "args.connection_id" ||
		!strings.Contains(e.Hint, "seconds") || !strings.Contains(e.Hint, "first_frame") {
		t.Fatalf("got %v", err)
	}
}

func TestVideoGenerate_ARequestForAnotherCapabilityIsRefused(t *testing.T) {
	t.Parallel()

	// A provider that makes text and images, not video.
	r := newGenerateRig(t, provider.CapabilityText, provider.CapabilityImage)
	r.setPrice(t, provider.Price{Currency: "USD", Unit: provider.UnitSecond, UnitMicros: 1})

	_, err := r.s.prepareVideo(context.Background(),
		videoStart(`{"connection_id":"`+r.conn+`","model":"m1","prompt":"p"}`))
	if e, ok := errors.AsType[*Error](err); !ok || e.Code != "E_VALIDATION" || e.Path != "args.connection_id" {
		t.Fatalf("got %v", err)
	}
}

func TestVideoGenerate_ThePollingWaitsWhatTheProviderAsksWithinBounds(t *testing.T) {
	t.Parallel()

	if !pollMayPass(&providers.Error{Kind: providers.KindRateLimited}) ||
		!pollMayPass(&providers.Error{Kind: providers.KindUnavailable}) ||
		!pollMayPass(context.DeadlineExceeded) {
		t.Error("a rate limit, an outage and a timeout may pass")
	}

	if pollMayPass(&providers.Error{Kind: providers.KindAuth}) ||
		pollMayPass(&providers.Error{Kind: providers.KindBadRequest}) {
		t.Error("a refused key or a refused request will not pass")
	}

	for _, tc := range []struct {
		asked, backoff, next, after time.Duration
	}{
		// A provider that asks for a tiny wait is not polled in a tight loop.
		{time.Millisecond, videoPollFirst, time.Second, videoPollFirst},
		{10 * time.Millisecond, videoPollFirst, time.Second, videoPollFirst},
		{5 * time.Second, videoPollFirst, 5 * time.Second, videoPollFirst},
		{time.Hour, videoPollFirst, videoPollCeiling, videoPollFirst},
		// With no wait asked, the backoff doubles up to its largest.
		{0, videoPollFirst, 2 * time.Second, 4 * time.Second},
		{0, 16 * time.Second, 16 * time.Second, videoPollMax},
		{0, videoPollMax, videoPollMax, videoPollMax},
		{-time.Second, videoPollFirst, videoPollFirst, 4 * time.Second},
	} {
		next, after := nextPoll(tc.asked, tc.backoff)
		if next != tc.next || after != tc.after {
			t.Errorf("nextPoll(%v, %v) = %v, %v; want %v, %v", tc.asked, tc.backoff, next, after, tc.next, tc.after)
		}
	}
}
