package main

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Verdicts printed by the audit modes, one per line: "verdict<TAB>subject<TAB>reason".
// scripts/release-audit.sh counts them; the allow-list is the one of the notices
// generator, so there is one source of truth (SDD-009 section 12.2 item 4).
const (
	verdictDeny       = "deny"       // GPL, AGPL, SSPL or another denied family
	verdictNotAllowed = "notallowed" // a known licence outside the allow-list
	verdictUnknown    = "unknown"    // no licence could be identified
)

// deniedPrefixes are the families that are never redistributable with the product.
var deniedPrefixes = []string{"GPL", "AGPL", "SSPL", "Commons-Clause", "LGPL-2", "LGPL-3.0-only"}

// auditExpr judges one SPDX expression found in an SBOM. The SBOM names no
// group, so the per-group limits (OFL-1.1 for fonts, LGPL for FFmpeg) are not
// applied: the verdict is about the allow-list and the denied families only.
// "A OR B" passes when one alternative does; "A AND B" needs every id.
func auditExpr(expr string) (verdict, reason string) {
	expr = strings.NewReplacer("(", " ", ")", " ").Replace(expr)

	var worst, why string

	for alt := range strings.SplitSeq(expr, " OR ") {
		v, r := auditAlternative(alt)
		if v == "" {
			return "", ""
		}

		if worst == "" || rank(v) > rank(worst) {
			worst, why = v, r
		}
	}

	return worst, why
}

func rank(v string) int {
	switch v {
	case verdictDeny:
		return 3
	case verdictNotAllowed:
		return 2
	default:
		return 1
	}
}

func auditAlternative(alt string) (verdict, reason string) {
	for id := range strings.SplitSeq(alt, " AND ") {
		id = strings.TrimSpace(id)

		var v string

		switch {
		case id == "":
		case allowed[id]:
		case id == "NOASSERTION" || id == "NONE" || strings.HasPrefix(id, "LicenseRef-"):
			v = verdictUnknown
		case slices.ContainsFunc(deniedPrefixes, func(p string) bool { return strings.HasPrefix(id, p) }):
			v = verdictDeny
		default:
			v = verdictNotAllowed
		}

		if v != "" && (verdict == "" || rank(v) > rank(verdict)) {
			verdict, reason = v, id
		}
	}

	return verdict, reason
}

// auditExprs reads one SPDX expression per line and prints a verdict line for
// each one that is not clean. It returns the number of lines printed.
func auditExprs(in io.Reader, out io.Writer) (int, error) {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	n := 0

	for sc.Scan() {
		subject, expr, ok := strings.Cut(sc.Text(), "\t")
		if !ok {
			subject, expr = "", subject
		}

		if v, id := auditExpr(expr); v != "" {
			fmt.Fprintf(out, "%s\t%s\t%s\n", v, subject, id)

			n++
		}
	}

	return n, sc.Err()
}

// auditTree matches every licence text under root/licenses with the same
// signatures as the generator and applies the allow-list, including the per-group
// limits. Notice files are skipped, as in the generator. It returns the number
// of verdict lines printed.
func auditTree(root string, out io.Writer) (int, error) {
	base := filepath.Join(root, "licenses")
	n := 0

	lic, err := os.OpenRoot(base)
	if err != nil {
		return 0, err
	}

	defer lic.Close()

	err = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || isNotice(d.Name()) {
			return err
		}

		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}

		rel = filepath.ToSlash(rel)

		raw, err := lic.ReadFile(rel)
		if err != nil {
			return err
		}

		group, name := auditGroup(rel)

		id := detect(strings.ReplaceAll(string(raw), "\r\n", "\n"))
		if id == "" {
			fmt.Fprintf(out, "%s\t%s\tno known licence text\n", verdictUnknown, rel)

			n++

			return nil
		}

		if err := checkSPDX(id, group, name); err != nil {
			v := verdictNotAllowed
			if slices.ContainsFunc(deniedPrefixes, func(p string) bool { return strings.HasPrefix(id, p) }) {
				v = verdictDeny
			}

			fmt.Fprintf(out, "%s\t%s\t%s\n", v, rel, id)

			n++
		}

		return nil
	})

	return n, err
}

// auditGroup derives the notices group and component name from a path below licenses/.
func auditGroup(rel string) (group, name string) {
	parts := strings.Split(rel, "/")

	switch parts[0] {
	case "fonts":
		return groupFonts, parts[len(parts)-1]
	case "programs":
		if len(parts) > 1 && parts[1] == "ffmpeg" {
			return groupPrograms, "FFmpeg"
		}

		return groupPrograms, parts[min(1, len(parts)-1)]
	case "vendored":
		return groupVendored, parts[min(1, len(parts)-1)]
	case "npm":
		return groupNPM, strings.Join(parts[1:len(parts)-1], "/")
	default:
		return groupGo, strings.Join(parts[1:len(parts)-1], "/")
	}
}

// runAudit is the audit entry of run. It exits 1 when a verdict line was
// printed, so a caller can also use the exit code.
func runAudit(exprs bool, dir string, stdin io.Reader, stdout, stderr io.Writer) int {
	total := 0

	if exprs {
		n, err := auditExprs(stdin, stdout)
		if err != nil {
			fmt.Fprintln(stderr, "notices:", err)

			return 2
		}

		total += n
	}

	if dir != "" {
		n, err := auditTree(dir, stdout)
		if err != nil {
			fmt.Fprintln(stderr, "notices:", err)

			return 2
		}

		total += n
	}

	if total > 0 {
		return 1
	}

	return 0
}
