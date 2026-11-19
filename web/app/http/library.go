package httpapi

import (
	"net/http"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

// The routes of the library (SDD-018 section 14): memory notes, skills and prompts. A handler parses its
// input, calls one Service method and formats the result; the rules are in core/library.

type libraryNameIn struct {
	Name      string `json:"name"                 validate:"required"`
	AgentView bool   `json:"agent_view,omitempty"                     doc:"Set by an agent: only what is reviewed or kept is returned."`
}

type skillReviewIn struct {
	Name        string `json:"name"         validate:"required"`
	ContentHash string `json:"content_hash" validate:"required" doc:"The content_hash of the skill as it was read (GET /library/skills/{name}); a skill that changed since is not marked."`
}

type memoryListIn struct {
	Scope             string `json:"scope,omitempty"              doc:"workspace or a project id."`
	Tags              string `json:"tags,omitempty"               doc:"Comma separated; every tag must be on the note."`
	IncludeUnreviewed bool   `json:"include_unreviewed,omitempty" doc:"Also the notes no person has kept yet."`
	AgentView         bool   `json:"agent_view,omitempty"`
}

type memoryWriteIn struct {
	Name        string   `json:"name,omitempty"  doc:"Empty: made from the description."`
	Description string   `json:"description"     doc:"One line, at most 200 characters."                                    validate:"required"`
	Scope       string   `json:"scope,omitempty" doc:"workspace (default) or a project id."`
	Tags        []string `json:"tags,omitempty"`
	Body        string   `json:"body,omitempty"  doc:"At most 8 KiB."`
	Keep        *bool    `json:"keep,omitempty"  doc:"A person may keep or release a note; a note a person writes is kept."`
}

type skillListIn struct {
	Origin            string `json:"origin,omitempty"             doc:"built-in, user or imported."`
	IncludeUnreviewed bool   `json:"include_unreviewed,omitempty" doc:"Also the skills no person has reviewed."`
	AgentView         bool   `json:"agent_view,omitempty"`
}

type skillWriteIn struct {
	Name          string                   `json:"name"                    validate:"required"`
	Description   string                   `json:"description"             validate:"required" doc:"At most 1024 characters."`
	License       string                   `json:"license,omitempty"`
	Compatibility string                   `json:"compatibility,omitempty"`
	AllowedTools  string                   `json:"allowed_tools,omitempty"`
	Metadata      map[string]string        `json:"metadata,omitempty"                          doc:"Keys that start with stuarty- are the product's and are dropped."`
	Body          string                   `json:"body"                    validate:"required"`
	Files         []service.SkillFileInput `json:"files"                                       doc:"The other files of the folder; null keeps those it has, [] removes them."`
}

type skillImportIn struct {
	Path    string `json:"path"              validate:"required" doc:"An absolute path on the machine of the server: a folder, a .zip or a .tar.gz."`
	Source  string `json:"source,omitempty"                      doc:"Where the skill came from; the base name of the path when empty."`
	Replace bool   `json:"replace,omitempty"`
}

type skillExportIn struct {
	Names []string `json:"names,omitempty" doc:"Empty: every reviewed skill."`
}

type promptListIn struct {
	Tags      string `json:"tags,omitempty"`
	AgentView bool   `json:"agent_view,omitempty"`
}

type promptWriteIn struct {
	Name        string            `json:"name"               validate:"required"`
	Description string            `json:"description"        validate:"required"`
	Tags        []string          `json:"tags,omitempty"`
	Defaults    map[string]string `json:"defaults,omitempty"                     doc:"Variable to its default value."`
	Body        string            `json:"body"               validate:"required" doc:"Text with {{variable}} placeholders; at most 32 KiB."`
}

type promptRenderIn struct {
	Name      string            `json:"name"                 validate:"required"`
	Values    map[string]string `json:"values,omitempty"                         doc:"Variable to its value."`
	AgentView bool              `json:"agent_view,omitempty"`
}

type librarySearchIn struct {
	Query             string `json:"query"                        validate:"required" doc:"Words; every one must start a word of the entry."`
	Kinds             string `json:"kinds,omitempty"                                  doc:"Comma separated: memory, skill, prompt."`
	Scope             string `json:"scope,omitempty"`
	Tags              string `json:"tags,omitempty"                                   doc:"Comma separated."`
	Origin            string `json:"origin,omitempty"`
	IncludeUnreviewed bool   `json:"include_unreviewed,omitempty"`
	Limit             int    `json:"limit,omitempty"                                  doc:"At most 50; default 20."`
	Budget            int    `json:"budget,omitempty"                                 doc:"Characters of the answer; at most 16000, default 4000."`
	AgentView         bool   `json:"agent_view,omitempty"`
}

// commaList splits a query value such as "a,b" into its items.
func commaList(s string) []string {
	var out []string

	for part := range strings.SplitSeq(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}

	return out
}

func (h *apiHandler) memoryList(c *web.Ctx, in memoryListIn) (service.LibraryListResult, error) {
	res, err := h.Service.MemoryList(c.Context(), service.MemoryListRequest{
		Scope: in.Scope, Tags: commaList(in.Tags), IncludeUnreviewed: in.IncludeUnreviewed, AgentView: in.AgentView,
	})

	return deref(res), err
}

func (h *apiHandler) memoryGet(c *web.Ctx, in libraryNameIn) (service.MemoryGetResult, error) {
	res, err := h.Service.MemoryGet(c.Context(), service.LibraryRef{Name: in.Name, AgentView: in.AgentView})

	return deref(res), err
}

func (h *apiHandler) memoryWrite(c *web.Ctx, in memoryWriteIn) (service.MemoryWriteResult, error) {
	res, err := h.Service.MemoryWrite(c.Context(), service.MemoryWriteRequest{
		Name: in.Name, Description: in.Description, Scope: in.Scope, Tags: in.Tags, Body: in.Body, Keep: in.Keep,
	})

	return deref(res), err
}

func (h *apiHandler) memoryRemove(c *web.Ctx, in libraryNameIn) (service.LibraryRemoveResult, error) {
	res, err := h.Service.MemoryRemove(c.Context(), service.LibraryRef{Name: in.Name})

	return deref(res), err
}

func (h *apiHandler) skillList(c *web.Ctx, in skillListIn) (service.LibraryListResult, error) {
	res, err := h.Service.SkillList(c.Context(), service.SkillListRequest{
		Origin: in.Origin, IncludeUnreviewed: in.IncludeUnreviewed, AgentView: in.AgentView,
	})

	return deref(res), err
}

func (h *apiHandler) skillGet(c *web.Ctx, in libraryNameIn) (service.SkillGetResult, error) {
	res, err := h.Service.SkillGet(c.Context(), service.LibraryRef{Name: in.Name, AgentView: in.AgentView})

	return deref(res), err
}

func (h *apiHandler) skillWrite(c *web.Ctx, in skillWriteIn) (service.SkillWriteResult, error) {
	res, err := h.Service.SkillWrite(c.Context(), service.SkillWriteRequest{
		Name: in.Name, Description: in.Description, License: in.License, Compatibility: in.Compatibility,
		AllowedTools: in.AllowedTools, Metadata: in.Metadata, Body: in.Body, Files: in.Files,
	})

	return deref(res), err
}

func (h *apiHandler) skillImport(c *web.Ctx, in skillImportIn) (service.SkillImportResult, error) {
	res, err := h.Service.SkillImport(c.Context(), service.SkillImportRequest{
		Path: in.Path, Source: in.Source, Replace: in.Replace,
	})
	if err != nil {
		return service.SkillImportResult{}, err
	}

	c.SetHeader("Location", BasePath+"/library/skills/"+res.Entry.Name)

	return service.SkillImportResult{}, c.JSON(http.StatusCreated, res)
}

func (h *apiHandler) skillReview(c *web.Ctx, in skillReviewIn) (service.SkillReviewResult, error) {
	res, err := h.Service.SkillMarkReviewed(c.Context(), service.SkillReviewRequest{
		Name: in.Name, ContentHash: in.ContentHash,
	})

	return deref(res), err
}

func (h *apiHandler) skillLint(c *web.Ctx, in libraryNameIn) (service.SkillLintResult, error) {
	res, err := h.Service.SkillLint(c.Context(), service.LibraryRef{Name: in.Name})

	return deref(res), err
}

func (h *apiHandler) skillExport(c *web.Ctx, in skillExportIn) (service.SkillExportResult, error) {
	res, err := h.Service.SkillExport(c.Context(), service.SkillExportRequest{Names: in.Names})

	return deref(res), err
}

func (h *apiHandler) promptList(c *web.Ctx, in promptListIn) (service.LibraryListResult, error) {
	res, err := h.Service.PromptList(
		c.Context(),
		service.PromptListRequest{Tags: commaList(in.Tags), AgentView: in.AgentView},
	)

	return deref(res), err
}

func (h *apiHandler) promptGet(c *web.Ctx, in libraryNameIn) (service.PromptGetResult, error) {
	res, err := h.Service.PromptGet(c.Context(), service.LibraryRef{Name: in.Name, AgentView: in.AgentView})

	return deref(res), err
}

func (h *apiHandler) promptWrite(c *web.Ctx, in promptWriteIn) (service.PromptWriteResult, error) {
	res, err := h.Service.PromptWrite(c.Context(), service.PromptWriteRequest{
		Name: in.Name, Description: in.Description, Tags: in.Tags, Defaults: in.Defaults, Body: in.Body,
	})

	return deref(res), err
}

func (h *apiHandler) promptRender(c *web.Ctx, in promptRenderIn) (service.PromptRenderResult, error) {
	res, err := h.Service.PromptRender(c.Context(), service.PromptRenderRequest{
		Name: in.Name, Values: in.Values, AgentView: in.AgentView,
	})

	return deref(res), err
}

func (h *apiHandler) librarySearch(c *web.Ctx, in librarySearchIn) (service.LibrarySearchResult, error) {
	res, err := h.Service.LibrarySearch(c.Context(), service.LibrarySearchRequest{
		Query: in.Query, Kinds: commaList(in.Kinds), Scope: in.Scope, Tags: commaList(in.Tags), Origin: in.Origin,
		IncludeUnreviewed: in.IncludeUnreviewed, Limit: in.Limit, Budget: in.Budget, AgentView: in.AgentView,
	})

	return deref(res), err
}

func (h *apiHandler) libraryReindex(c *web.Ctx, _ noIn) (service.LibraryReindexResult, error) {
	res, err := h.Service.LibraryReindex(c.Context(), service.LibraryReindexRequest{})

	return deref(res), err
}
