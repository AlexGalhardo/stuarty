package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// verdictWaived and verdictExpired are printed for a verdict line that a reviewed waiver covers:
// the first when the shipped text still matches the recorded checksum, the second when it does not.
const (
	verdictWaived  = "waived"
	verdictExpired = "expired"
)

// waiver records a reviewed decision that an SBOM scanner licence claim is wrong (SDD-009 section 20).
// It holds while the text at Path, inside the audited archive, still has the checksum SHA256.
type waiver struct {
	Package string `json:"package"`
	Claimed string `json:"claimed"`
	Shipped string `json:"shipped"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Reason  string `json:"reason"`
}

// waiverSet is the content of .config/sbom-waivers.json, applied against one extracted archive.
type waiverSet struct {
	Waivers []waiver `json:"waivers"`

	root *os.Root
}

// loadWaivers reads the waiver file and opens the extracted archive the checksums are read from.
func loadWaivers(file, archiveRoot string) (*waiverSet, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	var w waiverSet

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&w); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}

	for i, e := range w.Waivers {
		if e.Package == "" || e.Claimed == "" || e.Shipped == "" || e.Path == "" || e.SHA256 == "" || e.Reason == "" {
			return nil, fmt.Errorf("%s: waiver %d needs package, claimed, shipped, path, sha256 and reason", file, i+1)
		}
	}

	if archiveRoot != "" {
		if w.root, err = os.OpenRoot(archiveRoot); err != nil {
			return nil, err
		}
	}

	return &w, nil
}

// judge returns the verdict, the subject and the detail to print for a line that auditExpr flagged, or
// "" when no waiver names the package and the claimed id.
func (w *waiverSet) judge(subject, claimed string) (verdict, detail string) {
	if w == nil {
		return "", ""
	}

	for _, e := range w.Waivers {
		if e.Package != subject || e.Claimed != claimed {
			continue
		}

		sum, err := w.checksum(e.Path)

		switch {
		case err != nil:
			return verdictExpired, fmt.Sprintf("%s claimed, waiver text %s cannot be read: %v", claimed, e.Path, err)
		case !strings.EqualFold(sum, e.SHA256):
			return verdictExpired, fmt.Sprintf("%s claimed, the text at %s changed (sha256 %s)", claimed, e.Path, sum)
		}

		return verdictWaived, fmt.Sprintf("%s claimed, %s shipped: %s", claimed, e.Shipped, e.Reason)
	}

	return "", ""
}

func (w *waiverSet) checksum(path string) (string, error) {
	if w.root == nil {
		return "", errors.New("no archive to read it from")
	}

	raw, err := w.root.ReadFile(path)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(raw)

	return hex.EncodeToString(sum[:]), nil
}
