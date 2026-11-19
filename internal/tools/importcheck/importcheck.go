// Package importcheck enforces the import rules of ADR-0001 over the Go
// files of the repository module. It parses import declarations only, so it
// needs no build and sees files excluded by build tags and test files too.
//
// Rules:
//   - core/ imports the standard library, its own packages and the pure
//     libraries in [CoreLibraries]; nothing else.
//   - engine/ never imports web/, service/ or vukem, except the process
//     runner [EngineException] that ADR-0001 rule 2 allows.
//   - service/ never imports web/ or vukem.
//   - every package outside web/ never imports vukem (rule 4).
//   - service/vault, which can return a credential, is imported only by service/connections, by the
//     owner that wires it and by the test environment of the contract tests (ADR-0033 item 3, SDD-014 section 3); tests may import it.
//   - only internal/outbound opens a connection to a host ([CheckOutbound]).
package importcheck

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// VukemModule is the framework that only web/ may import.
const VukemModule = "github.com/AlexGalhardo/vukem"

// EngineException is the one vukem package engine/ may import (ADR-0001 rule 2).
const EngineException = VukemModule + "/contrib/engine"

// CoreLibraries are the pure third-party libraries core/ may import, as
// import path prefixes. A library enters this list only together with the
// author's approval of the dependency (ADR-0014 names the JSON Schema validator).
var CoreLibraries = []string{
	"github.com/santhosh-tekuri/jsonschema",
}

// Violation is one forbidden import.
type Violation struct {
	File   string // slash path relative to the root
	Import string
	Rule   string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s imports %q: %s", v.File, v.Import, v.Rule)
}

// Check scans the module rooted at root (whose module path is modulePath)
// and returns every violation, sorted. Nested modules (a folder with its own
// go.mod, such as the spikes), dot and underscore folders, testdata,
// node_modules and site/ are skipped.
func Check(root, modulePath string) ([]Violation, error) {
	var out []Violation

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			if rel != "." && skipDir(path, d.Name()) {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(rel, ".go") {
			return nil
		}

		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}

		for _, spec := range f.Imports {
			imp, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return fmt.Errorf("parse %s: import %s: %w", rel, spec.Path.Value, err)
			}

			if rule := forbidden(rel, imp, modulePath); rule != "" {
				out = append(out, Violation{File: rel, Import: imp, Rule: rule})
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}

		return out[i].Import < out[j].Import
	})

	return out, nil
}

func skipDir(path, name string) bool {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") ||
		name == "testdata" || name == "node_modules" || name == "site" {
		return true
	}

	_, err := os.Stat(filepath.Join(path, "go.mod"))

	return err == nil
}

// forbidden returns the broken rule for a file importing imp, or "".
func forbidden(rel, imp, modulePath string) string {
	area, _, _ := strings.Cut(rel, "/")
	inVukem := under(imp, VukemModule)

	if imp == modulePath+"/service/vault" && !vaultImporter(rel) {
		return "service/vault is imported only by service/connections and the owner (ADR-0033 item 3): a credential is read inside the call that uses it"
	}

	switch area {
	case "web":
		return ""
	case "core":
		if strings.HasPrefix(rel, "core/command/") && under(imp, modulePath+"/core/plan") {
			return "core/command never imports core/plan: both read shared helpers from core/timeline (SDD-004 §1 rule 1)"
		}

		if isStdlib(imp) || under(imp, modulePath+"/core") || underAny(imp, CoreLibraries) {
			return ""
		}

		return "core/ imports only the standard library, core/ and the pure libraries of ADR-0001"
	case "engine":
		if imp == EngineException {
			return ""
		}

		switch {
		case under(imp, modulePath+"/web"):
			return "engine/ never imports web/ (ADR-0001 rule 2)"
		case under(imp, modulePath+"/service"):
			return "engine/ never imports service/, which imports engine/ (ADR-0001 rule 3)"
		case inVukem:
			return "engine/ imports no vukem package except contrib/engine (ADR-0001 rules 2 and 4)"
		}
	case "service":
		switch {
		case under(imp, modulePath+"/web"):
			return "service/ never imports web/, which implements its interfaces (ADR-0001 rule 3)"
		case inVukem:
			return "service/ imports no vukem package (ADR-0001 rule 3)"
		}
	default:
		if inVukem {
			return "only web/ imports vukem (ADR-0001 rule 4)"
		}
	}

	return ""
}

// vaultImporter reports whether a file may import service/vault.
func vaultImporter(rel string) bool {
	if strings.HasSuffix(rel, "_test.go") {
		return true
	}

	for _, p := range []string{"service/vault/", "service/connections/", "web/app/owner/", "web/contract/testenv/"} {
		if strings.HasPrefix(rel, p) {
			return true
		}
	}

	return false
}

func under(imp, prefix string) bool {
	return imp == prefix || strings.HasPrefix(imp, prefix+"/")
}

func underAny(imp string, prefixes []string) bool {
	for _, p := range prefixes {
		if under(imp, p) {
			return true
		}
	}

	return false
}

// isStdlib reports whether an import path is in the standard library: its
// first element has no dot. "C" (cgo) is not accepted; the product has no cgo.
func isStdlib(imp string) bool {
	first, _, _ := strings.Cut(imp, "/")

	return imp != "C" && !strings.Contains(first, ".")
}
