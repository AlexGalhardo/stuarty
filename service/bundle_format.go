package service

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/project"
)

// The container of a project bundle (SDD-005 section 16.1): a zip with the extension .vvbundle and
// flat entry names.
const (
	// BundleFormat is the value of the manifest's "format".
	BundleFormat = "stuarty-bundle"
	// formerBundleFormat is the value written before the rename (ADR-0044); a bundle that carries it is
	// still imported.
	formerBundleFormat = "vukem-video-bundle"
	// BundleVersion is the layout of the container; an importer refuses a higher one. Version 2 adds the
	// production and the timelines after the first (SDD-005 section 16.1.1); version 1 is still read.
	BundleVersion = 2

	bundleManifestEntry  = "manifest.json"
	bundleDocumentEntry  = "document.json"
	bundleOperationsName = "operations.ndjson"
	bundleTimelinePrefix = "timeline."
	bundleItemPrefix     = "item."
	bundleMainKey        = "main"
	bundleMediaPrefix    = "media."
	bundleFontPrefix     = "font."
	bundleExt            = ".vvbundle"
	// bundleExportFormat is the input.format of the export job that writes a bundle.
	bundleExportFormat = "bundle"
)

// BundleManifest is manifest.json, the first entry of a bundle.
type BundleManifest struct {
	Format        string           `json:"format"`
	BundleVersion int              `json:"bundle_version"`
	AppVersion    string           `json:"app_version"`
	CreatedAt     string           `json:"created_at"`
	Project       BundleProject    `json:"project"`
	Document      BundleDocument   `json:"document"`
	Operations    *BundleLog       `json:"operations,omitempty"`
	Media         []BundleMedia    `json:"media"`
	Fonts         []BundleFontFile `json:"fonts"`
	// Production and Timelines are the members of version 2: the items of the project, and every timeline but
	// the first (the first is Document and Operations).
	Production *BundleProduction `json:"production,omitempty"`
	Timelines  []BundleTimeline  `json:"timelines,omitempty"`

	// texts are the entries item.<n>.md of an export, by entry name; they are written after the manifest.
	texts map[string]string
}

// BundleProduction is the project around its timelines.
type BundleProduction struct {
	Name string `json:"name"`
	// Description and Goal are the context of the project (SDD-005 section 5.12.2); a project with none, and a
	// bundle made before epic 56, leave them out.
	Description string        `json:"description,omitempty"`
	Goal        *project.Goal `json:"goal,omitempty"`
	Items       []BundleItem  `json:"items"`
}

// BundleItem is an active item of the project. Timeline names the timeline by its key in the bundle ("main",
// "t1", ...); an output keeps its format and no job, because the file is not in the bundle. Text describes the
// entry item.<n>.md (n is the index of the item in the list) that holds the Markdown of a brief or a text that
// is not empty: the texts stay out of the manifest, whose size is capped far below what 256 items of 256 KiB
// hold.
type BundleItem struct {
	Kind     string          `json:"kind"`
	Title    string          `json:"title"`
	Position int             `json:"position"`
	TextKind string          `json:"text_kind,omitempty"` // kind text: script, copy or caption (SDD-019)
	Text     *BundleDocument `json:"text,omitempty"`
	Format   string          `json:"format,omitempty"`
	Timeline string          `json:"timeline,omitempty"`
}

// bundleItemTextEntry names the entry of the Markdown of the item at index i of the production.
func bundleItemTextEntry(i int) string { return bundleItemPrefix + strconv.Itoa(i) + ".md" }

// BundleTimeline is a timeline other than the first: its document entry timeline.<key>.json and its log
// timeline.<key>.ndjson, described as Document and Operations are for the first.
type BundleTimeline struct {
	Key           string         `json:"key"`
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	SchemaVersion int            `json:"schema_version"`
	Revision      int64          `json:"revision"`
	Document      BundleDocument `json:"document"`
	Operations    *BundleLog     `json:"operations,omitempty"`
}

// bundleTimelineDocEntry and bundleTimelineLogEntry name the entries of a timeline other than the first.
func bundleTimelineDocEntry(key string) string { return bundleTimelinePrefix + key + ".json" }
func bundleTimelineLogEntry(key string) string { return bundleTimelinePrefix + key + ".ndjson" }

// BundleProject names the project the bundle was taken from.
type BundleProject struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	SchemaVersion int    `json:"schema_version"`
	Revision      int64  `json:"revision"`
}

// BundleDocument is the canonical document entry.
type BundleDocument struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// BundleLog describes operations.ndjson.
type BundleLog struct {
	Count  int64  `json:"count"`
	SHA256 string `json:"sha256"`
}

// BundleMedia is one original file of the bundle, with every asset id that points at it.
type BundleMedia struct {
	Hash     string   `json:"hash"`
	Kind     string   `json:"kind"`
	Name     string   `json:"name"`
	Bytes    int64    `json:"bytes"`
	AssetIDs []string `json:"asset_ids"`
}

// BundleFontFile is one weight of a user font.
type BundleFontFile struct {
	Hash   string `json:"hash"`
	Family string `json:"family"`
	Weight int    `json:"weight"`
	Bytes  int64  `json:"bytes"`
}

// bundleOperation is a line of operations.ndjson: the operations row with its parsed columns
// embedded as JSON values, without the project id.
type bundleOperation struct {
	Revision       int64           `json:"revision"`
	Kind           OperationKind   `json:"kind"`
	Actor          string          `json:"actor"`
	BaseRevision   int64           `json:"base_revision"`
	TargetRevision *int64          `json:"target_revision,omitempty"`
	AtMs           int64           `json:"at_ms"`
	Ops            json.RawMessage `json:"ops,omitempty"`
	Inverse        json.RawMessage `json:"inverse,omitempty"`
	Diff           json.RawMessage `json:"diff,omitempty"`
	Skipped        json.RawMessage `json:"skipped,omitempty"`
	SchemaVersion  int             `json:"schema_version"`
	DocSHA256      string          `json:"doc_sha256"`
	ClosedBy       *int64          `json:"closed_by,omitempty"`
}

// bundleMediaEntry and bundleFontEntry name the entry of a content hash ("sha256:<hex>").
func bundleMediaEntry(hash string) string {
	return bundleMediaPrefix + strings.TrimPrefix(hash, "sha256:")
}

func bundleFontEntry(hash string) string {
	return bundleFontPrefix + strings.TrimPrefix(hash, "sha256:")
}

// bundleFileName is the file of a bundle export under exports/<project id>/.
func bundleFileName(revision int64, jobID string) string {
	return "bundle-r" + strconv.FormatInt(revision, 10) + "-" + jobID + bundleExt
}
