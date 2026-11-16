package service

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// perMinute allows n calls and then refuses, like a limiter of n a minute.
type perMinute struct{ left int }

func (p *perMinute) Allow(context.Context, string) (bool, time.Duration, error) {
	if p.left == 0 {
		return false, 12 * time.Second, nil
	}

	p.left--

	return true, 0, nil
}

type brokenLimiter struct{}

func (brokenLimiter) Allow(context.Context, string) (bool, time.Duration, error) {
	return false, 0, errors.New("the store is down")
}

func callerCtx() context.Context {
	return WithCaller(context.Background(), Caller{Actor: "key:key_0123456789", Scopes: AllScopes})
}

type keyed struct{ seen []string }

func (k *keyed) Allow(_ context.Context, key string) (bool, time.Duration, error) {
	k.seen = append(k.seen, key)

	return true, 0, nil
}

func TestHeavyLimitIgnoresTheLocalLabel(t *testing.T) {
	k := &keyed{}
	s := &svc{heavy: k}

	for _, c := range []Caller{
		{Actor: "local:agent-a"},
		{Actor: "local:agent-b"},
		{Actor: "local:ui", Browser: true},
		{Actor: "key:key_0123456789"},
		{Actor: "user:usr_1"},
	} {
		if err := s.heavyLimit(context.Background(), c); err != nil {
			t.Fatal(err)
		}
	}

	want := []string{
		"heavy:local:agents", "heavy:local:agents", "heavy:local:ui", "heavy:key:key_0123456789", "heavy:user:usr_1",
	}
	if !slices.Equal(k.seen, want) {
		t.Errorf("keys = %v, want %v", k.seen, want)
	}
}

// SDD-007 section 4.4: the 11th heavy call within a minute is E_RATE_LIMITED
// with retry_after.
func TestRateLimitHeavyOperations(t *testing.T) {
	s := &svc{heavy: &perMinute{left: HeavyPerMinute}}
	c := Caller{Actor: "key:key_0123456789"}

	for i := range HeavyPerMinute {
		if err := s.heavyLimit(context.Background(), c); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}

	err := s.heavyLimit(context.Background(), c)

	var e *Error
	if !errors.As(err, &e) || e.Code != CodeRateLimited || !e.Retryable || e.Details["retry_after"] != 12 {
		t.Errorf("the 11th call = %v, want E_RATE_LIMITED with retry_after 12", err)
	}
}

func TestRateLimitCoversTheThreeHeavyOperations(t *testing.T) {
	s := &svc{heavy: &perMinute{}}
	ctx := callerCtx()

	calls := map[string]func() error{
		"JobStart": func() error {
			_, err := s.JobStart(ctx, JobStartRequest{ProjectID: "prj_0000000001", Kind: JobExport})
			return err
		},
		"MediaImport": func() error {
			_, err := s.MediaImport(ctx, MediaImportRequest{ProjectID: "prj_0000000001", Files: []ImportFile{{}}})
			return err
		},
		"TemplateImport": func() error {
			_, err := s.TemplateImport(ctx, TemplateImportRequest{})
			return err
		},
	}

	for name, call := range calls {
		var e *Error
		if err := call(); !errors.As(err, &e) || e.Code != CodeRateLimited {
			t.Errorf("%s over the limit = %v, want E_RATE_LIMITED", name, err)
		}
	}
}

// P4-SRV-003: the operations of the BETA phase that read or write a file or a long job are heavy
// too, and the refusal names the limit, what was counted and the ceiling.
func TestRateLimitCoversTheBetaHeavyOperations(t *testing.T) {
	s := &svc{heavy: &perMinute{}, heavyLimitN: 7}
	ctx := callerCtx()

	calls := map[string]func() error{
		"ProjectBundleExport": func() error {
			_, err := s.ProjectBundleExport(ctx, ProjectBundleExportRequest{ProjectID: "prj_0000000001"})
			return err
		},
		"ProjectBundleImport": func() error {
			_, err := s.ProjectBundleImport(ctx, ProjectBundleImportRequest{Path: "/x.vvbundle"})
			return err
		},
		"FontImport": func() error {
			_, err := s.FontImport(ctx, FontImportRequest{Path: "/x.ttf"})
			return err
		},
		"ProjectOtioImport": func() error {
			_, err := s.ProjectOtioImport(ctx, ProjectOtioImportRequest{ProjectID: "prj_0000000001", Path: "/x.otio"})
			return err
		},
	}

	for name, call := range calls {
		var e *Error
		if err := call(); !errors.As(err, &e) || e.Code != CodeRateLimited ||
			e.Details["limit"] != "heavy_per_minute" || e.Details["value"] != 7 || e.Details["ceiling"] != 7 {
			t.Errorf("%s over the limit = %v, want E_RATE_LIMITED with the limit details", name, err)
		}
	}
}

// A limiter whose store fails denies the call.
func TestLimiterFailsClosed(t *testing.T) {
	s := &svc{heavy: brokenLimiter{}}

	var e *Error
	if err := s.heavyLimit(context.Background(), Caller{Actor: "local:cli"}); !errors.As(err, &e) ||
		e.Code != CodeRateLimited {
		t.Errorf("a failing limiter = %v, want E_RATE_LIMITED", err)
	}
}

func TestRateLimitWithoutALimiterCountsNothing(t *testing.T) {
	s := &svc{}

	for range 100 {
		if err := s.heavyLimit(context.Background(), Caller{Actor: "local:cli"}); err != nil {
			t.Fatal(err)
		}
	}
}
