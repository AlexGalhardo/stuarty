package service

import (
	"encoding/json"
	"strconv"
	"strings"
)

// The container of a project bundle (SDD-005 section 16.1): a zip with the extension .vvbundle and
// flat entry names.
const (
	// BundleFormat is the value of the manifest's "format".
	BundleFormat = "vukem-video-bundle"
	// BundleVersion is the layout of the container; an importer refuses a higher one.
	BundleVersion = 1

	bundleManifestEntry  = "manifest.json"
	bundleDocumentEntry  = "document.json"
	bundleOperationsName = "operations.ndjson"
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
}

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
