package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/testproject"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// contractClips is a video provider that opens no connection and counts what it is asked: the real adapter waits
// for the approval of the provider (SDD-016 section 2.3), and the surfaces are judged by the job they share.
type contractClips struct {
	submits, polls atomic.Int32
}

func (*contractClips) Descriptor() providers.Descriptor {
	return providers.Descriptor{
		ID: "clipmaker", Name: "Clip Maker", Capabilities: []string{provider.CapabilityVideo},
		CredentialKinds: []string{provider.CredentialAPIKey}, DefaultEndpoint: "http://127.0.0.1:1/v1",
	}
}

func (*contractClips) Models(context.Context, connections.Credential) ([]providers.Model, error) {
	return []providers.Model{{ID: "clip-1", Capability: provider.CapabilityVideo}}, nil
}

func (*contractClips) VideoLimits(string) provider.VideoLimits { return provider.VideoLimits{} }

func (c *contractClips) Submit(
	context.Context, connections.Credential, provider.VideoRequest, *providers.VideoFrame,
) (string, error) {
	c.submits.Add(1)

	return "operations/contract-1", nil
}

func (c *contractClips) Poll(context.Context, connections.Credential, string) (providers.VideoStatus, error) {
	if c.polls.Add(1) < 2 {
		return providers.VideoStatus{State: providers.VideoPending, PollAfter: 10 * time.Millisecond}, nil
	}

	return providers.VideoStatus{State: providers.VideoDone}, nil
}

func (*contractClips) Fetch(context.Context, connections.Credential, string, int64) (providers.GeneratedFile, error) {
	return providers.GeneratedFile{Data: []byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00 fake clip")}, nil
}

func (*contractClips) Cancel(context.Context, connections.Credential, string) (bool, error) {
	return true, nil
}

// Acceptance criteria 1 and 2 of epic 11 on every surface (the Service, HTTP, MCP through job_start, the CLI):
// VideoGenerate returns a job that shows its estimate and waits for a person whatever the ceilings say, makes no
// call before that, and once approved ends with a clip asset whose origin names the provider, the model, the
// prompt, the cost and the actor, after one submission.
func TestVideoGenerate_EverySurfaceStartsAJobThatWaitsForAPersonAndStoresTheClip(t *testing.T) {
	master := bytes.Repeat([]byte{0x4E}, 32)

	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"in-process": newDirectWith, "http": newHTTPWith, "mcp": newMCPWith, "cli": newCLIWith,
	} {
		t.Run(name, func(t *testing.T) {
			maker := &contractClips{}
			d := newDriver(t, testenv.Options{MasterKey: master, Providers: []providers.Provider{maker}})
			svc := d.service()
			person := service.WithCaller(
				callerCtx(), service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true},
			)

			conn, err := svc.ConnectionCreate(callerCtx(), service.ConnectionCreateRequest{
				Kind: "clipmaker", Label: "Clips", Endpoint: "http://127.0.0.1:1/v1", Accepts: []string{"text"},
				Secret: providerKey,
			})
			if err != nil {
				t.Fatal(err)
			}

			micros := int64(400_000)
			if _, err := svc.ProviderPriceSet(person, service.ProviderPriceSetRequest{
				Provider: "clipmaker", Model: "clip-1", Currency: "USD", Unit: "second", UnitMicros: &micros,
			}); err != nil {
				t.Fatal(err)
			}

			project, err := testproject.Create(callerCtx(), svc, "clips")
			if err != nil {
				t.Fatal(err)
			}

			args, merr := json.Marshal(service.VideoGenerateRequest{
				ProjectID: project.ProjectID, ConnectionID: conn.ID,
				Model: "clip-1", Prompt: "A red fox in the snow", Seconds: 5, Aspect: "9:16",
			})
			if merr != nil {
				t.Fatal(merr)
			}

			raw, text, fail := d.do(t, step{Call: "VideoGenerate", Args: args})
			if fail != nil {
				t.Fatalf("%s: %+v", d.name(), fail)
			}

			var job generatedJob
			if err := json.Unmarshal(raw, &job); err != nil {
				t.Fatalf("%s: %v in %s", d.name(), err, raw)
			}

			if job.State != service.JobWaitingApproval || job.ApprovalID == "" || job.Estimate == nil ||
				job.Estimate.Micros != 2_000_000 || job.Estimate.Currency != "USD" {
				t.Fatalf("%s: job = %s, want waiting_approval with an estimate of 2000000 USD micros", d.name(), raw)
			}

			if d.name() != "http" && !strings.Contains(text, "estimated cost") {
				t.Errorf("%s: the text does not show the estimate: %q", d.name(), text)
			}

			if maker.submits.Load()+maker.polls.Load() != 0 {
				t.Fatalf("%s: the provider was reached before a person approved", d.name())
			}

			if _, err := svc.ApprovalDecide(person, service.ApprovalDecideRequest{
				ID: job.ApprovalID, Decision: "approve",
			}); err != nil {
				t.Fatal(err)
			}

			done := waitForGenerated(t, d, job.JobID)
			if done.State != service.JobSucceeded || done.Result == nil || done.Result.Generated == nil {
				t.Fatalf("%s: job after approval = %+v", d.name(), done)
			}

			res := done.Result.Generated
			if res.Provider != "clipmaker" || res.Model != "clip-1" || res.Delivered != 1 || len(res.Items) != 1 ||
				res.Cost.Micros != 2_000_000 {
				t.Fatalf("%s: result = %+v", d.name(), res)
			}

			a, aerr := svc.AssetGet(callerCtx(), service.AssetRef{Hash: res.Items[0].Hash})
			if aerr != nil {
				t.Fatal(aerr)
			}

			o := a.Origin
			if o.Kind != "generated" || o.Provider != "clipmaker" || o.Model != "clip-1" ||
				o.Prompt != "A red fox in the snow" || o.CostMicros == nil || *o.CostMicros != 2_000_000 ||
				a.CreatedBy == "" {
				t.Errorf("%s: origin = %+v, created by %q", d.name(), o, a.CreatedBy)
			}

			if n := maker.submits.Load(); n != 1 {
				t.Errorf("%s: submissions = %d, want 1", d.name(), n)
			}
		})
	}
}
