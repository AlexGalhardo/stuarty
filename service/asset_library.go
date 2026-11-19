package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service/assets"
	"github.com/AlexGalhardo/stuarty/service/textview"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// AssetListRequest asks for the files the workspace holds, across its projects (SDD-011 section 14.4).
type AssetListRequest struct {
	// Kind keeps only "image", "video", "audio" or "lut" files; empty lists every kind.
	Kind string `json:"kind,omitempty"`
	// Section keeps the kinds of a section of the library: "visual" (image, lut) or "video" (video, audio).
	Section string `json:"section,omitempty"`
	// Tag keeps the files that carry the tag.
	Tag string `json:"tag,omitempty"`
	// Project keeps the files a production, or one of its timelines, uses.
	Project string `json:"project,omitempty"`
	// Origin keeps the files of one origin kind.
	Origin string `json:"origin,omitempty"`
	// Reusable keeps the files marked reusable (true) or not (false).
	Reusable *bool `json:"reusable,omitempty"`
	// Text is a full-text query over name, tags, description and origin.
	Text string `json:"text,omitempty"`
	// Limit keeps the first files of the list; 0 is the default of 500 (VV-192).
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// LibraryAsset is one file of the workspace by content, with the hash that the brand kit and font.add take,
// its origin, tags and reuse (SDD-011 section 14.1).
type LibraryAsset = assets.LibraryAsset

// AssetUse is a project that uses a file.
type AssetUse = assets.Use

// AssetListResult lists the files, oldest name first.
type AssetListResult struct {
	Envelope
	Assets     []LibraryAsset `json:"assets"`
	Total      int            `json:"total"`
	NextCursor *string        `json:"next_cursor"`
	Text       string         `json:"text"`
}

// maxLibraryAssets bounds the answer; the scan of projects is bounded by maxFontOwnerScan.
const maxLibraryAssets = 500

type libraryEntry struct {
	name     string
	projects int
	holdings []asset.Holding
}

// libraryScan reads the active files of the projects the caller may see, by hash, bounded by maxFontOwnerScan.
func (s *svc) libraryScan(ctx context.Context, c Caller) (map[string]*libraryEntry, error) {
	byHash := map[string]*libraryEntry{}

	var after string

	for scanned := 0; scanned < maxFontOwnerScan; {
		items, next, lerr := s.store.Projects().List(ctx, ProjectQuery{
			OwnerUserID: c.OwnerUserID, ProjectID: c.ProjectID, After: after, Limit: 200,
		})
		if lerr != nil {
			return nil, asError(lerr)
		}

		for i := range items {
			scanned++

			rows, perr := s.store.Assets().ListProjectAssets(ctx, items[i].ID, ProjectAssetActive)
			if perr != nil {
				return nil, asError(perr)
			}

			if len(rows) == 0 {
				continue
			}

			for j := range rows {
				e, ok := byHash[rows[j].AssetHash]
				if !ok {
					e = &libraryEntry{name: rows[j].Name}
					byHash[rows[j].AssetHash] = e
				}

				e.projects++
				e.holdings = append(e.holdings, asset.Holding{TimelineID: items[i].ID, AssetID: rows[j].AssetID})
			}
		}

		if next == "" {
			break
		}

		after = next
	}

	return byHash, nil
}

// libraryHeld is the files the caller may see, with their facts. Only the files that are ready are listed,
// unless wantAll is set.
func (s *svc) libraryHeld(ctx context.Context, c Caller, wantAll bool) ([]assets.Held, error) {
	byHash, serr := s.libraryScan(ctx, c)
	if serr != nil {
		return nil, serr
	}

	held := make([]assets.Held, 0, len(byHash))
	production := map[string]string{}

	// A timeline is a use of the production that holds it; a timeline that no item holds stands for itself.
	productionOf := func(timelineID string) string {
		if p, ok := production[timelineID]; ok {
			return p
		}

		p := timelineID
		if owner, err := s.store.Productions().ProjectOfTimeline(ctx, timelineID); err == nil {
			p = owner
		}

		production[timelineID] = p

		return p
	}

	for hash, e := range byHash {
		ar, gerr := s.store.Assets().Get(ctx, hash)
		if gerr != nil || !wantAll && ar.IngestState != IngestReady {
			continue
		}

		h := assets.Held{
			Hash: hash, Name: e.name, MediaKind: ar.Kind, Bytes: ar.SizeBytes, CreatedMs: ar.CreatedMs,
			Holdings: make([]asset.Holding, len(e.holdings)),
		}

		for i, hd := range e.holdings {
			hd.ProjectID = productionOf(hd.TimelineID)
			h.Holdings[i] = hd
		}

		if ar.Facts.Width != nil && ar.Facts.Height != nil {
			h.Width, h.Height = *ar.Facts.Width, *ar.Facts.Height
		}

		if ar.Facts.DurationMs != nil {
			h.DurationMS = *ar.Facts.DurationMs
		}

		held = append(held, h)
	}

	return held, nil
}

// ruleError turns a refusal of the library rules into the E_VALIDATION of SDD-011 section 14.2.
func ruleError(err error) error {
	if bad, ok := errors.AsType[*asset.RuleError](err); ok {
		return newError(command.CodeValidation, "%s", bad.Detail).withPath(bad.Path).withDetail("reason", bad.Reason)
	}

	return asError(err)
}

// libraryRefusal is the error of a transaction that wrote the library side of a file: a refusal of the rules
// is the caller's E_VALIDATION, anything else goes on as it is.
func libraryRefusal(err error) error {
	if _, ok := errors.AsType[*asset.RuleError](err); ok {
		return ruleError(err)
	}

	return err
}

// AssetList lists the ready, active files of the projects the caller may see, by content hash, so a picker can
// offer the image of a brand logo without asking for a hash (VV-117), and a person or an agent can find a file
// of a past production (RF-320). A key limited to one project lists that project's files only.
func (s *svc) AssetList(ctx context.Context, req AssetListRequest) (*AssetListResult, error) {
	c, err := authorize(ctx, "AssetList", "")
	if err != nil {
		return nil, err
	}

	if err := checkLimit(req.Limit); err != nil {
		return nil, err
	}

	if req.Kind != "" && req.Kind != "image" && req.Kind != "video" && req.Kind != "audio" && req.Kind != "lut" {
		return nil, newError(command.CodeValidation, "the kind %q is not known.", req.Kind).
			withPath("kind").withHint("use image, video, audio or lut, or omit it")
	}

	off := 0

	if req.Cursor != "" {
		var derr *Error
		if off, derr = decodeOffset(req.Cursor, "assets", ""); derr != nil {
			return nil, derr
		}
	}

	held, serr := s.libraryHeld(ctx, c, false)
	if serr != nil {
		return nil, serr
	}

	lib := assets.Library{Store: s.store.AssetLibrary()}

	page, lerr := lib.List(ctx, held, &assets.Query{
		Filter: asset.Filter{
			Kind: req.Kind, Section: req.Section, Tag: req.Tag, Project: req.Project,
			Origin: asset.OriginKind(req.Origin), Reusable: req.Reusable,
		},
		Text: req.Text, Offset: off, Limit: clampLimit(req.Limit, maxLibraryAssets, maxLibraryAssets),
	})
	if lerr != nil {
		return nil, ruleError(lerr)
	}

	res := &AssetListResult{UntrustedNotice: UntrustedNotice, Assets: page.Assets, Total: page.Total}
	if res.Assets == nil {
		res.Assets = []LibraryAsset{}
	}

	res.Text = assetListText(res.Assets, page.Total)

	if page.Next >= 0 {
		cur := offsetCursor("assets", "", page.Next)
		res.NextCursor = &cur
		res.Text += "\nnext_cursor: " + cur
	}

	return res, nil
}

// assetListTextCap is the budget of the text of a list, in characters (SDD-003 section 3.10).
const assetListTextCap = textview.CapMediaList

func assetListText(list []LibraryAsset, total int) string {
	var b strings.Builder

	b.WriteString(textview.Note + "\n")
	fmt.Fprintf(&b, "%d files in the library", total)

	for i := range list {
		line := assetLine(&list[i])
		if b.Len()+len(line)+1 > assetListTextCap {
			fmt.Fprintf(&b, "\n... %d more in this page: ask for a smaller limit or use next_cursor", len(list)-i)

			break
		}

		b.WriteString("\n" + line)
	}

	return b.String()
}

func assetLine(a *LibraryAsset) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s %s %s", a.Hash, orDash(a.Kind), textview.Quote(a.Name, 80))

	if a.Width > 0 {
		fmt.Fprintf(&b, " %dx%d", a.Width, a.Height)
	}

	if a.DurationMS > 0 {
		fmt.Fprintf(&b, " %.1fs", float64(a.DurationMS)/1000)
	}

	fmt.Fprintf(&b, " %s", a.Origin.Kind)

	if a.Reusable {
		b.WriteString(" reusable")
	}

	if len(a.Tags) > 0 {
		b.WriteString(" tags=" + strings.Join(a.Tags, ","))
	}

	fmt.Fprintf(&b, " in %d projects", a.Projects)

	return b.String()
}

// assetDetailText is the text of one entry: the facts, and the text from outside as delimited untrusted text.
func assetDetailText(a *LibraryAsset) string {
	var b strings.Builder

	b.WriteString(textview.Note + "\n" + assetLine(a))

	field := func(name, value string, budget int) {
		if value != "" {
			b.WriteString(
				"\n" + untrusted.Block(untrusted.SourceMetadata, untrusted.Attrs{"field": name}, value, budget),
			)
		}
	}

	if a.Origin.Provider != "" || a.Origin.Model != "" {
		fmt.Fprintf(
			&b, "\nprovider %s model %s", textview.Quote(a.Origin.Provider, asset.MaxProvider),
			textview.Quote(a.Origin.Model, asset.MaxModel),
		)
	}

	if a.Origin.CostMicros != nil {
		fmt.Fprintf(&b, "\ncost %.4f USD", float64(*a.Origin.CostMicros)/1e6)
	}

	field("source", a.Origin.Source, asset.MaxSource)
	field("prompt", a.Origin.Prompt, 1000)
	field("rights_note", a.Origin.RightsNote, asset.MaxRightsNote)
	field("description", a.Description, asset.MaxDescription)

	writeUses(&b, a.Uses)

	if a.Origin.RightsNote != "" {
		b.WriteString("\nThe rights note is the record of the user, not a licence check.")
	}

	return b.String()
}

// writeUses writes one line for each project that uses a file.
func writeUses(b *strings.Builder, uses []AssetUse) {
	for i := range uses {
		u := &uses[i]
		fmt.Fprintf(
			b, "\nused by %s %s as %s in %d timelines", u.ProjectID, textview.Quote(u.Name, 80), u.AssetID,
			len(u.Timelines),
		)
	}
}

// assetUsesText is the text of AssetUses: the file and the page of projects that use it. It says nothing of the
// facts of the file, which AssetUses does not read.
func assetUsesText(hash string, uses []AssetUse) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s\n%s used by %d projects in this page", textview.Note, hash, len(uses))
	writeUses(&b, uses)

	return b.String()
}

// parseHash accepts "sha256:" and 64 lower-case hex characters, or the 64 characters alone.
func parseHash(in string) (string, error) {
	h := strings.TrimPrefix(strings.TrimSpace(in), "sha256:")

	valid := len(h) == 64

	for _, r := range h {
		if r < '0' || r > '9' && r < 'a' || r > 'f' {
			valid = false
		}
	}

	if !valid {
		return "", newError(command.CodeValidation, "the hash is not a sha256 content hash.").
			withPath("hash").withHint("a hash is sha256: and 64 hex characters, as asset:list shows it")
	}

	return "sha256:" + h, nil
}
