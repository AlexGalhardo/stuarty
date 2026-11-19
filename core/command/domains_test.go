package command

import (
	"strings"
	"testing"
)

// A registry with no domain, or with a domain that SDD-000 section 1.1 does not list, refuses every command: a
// later epic cannot register operations that no domain owns (epic 13).
func TestRegistryRefusesAnOperationWithNoDomain(t *testing.T) {
	t.Parallel()

	for _, domain := range []string{"", "videos", "Video"} {
		r := NewRegistry(domain)

		err := r.Register(testCommands()[0])
		if err == nil || !strings.Contains(err.Error(), "no domain") {
			t.Errorf("registry domain %q: got %v, want a refusal for no domain", domain, err)
		}

		if len(r.List()) != 0 {
			t.Errorf("registry domain %q: the command was stored", domain)
		}
	}

	if err := (Command{}).Check(); err == nil || !strings.Contains(err.Error(), "no domain") {
		t.Errorf("a command with no domain passed Check: %v", err)
	}
}

func TestCommandsInheritTheDomainOfTheirRegistry(t *testing.T) {
	t.Parallel()

	for _, domain := range []string{DomainVideo, DomainImage, DomainWriting} {
		r := NewRegistry(domain)
		if err := r.Register(testCommands()[0]); err != nil {
			t.Fatalf("%s: %v", domain, err)
		}

		for _, c := range r.List() {
			if c.Domain != domain {
				t.Errorf("%s: %s has domain %q", domain, c.Name, c.Domain)
			}
		}

		if r.Domain() != domain {
			t.Errorf("Domain() is %q, want %q", r.Domain(), domain)
		}
	}
}

func TestACommandOfAnotherDomainIsRefused(t *testing.T) {
	t.Parallel()

	c := withSpec(testCommands()[0], func(s *Spec) { s.Domain = DomainImage })

	err := NewRegistry(DomainVideo).Register(c)
	if err == nil || !strings.Contains(err.Error(), "differs") {
		t.Errorf("got %v, want a refusal for a domain that differs from the registry's", err)
	}

	if err := NewRegistry(DomainImage).Register(c); err != nil {
		t.Errorf("a command that names its registry's domain is refused: %v", err)
	}
}

func TestEveryCommandOfTheDefaultRegistryIsVideo(t *testing.T) {
	t.Parallel()

	for _, c := range Default().List() {
		if c.Domain != DomainVideo {
			t.Errorf("%s has domain %q, want video", c.Name, c.Domain)
		}
	}
}

func TestDomainTable(t *testing.T) {
	t.Parallel()

	want := []string{"project", "video", "image", "writing", "asset", "publish", "provider", "memory", "workspace"}
	if got := DomainNames(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("domains %v, want %v", got, want)
	}

	for _, d := range Domains() {
		if d.Summary == "" || len(d.Summary) > MaxDomainSummary {
			t.Errorf("%s: the summary is %d characters, want 1 to %d", d.Name, len(d.Summary), MaxDomainSummary)
		}

		if !KnownDomain(d.Name) {
			t.Errorf("%s is listed and not known", d.Name)
		}
	}

	if KnownDomain("") || KnownDomain("videos") {
		t.Error("an unlisted domain is known")
	}
}
