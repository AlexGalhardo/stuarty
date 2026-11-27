// Package audit holds the pure rules for classifying and filtering activity
// history entries across Stuarty (epic 111, ADR-0007).
package audit

import (
	"encoding/json"
	"slices"
	"strings"
)

// Actor kinds shown by the Activity History timeline.
const (
	ActorHuman  = "human"
	ActorAgent  = "agent"
	ActorSystem = "system"
)

// Domains shown by the Activity History filter bar.
const (
	DomainProject    = "project"
	DomainVideo      = "video"
	DomainImage      = "image"
	DomainAsset      = "asset"
	DomainWriting    = "writing"
	DomainPublish    = "publish"
	DomainApproval   = "approval"
	DomainAgent      = "agent"
	DomainConnection = "connection"
	DomainAccount    = "account"
	DomainWorkspace  = "workspace"
)

var allowedActorKinds = []string{
	"all",
	ActorHuman,
	ActorAgent,
	ActorSystem,
}

var allowedDomains = []string{
	"all",
	DomainProject,
	DomainVideo,
	DomainImage,
	DomainAsset,
	DomainWriting,
	DomainPublish,
	DomainApproval,
	DomainAgent,
	DomainConnection,
	DomainAccount,
	DomainWorkspace,
}

// AllowedActorKinds returns the accepted values for the actor_kind filter.
func AllowedActorKinds() []string {
	out := make([]string, len(allowedActorKinds))
	copy(out, allowedActorKinds)
	return out
}

// AllowedDomains returns the accepted values for the domain filter.
func AllowedDomains() []string {
	out := make([]string, len(allowedDomains))
	copy(out, allowedDomains)
	return out
}

// ValidateActorKind reports whether kind is empty or a known actor filter.
func ValidateActorKind(kind string) bool {
	return kind == "" || slices.Contains(allowedActorKinds, kind)
}

// ValidateDomain reports whether domain is empty or a known domain filter.
func ValidateDomain(domain string) bool {
	return domain == "" || slices.Contains(allowedDomains, domain)
}

// ClassifyActor determines whether an audit actor identifier belongs to a human,
// an AI agent, or the system, and returns a clean display label.
func ClassifyActor(actor string) (kind, label string) {
	trimmed := strings.TrimSpace(actor)
	low := strings.ToLower(trimmed)

	switch {
	case trimmed == "", low == "system", low == "scheduler", low == "internal", strings.HasPrefix(low, "system:"):
		return ActorSystem, "system"
	case strings.HasPrefix(low, "local:chat-"),
		strings.HasPrefix(low, "local:mcp-"),
		strings.HasPrefix(low, "chat:"),
		strings.HasPrefix(low, "agent:"),
		strings.HasPrefix(low, "mcp:"),
		strings.HasPrefix(low, "key:cht_"),
		low == "local:mcp",
		strings.Contains(low, ":chat-"),
		strings.Contains(low, ":agent:"):
		return ActorAgent, agentLabel(trimmed)
	default:
		if name, ok := mcpSessionName(low); ok {
			return ActorAgent, name
		}
		return ActorHuman, humanLabel(trimmed)
	}
}

// An MCP stdio caller without an explicit actor uses local:<name>-<session>.
func mcpSessionName(actor string) (string, bool) {
	label, local := strings.CutPrefix(actor, "local:")
	split := strings.LastIndexByte(label, '-')
	if !local || split < 1 || len(label)-split-1 != 6 {
		return "", false
	}
	for _, c := range label[split+1:] {
		if !strings.ContainsRune("0123456789abcdefghjkmnpqrstvwxyz", c) {
			return "", false
		}
	}
	return label[:split], true
}

func agentLabel(actor string) string {
	for _, prefix := range []string{"local:chat-", "chat:", "agent:", "mcp:", "key:cht_"} {
		if strings.HasPrefix(strings.ToLower(actor), prefix) {
			rest := strings.TrimSpace(actor[len(prefix):])
			if rest != "" {
				return rest
			}
		}
	}
	return actor
}

func humanLabel(actor string) string {
	switch strings.ToLower(actor) {
	case "local:ui", "local:cli", "local:api":
		return actor
	}
	for _, prefix := range []string{"user:", "session:"} {
		if strings.HasPrefix(strings.ToLower(actor), prefix) {
			rest := strings.TrimSpace(actor[len(prefix):])
			if rest != "" {
				return rest
			}
		}
	}
	return actor
}

// ClassifyDomain maps a canonical audit action name to its product domain.
func ClassifyDomain(action string) string {
	a := strings.ToLower(strings.TrimSpace(action))
	switch {
	case strings.HasPrefix(a, "job.image."):
		return DomainImage
	case strings.HasPrefix(a, "job.text."):
		return DomainWriting
	case strings.HasPrefix(a, "job.voice."):
		return DomainAsset
	case strings.HasPrefix(a, "job.publish."):
		return DomainPublish
	case strings.HasPrefix(a, "project."), strings.HasPrefix(a, "template."):
		return DomainProject
	case strings.HasPrefix(a, "timeline."),
		strings.HasPrefix(a, "recording."),
		strings.HasPrefix(a, "export."),
		strings.HasPrefix(a, "transcribe."),
		a == "job.start",
		strings.HasPrefix(a, "job."):
		return DomainVideo
	case strings.HasPrefix(a, "image."):
		return DomainImage
	case strings.HasPrefix(a, "asset."),
		strings.HasPrefix(a, "media."),
		strings.HasPrefix(a, "font."),
		strings.HasPrefix(a, "brand."):
		return DomainAsset
	case strings.HasPrefix(a, "text."), strings.HasPrefix(a, "brief."):
		return DomainWriting
	case strings.HasPrefix(a, "publication."),
		strings.HasPrefix(a, "calendar."),
		strings.HasPrefix(a, "social."):
		return DomainPublish
	case strings.HasPrefix(a, "approval."):
		return DomainApproval
	case strings.HasPrefix(a, "chat."),
		strings.HasPrefix(a, "agent."),
		strings.HasPrefix(a, "library."),
		strings.HasPrefix(a, "skill."),
		strings.HasPrefix(a, "prompt."),
		strings.HasPrefix(a, "memory."):
		return DomainAgent
	case strings.HasPrefix(a, "connection."),
		strings.HasPrefix(a, "provider."),
		strings.HasPrefix(a, "webhook."):
		return DomainConnection
	case strings.HasPrefix(a, "account."),
		strings.HasPrefix(a, "key."),
		strings.HasPrefix(a, "user."):
		return DomainAccount
	default:
		return DomainWorkspace
	}
}

// SanitizeDetails exposes only known numeric and boolean metadata from audit
// details. Older entries can contain free-form strings under arbitrary keys.
func SanitizeDetails(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	cleaned := make(map[string]any)
	for _, key := range []string{"count", "revision", "target_revision", "changes", "dry_run"} {
		switch value := obj[key].(type) {
		case float64, bool:
			cleaned[key] = value
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	out, err := json.Marshal(cleaned)
	if err != nil {
		return nil
	}
	return out
}
