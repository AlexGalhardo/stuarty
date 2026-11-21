package servicetest

import (
	"context"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/assets"
)

const generatedHash = "sha256:" + "ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12"

// seedAsset puts an active asset in the project with the origin given: the row of the file, the row of the
// project that holds it and the library row that records where it came from.
func (p *pubEnv) seedAsset(project, hash string, kind asset.OriginKind) {
	p.t.Helper()

	now := p.clock.now().UnixMilli()

	err := p.store.Write(context.Background(), func(ctx context.Context, tx service.Tx) error {
		row := service.AssetRow{
			Hash: hash, Kind: "video", SizeBytes: 10, OriginalName: "clip.mp4", IngestState: service.IngestReady,
			CreatedMs: now, UpdatedMs: now,
		}
		if _, _, err := tx.Assets().Upsert(ctx, row); err != nil {
			return err
		}

		if err := tx.Assets().PutProjectAsset(ctx, service.ProjectAssetRow{
			ProjectID: project, AssetID: "ast_disclose01", AssetHash: hash, State: service.ProjectAssetActive,
			Name: "clip.mp4", ImportedBy: ui, ImportedMs: now,
		}); err != nil {
			return err
		}

		return tx.AssetLibrary().Put(ctx, assets.Row{
			Hash: hash, Origin: asset.Origin{Kind: kind, Provider: "acme", Model: "clip-1", Prompt: "a cat"},
			CreatedBy: ui, CreatedMs: now, UpdatedMs: now,
		})
	})
	if err != nil {
		p.t.Fatal(err)
	}
}

// RF-407, SDD-020 section 20: the rule counts every asset the project holds, so the export of the second timeline of
// a project whose first timeline holds a generated asset carries the flag too (VV-315).
func TestDisclosure_AnExportOfASecondTimelineCountsTheWholeProject(t *testing.T) {
	p := newPubEnv(t)
	_, second := p.addSecondTimeline()
	p.seedAsset(p.project, generatedHash, asset.Generated)
	exp := p.seedExport(second, "job_0000000961", ".mp4", []byte("fake mp4 bytes"))

	args := p.args(exp)
	args.AIDisclosure = false

	check, err := p.pub.PublishValidate(
		p.as(token),
		service.PublishValidateRequest{ProjectID: p.project, PublishArgs: args},
	)
	if err != nil || !strings.Contains(check.Summary, "AI disclosure: set (a generated asset is used)") {
		t.Fatalf("validate = %+v, %v", check, err)
	}
}

// RF-407, acceptance criterion 4 of epic 19: a post whose video uses a generated asset carries the flag
// where the network takes one, whatever the arguments said, and the summary a person approves says so.
func TestDisclosure_AGeneratedAssetSetsTheFlagAndTheSummarySaysSo(t *testing.T) {
	p := newPubEnv(t)
	p.seedAsset(p.project, generatedHash, asset.Generated)
	exp := p.seedExport(p.project, "job_0000000951", ".mp4", []byte("fake mp4 bytes"))

	args := p.args(exp)
	args.AIDisclosure = false

	check, err := p.pub.PublishValidate(
		p.as(token),
		service.PublishValidateRequest{ProjectID: p.project, PublishArgs: args},
	)
	if err != nil || check.State != "prepared" ||
		!strings.Contains(check.Summary, "AI disclosure: set (a generated asset is used)") {
		t.Fatalf("validate = %+v, %v", check, err)
	}

	job, err := p.now(p.as(token), args)
	if err != nil {
		t.Fatal(err)
	}

	req, err := p.svc.ApprovalGet(p.as(token), service.ApprovalRef{ID: job.ApprovalID})
	if err != nil || !strings.Contains(req.Summary, "AI disclosure: set (a generated asset is used)") {
		t.Fatalf("the approval summary = %q, %v", req.Summary, err)
	}

	if len(p.conn.Disclosed()) != 0 {
		t.Fatal("a post left before the approval")
	}

	p.decide(job, "approve")

	if got := p.conn.Disclosed(); len(got) != 1 || !got[0] {
		t.Fatalf("disclosed = %v; the post must carry the flag", got)
	}
}

func TestDisclosure_AnImportedAssetLeavesTheFlagToThePerson(t *testing.T) {
	p := newPubEnv(t)
	p.seedAsset(p.project, generatedHash, asset.Imported)
	exp := p.seedExport(p.project, "job_0000000952", ".mp4", []byte("fake mp4 bytes"))

	check, err := p.pub.PublishValidate(p.as(token), service.PublishValidateRequest{
		ProjectID: p.project, PublishArgs: p.args(exp),
	})
	if err != nil || !strings.Contains(check.Summary, "AI disclosure: not set") {
		t.Fatalf("validate = %+v, %v", check, err)
	}

	said := p.args(exp)
	said.AIDisclosure = true

	check, err = p.pub.PublishValidate(
		p.as(token),
		service.PublishValidateRequest{ProjectID: p.project, PublishArgs: said},
	)
	if err != nil || !strings.HasSuffix(check.Summary, "AI disclosure: set") {
		t.Fatalf("validate with the flag = %+v, %v", check, err)
	}

	job, err := p.now(p.as(token), said)
	if err != nil {
		t.Fatal(err)
	}

	p.decide(job, "approve")

	if got := p.conn.Disclosed(); len(got) != 1 || !got[0] {
		t.Fatalf("disclosed = %v; the flag the person set was lost", got)
	}
}

func TestDisclosure_AnAssetOfAnotherProjectDoesNotSetTheFlag(t *testing.T) {
	p := newPubEnv(t)
	other := p.newProject("other")
	p.seedAsset(other, generatedHash, asset.Generated)
	exp := p.seedExport(p.project, "job_0000000953", ".mp4", []byte("fake mp4 bytes"))

	check, err := p.pub.PublishValidate(p.as(token), service.PublishValidateRequest{
		ProjectID: p.project, PublishArgs: p.args(exp),
	})
	if err != nil || !strings.Contains(check.Summary, "AI disclosure: not set") {
		t.Fatalf("validate = %+v, %v", check, err)
	}
}
