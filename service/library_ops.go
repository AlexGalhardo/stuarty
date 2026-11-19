package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/command"
	core "github.com/AlexGalhardo/stuarty/core/library"
	"github.com/AlexGalhardo/stuarty/service/library"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// The budgets of the library text (characters), in the style of SDD-007 section 8.3.
const (
	libDescriptionBudget = 1024
	libBodyBudget        = 64 << 10
	libTextBudget        = 6000 // the body inside the text of a result
	libFileBudget        = 16 << 10
	libMaxFilesShown     = 20
)

// libraryManager is the library of the workspace; a service with no workspace has none.
func (s *svc) libraryManager() (*library.Manager, error) {
	if s.lib == nil {
		return nil, newError(command.CodeNotFound, "this server keeps no library: it has no workspace.")
	}

	return s.lib, nil
}

func libCaller(c Caller) library.Caller {
	return library.Caller{Actor: c.Actor, Person: c.Browser, ProjectID: c.ProjectID}
}

// uiActor is the actor of a browser session of local mode, privileged or not: web/app/auth reserves the
// label "ui" for it, so no token caller can claim it.
const uiActor = "local:ui"

// libText is a text of the library as the caller may have it: the pages edit it, so a session of the UI gets
// it as it is (the page escapes it when it draws it, and a page that got it cleaned would write the cleaned
// text back over the file on its next save); every other caller gets it cleaned and with the less-than sign
// escaped, as data (SDD-018 section 13).
func libText(c Caller, text string, budget int) string {
	if c.Browser || c.Actor == uiActor {
		return text
	}

	return untrusted.Value(text, budget)
}

// libError turns an error of the library into a service error.
func libError(err error, what string) error {
	if err == nil {
		return nil
	}

	var (
		ve *library.ValidationError
		le *library.LimitError
	)

	switch {
	case errors.Is(err, library.ErrNotFound):
		return newError(
			command.CodeNotFound,
			"there is no %s with that name.",
			what,
		).withHint("list them to see the names")
	case errors.Is(err, library.ErrExists):
		return newError("E_ID_EXISTS", "%s", err.Error()).withHint("choose another name, or write over the entry")
	case errors.Is(err, library.ErrForbidden):
		return newError(CodeForbidden, "%s", strings.TrimPrefix(err.Error(), "library: not allowed for this caller: "))
	case errors.As(err, &le):
		return newError("E_LIMIT_EXCEEDED", "%s.", le.Msg).withDetail("limit", "library")
	case errors.As(err, &ve):
		e := newError(command.CodeValidation, "%s.", ve.Error())
		if ve.Field != "" {
			e = e.withPath(ve.Field)
		}

		for k, v := range ve.Details {
			e = e.withDetail(k, v)
		}

		if len(ve.Findings) > 0 {
			e = e.withDetail("findings", libFindings(ve.Findings))
		}

		return e
	}

	return asError(err)
}

func libFindings(fs []core.Finding) []LibraryFinding {
	out := make([]LibraryFinding, len(fs))
	for i, f := range fs {
		out[i] = LibraryFinding{Code: f.Code, Severity: f.Severity, Path: f.Path, Line: f.Line, Message: f.Message}
	}

	return out
}

func toLibraryEntry(c Caller, e *library.Entry) LibraryEntry {
	out := LibraryEntry{
		Kind: string(e.Kind), Name: e.Name, Description: libText(c, e.Description, libDescriptionBudget),
		Scope: e.Scope, Origin: e.Origin, Reviewed: e.Reviewed, Kept: e.Kept, Actor: e.Actor,
		State: e.State, Error: libText(c, e.Error, libDescriptionBudget), LintErrors: e.LintErrors,
		LintWarns: e.LintWarnings, HasScripts: e.HasScripts, UpdatedAt: time.UnixMilli(e.UpdatedMs).UTC(),
	}

	for _, t := range e.Tags {
		out.Tags = append(out.Tags, libText(c, t, 64))
	}

	switch {
	case e.State == library.StateInvalid:
		out.Warning = "The file cannot be read as an entry: see the error."
	case e.Kind == core.KindSkill && !e.Reviewed && e.Origin == core.OriginImported:
		out.Warning = "Imported and not reviewed: an agent is not offered it until a person has read it and marked it as read."
	case e.Kind == core.KindSkill && !e.Reviewed:
		out.Warning = "Not reviewed: an agent is not offered it until a person has read it and marked it as read."
	case e.Kind == core.KindMemory && !e.Kept:
		out.Warning = "Pending: written by an agent; it is not offered to an agent until a person keeps it."
	}

	if e.Kind == core.KindSkill && e.HasScripts {
		out.Warning += " It holds a script, stored as a file and never run."
		out.Warning = strings.TrimSpace(out.Warning)
	}

	return out
}

// libraryLine is the one line of an entry in a list: the names are the product's own, the description is a block.
func libraryLine(e *LibraryEntry) string {
	flags := ""

	switch {
	case e.State == library.StateInvalid:
		flags = " [invalid]"
	case e.Kind == string(core.KindSkill) && !e.Reviewed:
		flags = " [not reviewed]"
	case e.Kind == string(core.KindMemory) && !e.Kept:
		flags = " [pending]"
	}

	return fmt.Sprintf("%s/%s%s", e.Kind, e.Name, flags)
}

// listText writes a list: a line for each entry, and the descriptions as one untrusted block. rows are the
// entries as the files have them, so that the block cleans and escapes the text once.
func listText(kind string, rows []library.Entry, entries []LibraryEntry) string {
	if len(entries) == 0 {
		return "no " + kind + " entries"
	}

	var lines, descs []string

	for i := range entries {
		lines = append(lines, libraryLine(&entries[i]))
		descs = append(descs, entries[i].Name+": "+strings.Join(strings.Fields(rows[i].Description), " "))
	}

	return fmt.Sprintf("%d %s entries\n%s\n%s", len(entries), kind, strings.Join(lines, "\n"),
		untrusted.Block(untrusted.SourceLibraryText, untrusted.Attrs{"kind": kind, "field": "description"},
			strings.Join(descs, "\n"), libTextBudget))
}

// libraryPrep is what every library operation starts with: the caller with the scope, and the manager.
func (s *svc) libraryPrep(ctx context.Context, method string) (Caller, *library.Manager, error) {
	c, err := authorize(ctx, method, "")
	if err != nil {
		return Caller{}, nil, err
	}

	m, err := s.libraryManager()
	if err != nil {
		return Caller{}, nil, err
	}

	return c, m, nil
}

func (s *svc) libraryAudit(ctx context.Context, c Caller, action, subject string) error {
	return asError(s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		return appendAudit(ctx, tx, s.nowMs(), c.Actor, action, "", subject)
	}))
}

func (s *svc) listLibrary(
	ctx context.Context, method string, kind core.Kind, f library.Filter, agent bool,
) (*LibraryListResult, error) {
	c, m, err := s.libraryPrep(ctx, method)
	if err != nil {
		return nil, err
	}

	f.ProjectID = c.ProjectID
	f.IncludeUnreviewed = f.IncludeUnreviewed && !agent

	rows, lerr := m.List(ctx, kind, f)
	if lerr != nil {
		return nil, libError(lerr, string(kind))
	}

	res := &LibraryListResult{Kind: string(kind), Entries: make([]LibraryEntry, 0, len(rows))}
	res.UntrustedNotice = UntrustedNotice

	for i := range rows {
		res.Entries = append(res.Entries, toLibraryEntry(c, &rows[i]))
	}

	res.Text = listText(string(kind), rows, res.Entries)

	return res, nil
}

// MemoryList lists the notes the caller may see.
func (s *svc) MemoryList(ctx context.Context, req MemoryListRequest) (*LibraryListResult, error) {
	return s.listLibrary(ctx, "MemoryList", core.KindMemory, library.Filter{
		Scope: req.Scope, Tags: req.Tags, IncludeUnreviewed: req.IncludeUnreviewed,
	}, req.AgentView)
}

// SkillList lists the skills the caller may see: the reviewed ones unless it asks for the others.
func (s *svc) SkillList(ctx context.Context, req SkillListRequest) (*LibraryListResult, error) {
	return s.listLibrary(ctx, "SkillList", core.KindSkill, library.Filter{
		Origin: req.Origin, IncludeUnreviewed: req.IncludeUnreviewed,
	}, req.AgentView)
}

// PromptList lists the prompts.
func (s *svc) PromptList(ctx context.Context, req PromptListRequest) (*LibraryListResult, error) {
	return s.listLibrary(ctx, "PromptList", core.KindPrompt, library.Filter{Tags: req.Tags}, req.AgentView)
}

// block wraps the text of an entry as untrusted text for the text part of a result.
func block(kind, name, text string) string {
	return untrusted.Block(
		untrusted.SourceLibraryText,
		untrusted.Attrs{"kind": kind, "name": name},
		text,
		libTextBudget,
	)
}

// MemoryGet reads a note.
func (s *svc) MemoryGet(ctx context.Context, req LibraryRef) (*MemoryGetResult, error) {
	c, m, err := s.libraryPrep(ctx, "MemoryGet")
	if err != nil {
		return nil, err
	}

	mem, entry, gerr := m.ReadMemory(ctx, req.Name)
	if gerr != nil {
		return nil, libError(gerr, "note")
	}

	if c.ProjectID != "" && mem.Scope != "workspace" && mem.Scope != c.ProjectID {
		return nil, libError(library.ErrNotFound, "note")
	}

	if req.AgentView && !entry.Kept {
		return nil, newError(CodeForbidden, "this note is pending: a person has not kept it.").
			withHint("ask the person to keep it in the Memory section")
	}

	res := &MemoryGetResult{Entry: toLibraryEntry(c, &entry), Body: libText(c, mem.Body, libBodyBudget)}
	res.UntrustedNotice = UntrustedNotice
	res.Text = "memory/" + req.Name + " (" + mem.Scope + ")\n" + block(
		"memory",
		req.Name,
		mem.Description+"\n\n"+mem.Body,
	)

	return res, nil
}

// MemoryWrite creates or replaces a note.
func (s *svc) MemoryWrite(ctx context.Context, req MemoryWriteRequest) (*MemoryWriteResult, error) {
	c, m, err := s.libraryPrep(ctx, "MemoryWrite")
	if err != nil {
		return nil, err
	}

	_, entry, werr := m.SaveMemory(ctx, libCaller(c), library.MemoryInput{
		Name: req.Name, Description: req.Description, Scope: req.Scope, Tags: req.Tags, Body: req.Body, Keep: req.Keep,
	})
	if werr != nil {
		return nil, libError(werr, "note")
	}

	if err := s.libraryAudit(ctx, c, "library.memory.write", entry.Key); err != nil {
		return nil, err
	}

	res := &MemoryWriteResult{Entry: toLibraryEntry(c, &entry)}
	res.UntrustedNotice = UntrustedNotice
	res.Text = "memory/" + entry.Name + " written"

	if !entry.Kept {
		res.Text += "; it is pending until a person keeps it"
	}

	return res, nil
}

// MemoryRemove deletes a note.
func (s *svc) MemoryRemove(ctx context.Context, req LibraryRef) (*LibraryRemoveResult, error) {
	c, m, err := s.libraryPrep(ctx, "MemoryRemove")
	if err != nil {
		return nil, err
	}

	if rerr := m.DeleteMemory(ctx, libCaller(c), req.Name); rerr != nil {
		return nil, libError(rerr, "note")
	}

	if err := s.libraryAudit(ctx, c, "library.memory.remove", core.Key(core.KindMemory, req.Name)); err != nil {
		return nil, err
	}

	return &LibraryRemoveResult{Kind: "memory", Name: req.Name, Text: "memory/" + req.Name + " removed"}, nil
}

// PromptGet reads a prompt.
func (s *svc) PromptGet(ctx context.Context, req LibraryRef) (*PromptGetResult, error) {
	c, m, err := s.libraryPrep(ctx, "PromptGet")
	if err != nil {
		return nil, err
	}

	p, entry, gerr := m.ReadPrompt(ctx, req.Name)
	if gerr != nil {
		return nil, libError(gerr, "prompt")
	}

	res := &PromptGetResult{
		Entry: toLibraryEntry(c, &entry), Body: libText(c, p.Body, libBodyBudget), Variables: core.Variables(p.Body),
	}

	if len(p.Defaults) > 0 {
		res.Defaults = map[string]string{}
		for k, v := range p.Defaults {
			res.Defaults[k] = libText(c, v, core.MaxValueLength)
		}
	}

	res.UntrustedNotice = UntrustedNotice
	res.Text = "prompt/" + req.Name + " variables: " + strings.Join(res.Variables, ", ") + "\n" +
		block("prompt", req.Name, p.Description+"\n\n"+p.Body)

	return res, nil
}

// PromptWrite creates or replaces a prompt.
func (s *svc) PromptWrite(ctx context.Context, req PromptWriteRequest) (*PromptWriteResult, error) {
	c, m, err := s.libraryPrep(ctx, "PromptWrite")
	if err != nil {
		return nil, err
	}

	p, entry, werr := m.SavePrompt(ctx, libCaller(c), library.PromptInput{
		Name: req.Name, Description: req.Description, Tags: req.Tags, Defaults: req.Defaults, Body: req.Body,
	})
	if werr != nil {
		return nil, libError(werr, "prompt")
	}

	if err := s.libraryAudit(ctx, c, "library.prompt.write", entry.Key); err != nil {
		return nil, err
	}

	res := &PromptWriteResult{Entry: toLibraryEntry(c, &entry), Variables: core.Variables(p.Body)}
	res.UntrustedNotice = UntrustedNotice
	res.Text = "prompt/" + entry.Name + " written; variables: " + strings.Join(res.Variables, ", ")

	return res, nil
}

// PromptRender replaces the variables of a prompt and returns the text. Nothing is sent anywhere.
func (s *svc) PromptRender(ctx context.Context, req PromptRenderRequest) (*PromptRenderResult, error) {
	c, m, err := s.libraryPrep(ctx, "PromptRender")
	if err != nil {
		return nil, err
	}

	out, _, rerr := m.Render(ctx, req.Name, req.Values)
	if rerr != nil {
		return nil, libError(rerr, "prompt")
	}

	res := &PromptRenderResult{Name: req.Name, Rendered: libText(c, out, libBodyBudget)}
	res.UntrustedNotice = UntrustedNotice
	res.Text = block("prompt", req.Name, out)

	return res, nil
}

// SkillGet reads a skill: its text and its files. A person at the browser may read a skill that is not reviewed
// (that is how it gets reviewed); an agent surface may not.
func (s *svc) SkillGet(ctx context.Context, req LibraryRef) (*SkillGetResult, error) {
	c, m, err := s.libraryPrep(ctx, "SkillGet")
	if err != nil {
		return nil, err
	}

	skill, entry, gerr := m.ReadSkill(ctx, req.Name)
	if gerr != nil {
		return nil, libError(gerr, "skill")
	}

	if req.AgentView && !entry.Reviewed {
		return nil, newError(
			CodeForbidden,
			"this skill is not reviewed: a person has not read it and marked it as read.",
		).
			withHint("ask the person to review it in the Skills section")
	}

	all, _ := skill.AllFiles()
	findings := libFindings(core.LintSkill(req.Name, all))

	res := &SkillGetResult{
		Entry: toLibraryEntry(
			c,
			&entry,
		),
		License:       libText(c, skill.License, 200),
		Compatibility: libText(c, skill.Compatibility, core.MaxCompatibility),
		AllowedTools:  libText(c, skill.AllowedTools, 200),
		Source:        libText(c, skill.Source(), 200),
		Body:          libText(c, skill.Body, libBodyBudget),
		Findings:      findings,
		ContentHash:   skill.ContentHash(),
	}

	for i := range skill.Files {
		f := &skill.Files[i]
		view := SkillFileView{Path: f.Path, Bytes: len(f.Data)}

		for _, fd := range findings {
			if fd.Code == core.LintScript && fd.Path == f.Path {
				view.Script = true
			}
		}

		if utf8.Valid(f.Data) && len(f.Data) <= libFileBudget*4 {
			view.Text = libText(c, string(f.Data), libFileBudget)
		} else {
			view.Binary = true
		}

		res.Files = append(res.Files, view)
	}

	res.UntrustedNotice = UntrustedNotice
	res.Text = skillText(&res.Entry, skill, res.Files, findings)

	return res, nil
}

func skillText(e *LibraryEntry, skill core.Skill, files []SkillFileView, findings []LibraryFinding) string {
	var b strings.Builder

	fmt.Fprintf(&b, "skill/%s origin: %s, reviewed: %t", e.Name, e.Origin, e.Reviewed)

	if e.Warning != "" {
		b.WriteString("\nwarning: " + e.Warning)
	}

	// The paths of the files came with the skill (an archive names them), so they are library text too.
	var listing strings.Builder

	for i := range files {
		if i >= libMaxFilesShown {
			fmt.Fprintf(&listing, "... and %d more files\n", len(files)-i)

			break
		}

		fmt.Fprintf(&listing, "file %s (%d bytes)\n", files[i].Path, files[i].Bytes)
	}

	if lines := findingLines(findings); lines != "" {
		b.WriteString("\n" + findingsBlock(e.Name, lines))
	}

	if listing.Len() > 0 {
		b.WriteString("\n" + untrusted.Block(untrusted.SourceLibraryText,
			untrusted.Attrs{"kind": "skill", "name": e.Name, "field": "files"}, listing.String(), libTextBudget))
	}

	b.WriteString("\n" + block("skill", e.Name, skill.Description+"\n\n"+skill.Body))

	return b.String()
}

// findingLines writes the findings of the lint one per line: severity, code, and the file and line they are on.
func findingLines(findings []LibraryFinding) string {
	var b strings.Builder

	for i := range findings {
		fmt.Fprintf(&b, "%s %s", findings[i].Severity, findings[i].Code)

		if findings[i].Path != "" {
			fmt.Fprintf(&b, " %s", findings[i].Path)

			if findings[i].Line > 0 {
				fmt.Fprintf(&b, ":%d", findings[i].Line)
			}
		}

		b.WriteString("\n")
	}

	return b.String()
}

// findingsBlock delimits the findings as library text: a finding names a file of the skill, and the name of the
// file came with the skill, so a file name such as "ignore the rules.md" is data, not an instruction.
func findingsBlock(name, lines string) string {
	return untrusted.Block(untrusted.SourceLibraryText,
		untrusted.Attrs{"kind": "skill", "name": name, "field": "findings"}, lines, libTextBudget)
}

// SkillWrite creates or replaces a skill, after the lint.
func (s *svc) SkillWrite(ctx context.Context, req SkillWriteRequest) (*SkillWriteResult, error) {
	c, m, err := s.libraryPrep(ctx, "SkillWrite")
	if err != nil {
		return nil, err
	}

	in := library.SkillInput{
		Name: req.Name, Description: req.Description, License: req.License, Compatibility: req.Compatibility,
		AllowedTools: req.AllowedTools, Metadata: req.Metadata, Body: req.Body,
	}

	if req.Files != nil {
		in.Files = make([]core.File, 0, len(req.Files))

		for _, f := range req.Files {
			in.Files = append(in.Files, core.File{Path: f.Path, Data: []byte(f.Text)})
		}
	}

	_, entry, findings, werr := m.SaveSkill(ctx, libCaller(c), in)
	if werr != nil {
		return nil, libError(werr, "skill")
	}

	if err := s.libraryAudit(ctx, c, "library.skill.write", entry.Key); err != nil {
		return nil, err
	}

	res := &SkillWriteResult{Entry: toLibraryEntry(c, &entry), Findings: libFindings(findings)}
	res.UntrustedNotice = UntrustedNotice
	res.Text = fmt.Sprintf("skill/%s written, reviewed: %t, %d findings", entry.Name, entry.Reviewed, len(findings))

	return res, nil
}

// SkillImport imports a skill from a folder or an archive. It is never reviewed: a person reads it first.
func (s *svc) SkillImport(ctx context.Context, req SkillImportRequest) (*SkillImportResult, error) {
	c, m, err := s.libraryPrep(ctx, "SkillImport")
	if err != nil {
		return nil, err
	}

	if s.media.Guard == nil {
		return nil, internalError()
	}

	file, dir, gerr := s.media.Guard.OpenImportPath(req.Path)
	if gerr != nil {
		return nil, s.withRootsHint(c, fromEngine(gerr))
	}

	opts := library.ImportOptions{Source: req.Source, BaseName: baseName(req.Path), Replace: req.Replace}

	var (
		res  library.ImportResult
		ierr error
	)

	if dir != nil {
		defer func() { _ = dir.Close() }()

		res, ierr = m.ImportSkillDir(ctx, libCaller(c), dir, opts)
	} else {
		defer func() { _ = file.Close() }()

		res, ierr = m.ImportSkillArchive(ctx, libCaller(c), file, opts)
	}

	if ierr != nil {
		return nil, libError(ierr, "skill")
	}

	if err := s.libraryAudit(ctx, c, "library.skill.import", res.Entry.Key); err != nil {
		return nil, err
	}

	out := &SkillImportResult{
		Entry:    toLibraryEntry(c, &res.Entry),
		Findings: libFindings(res.Findings),
		Files:    res.Files,
	}
	out.UntrustedNotice = UntrustedNotice
	out.Text = fmt.Sprintf(
		"skill/%s imported from %s: %d files, %d findings; it is not reviewed, so no agent is offered it until a person reads it",
		res.Entry.Name,
		baseName(req.Path),
		res.Files,
		len(res.Findings),
	)

	return out, nil
}

// baseName is the last element of a path in either syntax, cut to a length that is safe to record.
func baseName(p string) string {
	p = strings.TrimRight(strings.ReplaceAll(p, `\`, "/"), "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}

	return p
}

// SkillMarkReviewed records that a person read a skill, as it was when they read it. Only a person at the
// browser may.
func (s *svc) SkillMarkReviewed(ctx context.Context, req SkillReviewRequest) (*SkillReviewResult, error) {
	c, m, err := s.libraryPrep(ctx, "SkillMarkReviewed")
	if err != nil {
		return nil, err
	}

	if !c.Browser {
		return nil, newError(CodeForbidden, "only a person at the browser marks a skill as read.").
			withHint("open the skill in the Skills section of the desktop application or of a signed-in session of network mode, and mark it as read there")
	}

	entry, _, rerr := m.MarkReviewed(ctx, libCaller(c), req.Name, req.ContentHash)
	if errors.Is(rerr, library.ErrChanged) {
		return nil, newError(command.CodeRevisionConflict, "the skill changed since it was read; it is not marked.").
			withHint("read the skill again, then mark it as read")
	}

	if rerr != nil {
		return nil, libError(rerr, "skill")
	}

	if err := s.libraryAudit(ctx, c, "library.skill.review", entry.Key); err != nil {
		return nil, err
	}

	res := &SkillReviewResult{Entry: toLibraryEntry(c, &entry)}
	res.UntrustedNotice = UntrustedNotice
	res.Text = "skill/" + entry.Name + " marked as read"

	return res, nil
}

// SkillLint runs the lint on a skill folder.
func (s *svc) SkillLint(ctx context.Context, req LibraryRef) (*SkillLintResult, error) {
	_, m, err := s.libraryPrep(ctx, "SkillLint")
	if err != nil {
		return nil, err
	}

	findings, lerr := m.LintSkill(ctx, req.Name)
	if lerr != nil {
		return nil, libError(lerr, "skill")
	}

	res := &SkillLintResult{Name: req.Name, Findings: libFindings(findings)}

	for i := range res.Findings {
		if res.Findings[i].Severity == core.SeverityError {
			res.Errors++
		} else {
			res.Warnings++
		}
	}

	res.Text = fmt.Sprintf("skill/%s: %d errors, %d warnings", req.Name, res.Errors, res.Warnings)

	if lines := findingLines(res.Findings); lines != "" {
		res.Text += "\n" + findingsBlock(req.Name, lines)
	}

	return res, nil
}

// SkillExport writes skills as folders in the Agent Skills layout, to exports/library/ of the workspace.
func (s *svc) SkillExport(ctx context.Context, req SkillExportRequest) (*SkillExportResult, error) {
	c, m, err := s.libraryPrep(ctx, "SkillExport")
	if err != nil {
		return nil, err
	}

	if c.ProjectID != "" {
		return nil, newError(CodeForbidden, "the key is limited to one project and cannot export skills.").
			withHint("use a key without a project limit")
	}

	out, eerr := m.ExportSkills(ctx, req.Names)
	if eerr != nil {
		return nil, libError(eerr, "skill")
	}

	if len(out.Names) > 0 {
		if err := s.libraryAudit(ctx, c, "library.skill.export", strings.Join(out.Names, ",")); err != nil {
			return nil, err
		}
	}

	res := &SkillExportResult{Names: out.Names}
	if res.Names == nil {
		res.Names = []string{}
	}

	if c.OwnerUserID == "" && len(out.Names) > 0 {
		res.Path = out.Dir
	}

	res.Text = fmt.Sprintf("%d skills exported", len(out.Names))

	if len(out.Names) == 0 {
		res.Text = "no reviewed skill to export"
	}

	return res, nil
}

// LibrarySearch runs one full-text query over the three kinds. The answer fits the budget and every text in it is
// delimited as untrusted.
func (s *svc) LibrarySearch(ctx context.Context, req LibrarySearchRequest) (*LibrarySearchResult, error) {
	c, m, err := s.libraryPrep(ctx, "LibrarySearch")
	if err != nil {
		return nil, err
	}

	f := library.Filter{
		Scope: req.Scope, Tags: req.Tags, Origin: req.Origin, ProjectID: c.ProjectID,
		IncludeUnreviewed: req.IncludeUnreviewed && !req.AgentView,
	}

	for _, k := range req.Kinds {
		kind := core.Kind(k)
		if !kind.Valid() {
			return nil, newError(command.CodeValidation, "kind %q is not memory, skill or prompt.", k).withPath("kinds")
		}

		f.Kinds = append(f.Kinds, kind)
	}

	found, serr := m.Search(ctx, library.SearchQuery{
		Query: req.Query, Filter: f, Limit: req.Limit, Budget: req.Budget, Agent: req.AgentView,
	})
	if serr != nil {
		return nil, libError(serr, "entry")
	}

	res := &LibrarySearchResult{
		Hits:      make([]LibraryHit, 0, len(found.Hits)),
		Total:     found.Total,
		Truncated: found.Truncated,
	}
	res.UntrustedNotice = UntrustedNotice

	var b strings.Builder

	fmt.Fprintf(&b, "%d of %d results", len(found.Hits), found.Total)

	if found.Truncated {
		b.WriteString(", the rest cut to fit the budget: narrow the query or filter by kind")
	}

	for i := range found.Hits {
		h := &found.Hits[i]
		entry := toLibraryEntry(c, &h.Entry)
		hit := LibraryHit{LibraryEntry: entry, Snippet: libText(c, h.Snippet, core.MaxSnippet+2)}
		res.Hits = append(res.Hits, hit)

		fmt.Fprintf(&b, "\n%s\n%s", libraryLine(&entry),
			untrusted.Block(untrusted.SourceLibraryText, untrusted.Attrs{"kind": entry.Kind, "name": entry.Name},
				h.Entry.Description+"\n"+h.Snippet, 0))
	}

	res.Text = b.String()

	return res, nil
}

// LibraryReindex empties the index and reads every file again. It changes no file.
func (s *svc) LibraryReindex(ctx context.Context, _ LibraryReindexRequest) (*LibraryReindexResult, error) {
	c, m, err := s.libraryPrep(ctx, "LibraryReindex")
	if err != nil {
		return nil, err
	}

	st, rerr := m.Reindex(ctx)
	if rerr != nil {
		return nil, libError(rerr, "entry")
	}

	res := &LibraryReindexResult{Memory: st.Memory, Skills: st.Skills, Prompts: st.Prompts, Invalid: st.Invalid}
	for _, name := range st.Ignored {
		res.Ignored = append(res.Ignored, libText(c, name, 200))
	}

	res.UntrustedNotice = UntrustedNotice
	res.Text = fmt.Sprintf("index rebuilt: %d notes, %d skills, %d prompts, %d invalid, %d ignored",
		st.Memory, st.Skills, st.Prompts, st.Invalid, len(st.Ignored))

	return res, nil
}
