package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// stub answers every Service method with an empty result, or with err.
type stub struct {
	service.Service

	err error
}

func reply[T any](err error) (*T, error) {
	if err != nil {
		return nil, err
	}

	return new(T), nil
}

func (s stub) ProjectList(context.Context, service.ProjectListRequest) (*service.ProjectListResult, error) {
	return reply[service.ProjectListResult](s.err)
}

func (s stub) ProjectCreate(context.Context, service.ProjectCreateRequest) (*service.ProjectCreateResult, error) {
	return reply[service.ProjectCreateResult](s.err)
}

func (s stub) ProjectGet(context.Context, service.ProjectGetRequest) (*service.ProjectGetResult, error) {
	return reply[service.ProjectGetResult](s.err)
}

func (s stub) ProjectTrash(context.Context, service.ProjectTrashRequest) (*service.ProjectStateResult, error) {
	return reply[service.ProjectStateResult](s.err)
}

func (s stub) ProjectRestore(context.Context, service.ProjectTrashRequest) (*service.ProjectStateResult, error) {
	return reply[service.ProjectStateResult](s.err)
}

func (s stub) Edit(context.Context, service.EditRequest) (*service.EditResult, error) {
	return reply[service.EditResult](s.err)
}

func (s stub) CommandHelp(context.Context, service.CommandHelpRequest) (*service.CommandHelpResult, error) {
	return reply[service.CommandHelpResult](s.err)
}

func (s stub) HistoryList(context.Context, service.HistoryListRequest) (*service.HistoryListResult, error) {
	return reply[service.HistoryListResult](s.err)
}

func (s stub) Undo(context.Context, service.UndoRequest) (*service.UndoResult, error) {
	return reply[service.UndoResult](s.err)
}

func (s stub) Redo(context.Context, service.UndoRequest) (*service.UndoResult, error) {
	return reply[service.UndoResult](s.err)
}

func (s stub) Lint(context.Context, service.LintRequest) (*service.LintResult, error) {
	return reply[service.LintResult](s.err)
}

func (s stub) MediaImport(context.Context, service.MediaImportRequest) (*service.MediaImportResult, error) {
	return reply[service.MediaImportResult](s.err)
}

func (s stub) MediaInspect(context.Context, service.MediaInspectRequest) (*service.MediaInspectResult, error) {
	return reply[service.MediaInspectResult](s.err)
}

func (s stub) MediaList(context.Context, service.MediaListRequest) (*service.MediaListResult, error) {
	return reply[service.MediaListResult](s.err)
}

func (s stub) MediaTrash(context.Context, service.MediaTrashRequest) (*service.MediaStateResult, error) {
	return reply[service.MediaStateResult](s.err)
}

func (s stub) MediaRestore(context.Context, service.MediaTrashRequest) (*service.MediaStateResult, error) {
	return reply[service.MediaStateResult](s.err)
}

func final(err error) (*service.Job, error) {
	if err != nil {
		return nil, err
	}

	return &service.Job{State: service.JobSucceeded}, nil
}

func (s stub) JobStart(context.Context, service.JobStartRequest) (*service.Job, error) {
	return final(s.err)
}

func (s stub) JobGet(context.Context, service.JobGetRequest) (*service.Job, error) {
	return final(s.err)
}

func (s stub) JobCancel(context.Context, service.JobCancelRequest) (*service.Job, error) {
	return final(s.err)
}

func (s stub) ExportBatch(context.Context, service.ExportBatchRequest) (*service.Job, error) {
	return final(s.err)
}

func (s stub) ExportCheck(context.Context, service.ExportCheckRequest) (*service.ExportCheckResult, error) {
	return reply[service.ExportCheckResult](s.err)
}

func (s stub) ExportPresetList(
	context.Context,
	service.ExportPresetListRequest,
) (*service.ExportPresetListResult, error) {
	return reply[service.ExportPresetListResult](s.err)
}

func (s stub) JobList(context.Context, service.JobListRequest) (*service.JobListResult, error) {
	return reply[service.JobListResult](s.err)
}

func (s stub) TemplateList(context.Context, service.TemplateListRequest) (*service.TemplateListResult, error) {
	return reply[service.TemplateListResult](s.err)
}

func (s stub) TemplateSave(context.Context, service.TemplateSaveRequest) (*service.TemplateResult, error) {
	return reply[service.TemplateResult](s.err)
}

func (s stub) TemplateInstantiate(
	context.Context, service.TemplateInstantiateRequest,
) (*service.TemplateInstantiateResult, error) {
	return reply[service.TemplateInstantiateResult](s.err)
}

func (s stub) TemplateExport(context.Context, service.TemplateRef) (*service.FileResult, error) {
	return reply[service.FileResult](s.err)
}

func (s stub) TemplateImport(context.Context, service.TemplateImportRequest) (*service.TemplateResult, error) {
	return reply[service.TemplateResult](s.err)
}

func (s stub) ProjectBundleExport(context.Context, service.ProjectBundleExportRequest) (*service.Job, error) {
	return reply[service.Job](s.err)
}

func (s stub) ProjectBundleImport(
	context.Context, service.ProjectBundleImportRequest,
) (*service.ProjectCreateResult, error) {
	return reply[service.ProjectCreateResult](s.err)
}

func (s stub) ViewFrame(context.Context, service.ViewFrameRequest) (*service.ViewResult, error) {
	return reply[service.ViewResult](s.err)
}

func (s stub) ViewSheet(context.Context, service.ViewSheetRequest) (*service.ViewResult, error) {
	return reply[service.ViewResult](s.err)
}

func (s stub) Doctor(context.Context, service.DoctorRequest) (*service.DoctorResult, error) {
	return reply[service.DoctorResult](s.err)
}

func (s stub) FontList(context.Context, service.FontListRequest) (*service.FontListResult, error) {
	return reply[service.FontListResult](s.err)
}

func (s stub) AuditList(context.Context, service.AuditListRequest) (*service.AuditListResult, error) {
	return reply[service.AuditListResult](s.err)
}

func (s stub) ToolsInstall(context.Context, service.ToolsInstallRequest) (*service.ToolsInstallResult, error) {
	return reply[service.ToolsInstallResult](s.err)
}

func (s stub) ProjectOtioExport(
	context.Context, service.ProjectOtioExportRequest,
) (*service.ProjectOtioExportResult, error) {
	return reply[service.ProjectOtioExportResult](s.err)
}

func (s stub) ProjectOtioImport(
	context.Context, service.ProjectOtioImportRequest,
) (*service.ProjectOtioImportResult, error) {
	return reply[service.ProjectOtioImportResult](s.err)
}

func (s stub) ToolsStatus(context.Context, service.ToolsStatusRequest) (*service.ToolsStatusResult, error) {
	return reply[service.ToolsStatusResult](s.err)
}

// TestCLIJSONPurity is SDD-003 section 9 check 13: with --json standard output
// is exactly one JSON document and standard error is empty, for successes,
// errors and flag errors, over at least 100 invocations of every command.
func TestCLIJSONPurity(t *testing.T) {
	t.Parallel()

	const project = "--project=prj_7k2m9x4q1b"

	variants := [][]string{
		{},
		{"--nope"},
		{"x"},
		{project},
		{"x", project},
		{project, "--name", "n"},
		{"--limit", "bad"},
		{"--actor", "user:x"},
		{"--help-not-a-flag=1"},
	}

	answers := []error{nil, &service.Error{Code: command.CodeNotFound, Message: "no such thing."}}

	runs := 0

	for _, s := range registry() {
		if s.name == "mcp" || s.name == "events" || s.pending != "" {
			continue
		}

		for _, answer := range answers {
			for _, v := range variants {
				h := newHarness(t)
				h.res.svc = stub{err: answer}

				args := append(append([]string{}, v...), "--json")
				code := h.run(append([]string{s.name}, args...)...)
				runs++

				label := s.name + " " + strings.Join(args, " ")

				if h.stderr.Len() != 0 {
					t.Errorf("%s: standard error is not empty: %q", label, h.stderr.String())
				}

				out := h.stdout.Bytes()
				if bytes.Count(out, []byte("\n")) != 1 || !json.Valid(out) {
					t.Errorf("%s: standard output is not one JSON document: %q", label, out)
				}

				if code < 0 || code > 2 {
					t.Errorf("%s: exit %d", label, code)
				}
			}
		}
	}

	if runs < 100 {
		t.Errorf("only %d invocations, want at least 100", runs)
	}
}

func (s stub) FontImport(context.Context, service.FontImportRequest) (*service.FontImportResult, error) {
	return reply[service.FontImportResult](s.err)
}

func (s stub) BrandKitGet(context.Context, service.BrandKitGetRequest) (*service.BrandKit, error) {
	return reply[service.BrandKit](s.err)
}

func (s stub) BrandKitSet(context.Context, service.BrandKitSetRequest) (*service.BrandKit, error) {
	return reply[service.BrandKit](s.err)
}

func (s stub) ProjectDescribe(context.Context, service.ProjectDescribeRequest) (*service.ProjectDescribeResult, error) {
	return reply[service.ProjectDescribeResult](s.err)
}

func (s stub) ProjectDuplicate(context.Context, service.ProjectDuplicateRequest) (*service.ProjectCreateResult, error) {
	return reply[service.ProjectCreateResult](s.err)
}

func (s stub) ProjectItemAdd(context.Context, service.ProjectItemAddRequest) (*service.ProjectItemResult, error) {
	return reply[service.ProjectItemResult](s.err)
}

func (s stub) ProjectItemMove(context.Context, service.ProjectItemMoveRequest) (*service.ProjectItemResult, error) {
	return reply[service.ProjectItemResult](s.err)
}

func (s stub) ProjectItemRename(context.Context, service.ProjectItemRenameRequest) (*service.ProjectItemResult, error) {
	return reply[service.ProjectItemResult](s.err)
}

func (s stub) ProjectItemTrash(context.Context, service.ProjectItemTrashRequest) (*service.ProjectItemResult, error) {
	return reply[service.ProjectItemResult](s.err)
}

func (s stub) ProjectItemRestore(context.Context, service.ProjectItemTrashRequest) (*service.ProjectItemResult, error) {
	return reply[service.ProjectItemResult](s.err)
}

func (s stub) BriefGet(context.Context, service.BriefGetRequest) (*service.BriefResult, error) {
	return reply[service.BriefResult](s.err)
}

func (s stub) BriefSet(context.Context, service.BriefSetRequest) (*service.BriefResult, error) {
	return reply[service.BriefResult](s.err)
}

func (s stub) AssetList(context.Context, service.AssetListRequest) (*service.AssetListResult, error) {
	return reply[service.AssetListResult](s.err)
}

func (s stub) AssetGet(context.Context, service.AssetRef) (*service.AssetResult, error) {
	return reply[service.AssetResult](s.err)
}

func (s stub) AssetUses(context.Context, service.AssetUsesRequest) (*service.AssetUsesResult, error) {
	return reply[service.AssetUsesResult](s.err)
}

func (s stub) AssetSetInfo(context.Context, service.AssetSetInfoRequest) (*service.AssetResult, error) {
	return reply[service.AssetResult](s.err)
}

func (s stub) AssetTag(context.Context, service.AssetTagRequest) (*service.AssetResult, error) {
	return reply[service.AssetResult](s.err)
}

func (s stub) BriefingQuestions(
	context.Context,
	service.BriefingQuestionsRequest,
) (*service.BriefingQuestionsResult, error) {
	return reply[service.BriefingQuestionsResult](s.err)
}

func (s stub) BriefingAnswer(context.Context, service.BriefingAnswerRequest) (*service.BriefResult, error) {
	return reply[service.BriefResult](s.err)
}

func (s stub) MotionBrief(context.Context, service.MotionBriefRequest) (*service.MotionBriefResult, error) {
	return reply[service.MotionBriefResult](s.err)
}

func (s stub) MemoryList(context.Context, service.MemoryListRequest) (*service.LibraryListResult, error) {
	return reply[service.LibraryListResult](s.err)
}

func (s stub) MemoryGet(context.Context, service.LibraryRef) (*service.MemoryGetResult, error) {
	return reply[service.MemoryGetResult](s.err)
}

func (s stub) MemoryWrite(context.Context, service.MemoryWriteRequest) (*service.MemoryWriteResult, error) {
	return reply[service.MemoryWriteResult](s.err)
}

func (s stub) MemoryRemove(context.Context, service.LibraryRef) (*service.LibraryRemoveResult, error) {
	return reply[service.LibraryRemoveResult](s.err)
}

func (s stub) SkillList(context.Context, service.SkillListRequest) (*service.LibraryListResult, error) {
	return reply[service.LibraryListResult](s.err)
}

func (s stub) SkillGet(context.Context, service.LibraryRef) (*service.SkillGetResult, error) {
	return reply[service.SkillGetResult](s.err)
}

func (s stub) SkillWrite(context.Context, service.SkillWriteRequest) (*service.SkillWriteResult, error) {
	return reply[service.SkillWriteResult](s.err)
}

func (s stub) SkillImport(context.Context, service.SkillImportRequest) (*service.SkillImportResult, error) {
	return reply[service.SkillImportResult](s.err)
}

func (s stub) SkillMarkReviewed(context.Context, service.SkillReviewRequest) (*service.SkillReviewResult, error) {
	return reply[service.SkillReviewResult](s.err)
}

func (s stub) SkillLint(context.Context, service.LibraryRef) (*service.SkillLintResult, error) {
	return reply[service.SkillLintResult](s.err)
}

func (s stub) SkillExport(context.Context, service.SkillExportRequest) (*service.SkillExportResult, error) {
	return reply[service.SkillExportResult](s.err)
}

func (s stub) PromptList(context.Context, service.PromptListRequest) (*service.LibraryListResult, error) {
	return reply[service.LibraryListResult](s.err)
}

func (s stub) PromptGet(context.Context, service.LibraryRef) (*service.PromptGetResult, error) {
	return reply[service.PromptGetResult](s.err)
}

func (s stub) PromptWrite(context.Context, service.PromptWriteRequest) (*service.PromptWriteResult, error) {
	return reply[service.PromptWriteResult](s.err)
}

func (s stub) PromptRender(context.Context, service.PromptRenderRequest) (*service.PromptRenderResult, error) {
	return reply[service.PromptRenderResult](s.err)
}

func (s stub) LibrarySearch(context.Context, service.LibrarySearchRequest) (*service.LibrarySearchResult, error) {
	return reply[service.LibrarySearchResult](s.err)
}

func (s stub) LibraryReindex(context.Context, service.LibraryReindexRequest) (*service.LibraryReindexResult, error) {
	return reply[service.LibraryReindexResult](s.err)
}
