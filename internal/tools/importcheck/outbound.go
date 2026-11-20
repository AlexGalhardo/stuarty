package importcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// The outbound rule of SDD-007 section 10.2 and SDD-014 section 6: the only package that opens a
// connection to a host is internal/outbound. A client of a loopback address (the CLI towards the
// owner, the probe of the desktop shell) says so on the line, with the same marker the linter uses:
//
//	//nolint:forbidigo // loopback only: <why>
//
// Test files are not product code and are not scanned.

// outboundSelectors are the names that open a connection, by package. A DNS lookup is a packet to the
// resolver of the machine, so the lookups are here too (SDD-007 section 10.1 says which paths resolve).
var outboundSelectors = map[string]map[string]bool{
	"net": {
		"Dial": true, "DialTimeout": true, "DialTCP": true, "DialUDP": true, "DialIP": true, "DialUnix": true,
		"Dialer":     true,
		"LookupHost": true, "LookupIP": true, "LookupAddr": true, "LookupCNAME": true, "LookupMX": true,
		"LookupNS": true, "LookupSRV": true, "LookupTXT": true, "LookupPort": true, "Resolver": true,
		"DefaultResolver": true, "ResolveTCPAddr": true, "ResolveUDPAddr": true, "ResolveIPAddr": true,
	},
	"net/http": {
		"Get": true, "Post": true, "PostForm": true, "Head": true, "DefaultClient": true,
		"DefaultTransport": true, "Client": true, "Transport": true,
	},
	"crypto/tls": {"Dial": true, "DialWithDialer": true, "Dialer": true},
	"net/smtp":   {"Dial": true, "SendMail": true},
}

// outboundHome is where those names may be used without a marker.
const outboundHome = "internal/outbound/"

// outboundExempt are files with a documented reason: the owner probe and the handshake talk to a
// loopback address only (SDD-007 section 3.8), through a client the caller may replace.
var outboundExempt = map[string]bool{
	"service/workspace/open.go": true,
}

const loopbackMarker = "nolint:forbidigo"

// CheckOutbound scans the product Go files of the module at root and returns every use of a name
// that opens a connection outside internal/outbound that is not marked as loopback only.
func CheckOutbound(root string) ([]Violation, error) {
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

		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") ||
			strings.HasPrefix(rel, outboundHome) || outboundExempt[rel] {
			return nil
		}

		found, err := outboundUses(path, rel)
		out = append(out, found...)

		return err
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].File < out[j].File || out[i].File == out[j].File && out[i].Import < out[j].Import
	})

	return out, nil
}

// outboundUses lists the unmarked uses in one file. Violation.Import carries the name and the line.
func outboundUses(path, rel string) ([]Violation, error) {
	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", rel, err)
	}

	// The local name of each package that matters.
	locals := map[string]string{}

	for _, spec := range f.Imports {
		imp, err := strconv.Unquote(spec.Path.Value)
		if err != nil || outboundSelectors[imp] == nil {
			continue
		}

		name := imp[strings.LastIndex(imp, "/")+1:]
		if spec.Name != nil {
			name = spec.Name.Name
		}

		locals[name] = imp
	}

	if len(locals) == 0 {
		return nil, nil
	}

	marked := map[int]bool{}

	for _, group := range f.Comments {
		for _, c := range group.List {
			if strings.Contains(c.Text, loopbackMarker) && strings.Contains(c.Text, "loopback only") {
				marked[fset.Position(c.Slash).Line] = true
			}
		}
	}

	var out []Violation

	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Obj != nil {
			return true
		}

		imp, ok := locals[id.Name]
		if !ok || !outboundSelectors[imp][sel.Sel.Name] {
			return true
		}

		line := fset.Position(sel.Pos()).Line
		if marked[line] || marked[line-1] {
			return true
		}

		out = append(out, Violation{
			File:   rel,
			Import: fmt.Sprintf("%s.%s (line %d)", id.Name, sel.Sel.Name, line),
			Rule: "only internal/outbound opens a connection to a host; a client of a loopback address carries " +
				"the comment //nolint:forbidigo // loopback only: <why> (SDD-007 section 10.2)",
		})

		return true
	})

	return out, nil
}
