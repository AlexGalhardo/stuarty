package service

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
)

// maxVoices bounds the list of voices of a connection, as MaxModels bounds the catalogue.
const maxVoices = 500

// ImageGenerateRequest starts an image generation in a project: the arguments of the job kind image.generate
// (SDD-016 section 6.1) and the wait and the dry run of JobStart.
type ImageGenerateRequest struct {
	ProjectID    string `json:"project_id"`
	ConnectionID string `json:"connection_id"`
	provider.ImageRequest
	WaitMS int  `json:"wait_ms,omitempty"`
	DryRun bool `json:"dry_run,omitempty"`
}

// VoiceGenerateRequest starts a speech generation in a project: the arguments of the job kind voice.generate.
type VoiceGenerateRequest struct {
	ProjectID    string `json:"project_id"`
	ConnectionID string `json:"connection_id"`
	provider.VoiceRequest
	WaitMS int  `json:"wait_ms,omitempty"`
	DryRun bool `json:"dry_run,omitempty"`
}

// VoiceListRequest reads the voices of one connection.
type VoiceListRequest struct {
	ConnectionID string `json:"connection_id"`
}

// VoiceView is a voice of a provider.
type VoiceView struct {
	ID       string `json:"id"`
	Name     string `json:"name"               untrusted:"true"`
	Language string `json:"language,omitempty"`
	// Source is provider for a list the provider answered and documented for one its documentation gives.
	Source string `json:"source"`
}

// VoiceListResult is the voices of a connection.
type VoiceListResult struct {
	ConnectionID string      `json:"connection_id"`
	Provider     string      `json:"provider"`
	Voices       []VoiceView `json:"voices"`
}

// ImageGenerate starts an image.generate job in a project and returns it. The estimate is on the job from the
// start; the job waits for a person, or for a rule, as every costly generation does (SDD-015 section 5).
func (s *svc) ImageGenerate(ctx context.Context, req ImageGenerateRequest) (*Job, error) {
	args, err := json.Marshal(ImageArgs{ConnectionID: req.ConnectionID, ImageRequest: req.ImageRequest})
	if err != nil {
		return nil, newError(command.CodeInternal, "the request cannot be encoded.")
	}

	return s.JobStart(ctx, JobStartRequest{
		ProjectID: req.ProjectID, Kind: JobImageGenerate, Args: args, WaitMS: req.WaitMS, DryRun: req.DryRun,
	})
}

// VoiceGenerate starts a voice.generate job in a project and returns it.
func (s *svc) VoiceGenerate(ctx context.Context, req VoiceGenerateRequest) (*Job, error) {
	args, err := json.Marshal(VoiceArgs{ConnectionID: req.ConnectionID, VoiceRequest: req.VoiceRequest})
	if err != nil {
		return nil, newError(command.CodeInternal, "the request cannot be encoded.")
	}

	return s.JobStart(ctx, JobStartRequest{
		ProjectID: req.ProjectID, Kind: JobVoiceGenerate, Args: args, WaitMS: req.WaitMS, DryRun: req.DryRun,
	})
}

// VoiceList reads the voices of a connection through the connection (SDD-016 section 6.3). It costs nothing.
func (s *svc) VoiceList(ctx context.Context, req VoiceListRequest) (*VoiceListResult, error) {
	if _, err := s.connectionCaller(ctx, "VoiceList"); err != nil {
		return nil, err
	}

	conn, err := s.conns.Get(ctx, req.ConnectionID)
	if err != nil {
		return nil, connectionError(err)
	}

	gen, ok := s.providers.Voice(conn.Kind)
	if !ok {
		return nil, newError(command.CodeValidation, "the connection is not a connection to a provider of speech.").
			withPath("connection_id").
			withHint("list the providers with provider:list and connect one with connection:add")
	}

	var voices []providers.Voice

	err = s.conns.With(ctx, conn.ID, func(c connections.Credential) error {
		var verr error

		voices, verr = gen.Voices(ctx, c)
		voices = slices.DeleteFunc(voices, func(v providers.Voice) bool {
			// A server that speaks the protocol may repeat the key it was sent: such a voice is not listed (T-55).
			return c.Echoes(v.ID) || c.Echoes(v.Name) || c.Echoes(v.Language)
		})

		return verr
	})
	if err != nil {
		return nil, providerError(err)
	}

	out := &VoiceListResult{ConnectionID: conn.ID, Provider: conn.Kind, Voices: make([]VoiceView, 0, len(voices))}
	seen := map[string]bool{}

	for i := range voices {
		v := &voices[i]
		if !provider.ValidVoice(v.ID) || seen[v.ID] || len(out.Voices) >= maxVoices {
			continue
		}

		seen[v.ID] = true

		source := providers.SourceProvider
		if v.Source == providers.SourceDocumented {
			source = providers.SourceDocumented
		}

		out.Voices = append(out.Voices, VoiceView{
			ID: v.ID, Name: providers.CleanName(v.Name), Language: cleanLanguage(v.Language), Source: source,
		})
	}

	slices.SortFunc(out.Voices, func(a, b VoiceView) int { return strings.Compare(a.ID, b.ID) })

	return out, nil
}

// cleanLanguage keeps a language tag a provider names, or nothing: the text is not ours.
func cleanLanguage(s string) string {
	if !provider.ValidLanguage(s) {
		return ""
	}

	return s
}
