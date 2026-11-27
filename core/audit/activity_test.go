package audit

import (
	"encoding/json"
	"testing"
)

func TestClassifyActor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		actor     string
		wantKind  string
		wantLabel string
	}{
		{"", ActorSystem, "system"},
		{"system", ActorSystem, "system"},
		{"scheduler", ActorSystem, "system"},
		{"local:ui", ActorHuman, "local:ui"},
		{"local:cli", ActorHuman, "local:cli"},
		{"user:ana@example.com", ActorHuman, "ana@example.com"},
		{"local:chat-claude", ActorAgent, "claude"},
		{"agent:codex", ActorAgent, "codex"},
		{"local:mcp", ActorAgent, "local:mcp"},
		{"mcp:cursor", ActorAgent, "cursor"},
		{"local:claude-code-aaaaaa", ActorAgent, "claude-code"},
		{"key:cht_example", ActorAgent, "example"},
	}
	for _, tc := range cases {
		kind, label := ClassifyActor(tc.actor)
		if kind != tc.wantKind || label != tc.wantLabel {
			t.Errorf("ClassifyActor(%q) = (%q, %q), want (%q, %q)",
				tc.actor, kind, label, tc.wantKind, tc.wantLabel)
		}
	}
}

func TestClassifyDomain(t *testing.T) {
	t.Parallel()

	cases := []struct {
		action string
		want   string
	}{
		{"project.create", DomainProject},
		{"template.instantiate", DomainProject},
		{"recording.start", DomainVideo},
		{"job.start", DomainVideo},
		{"image.create", DomainImage},
		{"asset.rename", DomainAsset},
		{"media.trash", DomainAsset},
		{"brand.set", DomainAsset},
		{"text.draft", DomainWriting},
		{"publication.schedule", DomainPublish},
		{"social.connect", DomainPublish},
		{"approval.decide", DomainApproval},
		{"chat.start", DomainAgent},
		{"library.write", DomainAgent},
		{"connection.create", DomainConnection},
		{"webhook.test", DomainConnection},
		{"key.create", DomainAccount},
		{"trash.empty", DomainWorkspace},
	}
	for _, tc := range cases {
		if got := ClassifyDomain(tc.action); got != tc.want {
			t.Errorf("ClassifyDomain(%q) = %q, want %q", tc.action, got, tc.want)
		}
	}
}

func TestValidateFilters(t *testing.T) {
	t.Parallel()

	for _, ok := range []string{"", "all", "human", "agent", "system"} {
		if !ValidateActorKind(ok) {
			t.Errorf("ValidateActorKind(%q) = false, want true", ok)
		}
	}
	if ValidateActorKind("robot") {
		t.Error("ValidateActorKind(robot) = true, want false")
	}

	for _, ok := range []string{"", "all", "project", "video", "image", "asset", "writing", "publish", "approval", "agent", "connection", "account", "workspace"} {
		if !ValidateDomain(ok) {
			t.Errorf("ValidateDomain(%q) = false, want true", ok)
		}
	}
	if ValidateDomain("unknown") {
		t.Error("ValidateDomain(unknown) = true, want false")
	}
}

func TestSanitizeDetailsAllowsOnlySafeMetadata(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(
		`{"count":3,"revision":"private text","nested":[{"value":"credential"}],"title":"sealed copy"}`,
	)
	got := SanitizeDetails(raw)
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("unmarshal sanitized: %v", err)
	}
	if len(m) != 1 || m["count"] != float64(3) {
		t.Fatalf("details = %v, want only count = 3", m)
	}
}
