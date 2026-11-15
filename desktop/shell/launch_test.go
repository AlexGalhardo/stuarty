package shell

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLaunchURL(t *testing.T) {
	s, err := Start(context.Background(), newConfig(t, "ok"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	got, err := s.LaunchURL(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, s.URL()+"/auth/launch?code=") || !Allowed(got, s.Port) {
		t.Errorf("launch url %q", got)
	}
}

func TestLaunchURLRefusesAnImpostor(t *testing.T) {
	s, err := Start(context.Background(), newConfig(t, "impostor"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	if _, err := s.LaunchURL(context.Background()); !errors.Is(err, ErrImpostor) {
		t.Errorf("want ErrImpostor, got %v", err)
	}
}
