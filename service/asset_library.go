package service

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service/textview"
)

// AssetListRequest asks for the media files the workspace holds, across its projects.
type AssetListRequest struct {
	// Kind keeps only "image", "video" or "audio" files; empty lists every kind.
	Kind string `json:"kind,omitempty"`
	// Limit keeps the first files of the list; 0 is the default of 500 (VV-192).
	Limit int `json:"limit,omitempty"`
}

// LibraryAsset is one file of the workspace by content, with the hash that the brand kit and font.add take.
type LibraryAsset struct {
	Hash     string `json:"hash"`
	Name     string `json:"name"             untrusted:"true"` // the name it was first imported under
	Kind     string `json:"kind"`
	Bytes    int64  `json:"bytes"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Projects int    `json:"projects"` // how many projects hold it
}

// AssetListResult lists the files, oldest name first.
type AssetListResult struct {
	Envelope
	Assets []LibraryAsset `json:"assets"`
	Text   string         `json:"text"`
}

// maxLibraryAssets bounds the answer; the scan of projects is bounded by maxFontOwnerScan.
const maxLibraryAssets = 500

type libraryEntry struct {
	name     string
	projects int
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

			for j := range rows {
				e, ok := byHash[rows[j].AssetHash]
				if !ok {
					e = &libraryEntry{name: rows[j].Name}
					byHash[rows[j].AssetHash] = e
				}

				e.projects++
			}
		}

		if next == "" {
			break
		}

		after = next
	}

	return byHash, nil
}

// AssetList lists the ready, active files of the projects the caller may see, by content hash, so a picker can
// offer the image of a brand logo without asking for a hash (VV-117). A key limited to one project lists that
// project's files only.
func (s *svc) AssetList(ctx context.Context, req AssetListRequest) (*AssetListResult, error) {
	c, err := authorize(ctx, "AssetList", "")
	if err != nil {
		return nil, err
	}

	if err := checkLimit(req.Limit); err != nil {
		return nil, err
	}

	if req.Kind != "" && req.Kind != "image" && req.Kind != "video" && req.Kind != "audio" {
		return nil, newError(command.CodeValidation, "the kind %q is not known.", req.Kind).
			withPath("kind").withHint("use image, video or audio, or omit it")
	}

	byHash, serr := s.libraryScan(ctx, c)
	if serr != nil {
		return nil, serr
	}

	res := &AssetListResult{UntrustedNotice: UntrustedNotice, Assets: []LibraryAsset{}}

	for hash, e := range byHash {
		ar, gerr := s.store.Assets().Get(ctx, hash)
		if gerr != nil || ar.IngestState != IngestReady || req.Kind != "" && ar.Kind != req.Kind {
			continue
		}

		a := LibraryAsset{Hash: hash, Name: e.name, Kind: ar.Kind, Bytes: ar.SizeBytes, Projects: e.projects}
		if ar.Facts.Width != nil && ar.Facts.Height != nil {
			a.Width, a.Height = *ar.Facts.Width, *ar.Facts.Height
		}

		res.Assets = append(res.Assets, a)
	}

	slices.SortFunc(res.Assets, func(a, b LibraryAsset) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}

		return strings.Compare(a.Hash, b.Hash)
	})

	if keep := clampLimit(req.Limit, maxLibraryAssets, maxLibraryAssets); len(res.Assets) > keep {
		res.Assets = res.Assets[:keep]
	}

	res.Text = libraryText(res.Assets)

	return res, nil
}

func libraryText(assets []LibraryAsset) string {
	var b strings.Builder

	b.WriteString(textview.Note + "\n")
	fmt.Fprintf(&b, "%d files in the workspace", len(assets))

	for i := range assets {
		a := &assets[i]
		fmt.Fprintf(&b, "\n%s %s %dx%d %s", a.Hash, orDash(a.Kind), a.Width, a.Height, textview.Quote(a.Name, 80))
	}

	return b.String()
}
