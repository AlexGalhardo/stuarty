package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Component is one row of THIRD_PARTY_NOTICES.md.
type Component struct {
	Group     string
	Name      string
	Version   string
	SPDX      string
	Copyright string
	Note      string
	ShippedIn []string
	// Sources are licence files on disk that the generator copies into
	// Licenses (Go modules and npm packages). Inventory components have none:
	// their Licenses already exist in the repository.
	Sources []string
	// Licenses are repository-relative, slash-separated paths of the licence texts.
	Licenses []string
}

// Runner runs a command in a directory with extra environment variables and
// returns its standard output. Tests replace it.
type Runner func(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error)

// buildTargets are the six platforms of the release (SDD-009 section 10). The Go
// modules are collected for all of them so that the result does not depend on
// the operating system that runs the generator.
var buildTargets = [][2]string{
	{
		"linux",
		"amd64",
	},
	{"linux", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"windows", "amd64"},
	{"windows", "arm64"},
}

// goRoot is a Go module whose binary is shipped: the directory to run `go list`
// in, the package to list, the targets it is built for and CGO_ENABLED. The
// desktop shell is a nested module (ADR-0023) that links Wails v3, which needs
// cgo on macOS and Linux; `go list` does not compile, so it needs no C compiler.
type goRoot struct {
	dir, pkg, cgo string
	targets       [][2]string
}

var goRoots = []goRoot{
	{dir: ".", pkg: "./web/cmd/app", cgo: "0", targets: buildTargets},
	{dir: "desktop/shell/app", pkg: ".", cgo: "1", targets: desktopTargets},
}

// desktopTargets are the systems the desktop shell is built for.
var desktopTargets = [][2]string{{"linux", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"}}

// productModule is the module of the repository; the shell requires it through a
// replace directive, and it is not a third-party component.
const productModule = "github.com/AlexGalhardo/stuarty"

var licenseName = regexp.MustCompile(`(?i)^(licen[sc]e|copying|notice)([-_.].*)?$`)

// readLicenseFiles lists the licence files of a directory (not recursive) and
// reads them, with CRLF turned into LF so the output is the same everywhere.
func readLicenseFiles(dir string) (names []string, texts map[string]string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}

	texts = map[string]string{}

	for _, e := range entries {
		if e.IsDir() || !licenseName.MatchString(e.Name()) {
			continue
		}

		raw, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			return nil, nil, rerr
		}

		names = append(names, e.Name())
		texts[e.Name()] = strings.ReplaceAll(string(raw), "\r\n", "\n")
	}

	slices.Sort(names)

	return names, texts, nil
}

// isNotice reports whether a file is copied without being matched against the
// licence table: a NOTICE file, the licence of a logo (modernc.org/memory
// ships LICENSE-LOGO, a link to the logo's source, not a licence of the code) or
// an accounting of other components (modernc.org/sqlite ships LICENSE-3RD-PARTY.md).
func isNotice(name string) bool {
	n := strings.ToLower(name)

	stem := strings.TrimSuffix(n, filepath.Ext(n))

	return strings.HasPrefix(n, "notice") || strings.HasSuffix(stem, "-logo") || strings.HasSuffix(stem, "-3rd-party")
}

// licenseOf finds the SPDX expression of a directory from its licence files.
func licenseOf(kind, name, dir string) (spdx, copyright string, files []string, err error) {
	names, texts, err := readLicenseFiles(dir)
	if err != nil {
		return "", "", nil, fmt.Errorf("%s %s: %w", kind, name, err)
	}

	var ids []string

	for _, n := range names {
		if isNotice(n) {
			continue
		}

		id := detect(texts[n])
		if id == "" {
			return "", "", nil, fmt.Errorf(
				"%s %s: the licence text %s matches no known licence; add it to the generator or to the inventory",
				kind,
				name,
				n,
			)
		}

		ids = append(ids, id)

		if copyright == "" {
			copyright = copyrightLine(texts[n])
		}
	}

	if len(ids) == 0 {
		return "", "", nil, fmt.Errorf("%s %s: no licence file in %s", kind, name, dir)
	}

	return joinIDs(ids), copyright, names, nil
}

var copyrightRe = regexp.MustCompile(`(?i)^\W*(copyright\b|\(c\)).*`)

// copyrightLine is the first line of a licence text that names a copyright.
func copyrightLine(text string) string {
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if copyrightRe.MatchString(line) && !strings.Contains(strings.ToLower(line), "copyright owner") &&
			!strings.Contains(strings.ToLower(line), "copyright holder") {
			return truncate(line, 160)
		}
	}

	return ""
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}

	return string(r[:n-1]) + "…"
}

// goModules collects the Go modules the application binary links, for every
// release target (SDD-009 section 12.2 item 1).
func goModules(ctx context.Context, run Runner, root string) ([]Component, error) {
	seen := map[string]Component{}

	for _, gr := range goRoots {
		for _, tg := range gr.targets {
			out, err := run(
				ctx,
				filepath.Join(root, filepath.FromSlash(gr.dir)),
				[]string{"GOOS=" + tg[0], "GOARCH=" + tg[1], "CGO_ENABLED=" + gr.cgo},
				"go",
				"list",
				"-deps",
				"-f",
				`{{with .Module}}{{if not .Main}}{{.Path}}{{"\t"}}{{.Version}}{{"\t"}}{{.Dir}}{{end}}{{end}}`,
				gr.pkg,
			)
			if err != nil {
				return nil, fmt.Errorf("go list %s for %s/%s: %w", gr.pkg, tg[0], tg[1], err)
			}

			for line := range strings.SplitSeq(string(out), "\n") {
				parts := strings.Split(strings.TrimRight(line, "\r"), "\t")
				if len(parts) != 3 || parts[0] == "" || parts[0] == productModule {
					continue
				}

				seen[parts[0]+"@"+parts[1]] = Component{
					Group:   groupGo,
					Name:    parts[0],
					Version: parts[1],
					Sources: []string{parts[2]},
				}
			}
		}
	}

	out := make([]Component, 0, len(seen))

	for key := range seen {
		c := seen[key]
		spdx, cr, files, err := licenseOf("Go module", c.Name, c.Sources[0])
		if err != nil {
			return nil, err
		}

		c.SPDX, c.Copyright = spdx, cr
		c.Sources = absAll(c.Sources[0], files)
		c.Licenses = outputPaths("go-modules", c.Name, c.Version, files)
		out = append(out, c)
	}

	return out, nil
}

func absAll(dir string, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = filepath.Join(dir, n)
	}

	return out
}

// outputPaths are the repository paths of the copied licence files:
// licenses/<group>/<name>@<version>/<file>.
func outputPaths(group, name, version string, files []string) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = path.Join("licenses", group, name+"@"+version, f)
	}

	return out
}

type packageJSON struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	License      json.RawMessage   `json:"license"`
	Licenses     []json.RawMessage `json:"licenses"`
	Author       json.RawMessage   `json:"author"`
	Dependencies map[string]string `json:"dependencies"`
}

// licenseField reads the license of a package.json, which is a string, an
// object with a type, or (old packages) a list of them.
func (p packageJSON) licenseField() string {
	var ids []string

	add := func(raw json.RawMessage) {
		var s string
		if json.Unmarshal(raw, &s) == nil && s != "" {
			ids = append(ids, s)

			return
		}

		var o struct {
			Type string `json:"type"`
		}

		if json.Unmarshal(raw, &o) == nil && o.Type != "" {
			ids = append(ids, o.Type)
		}
	}

	if len(p.License) > 0 {
		add(p.License)
	}

	for _, l := range p.Licenses {
		add(l)
	}

	return strings.Join(ids, " OR ")
}

func (p packageJSON) author() string {
	var s string
	if json.Unmarshal(p.Author, &s) == nil && s != "" {
		return s
	}

	var o struct {
		Name string `json:"name"`
	}

	if json.Unmarshal(p.Author, &o) == nil {
		return o.Name
	}

	return ""
}

// npmPackages walks the production closure of web/package.json through
// web/node_modules (only "dependencies", transitively: never devDependencies,
// and not optionalDependencies, which differ between operating systems).
func npmPackages(root string) ([]Component, error) {
	webDir := filepath.Join(root, "web")

	raw, err := os.ReadFile(filepath.Join(webDir, "package.json"))
	if err != nil {
		return nil, err
	}

	var top packageJSON
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("web/package.json: %w", err)
	}

	visited := map[string]bool{}
	byKey := map[string]Component{}
	queue := make([]string, 0, len(top.Dependencies))

	for dep := range top.Dependencies {
		queue = append(queue, webDir+"|"+dep)
	}

	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]

		from, dep, _ := strings.Cut(next, "|")

		dir, err := resolveNPM(from, dep, webDir)
		if err != nil {
			return nil, err
		}

		if visited[dir] {
			continue
		}

		visited[dir] = true

		pj, err := readPackageJSON(dir)
		if err != nil {
			return nil, err
		}

		c, err := npmComponent(dep, dir, pj)
		if err != nil {
			return nil, err
		}

		byKey[c.Name+"@"+c.Version] = c

		for d := range pj.Dependencies {
			queue = append(queue, dir+"|"+d)
		}
	}

	out := make([]Component, 0, len(byKey))
	for key := range byKey {
		out = append(out, byKey[key])
	}

	return out, nil
}

func readPackageJSON(dir string) (packageJSON, error) {
	var pj packageJSON

	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return pj, err
	}

	if err := json.Unmarshal(raw, &pj); err != nil {
		return pj, fmt.Errorf("%s: %w", filepath.Join(dir, "package.json"), err)
	}

	return pj, nil
}

// resolveNPM finds the directory of a dependency the way Node does: in the
// node_modules of the requesting package, then of each parent up to web/.
func resolveNPM(from, dep, webDir string) (string, error) {
	for dir := from; ; {
		if filepath.Base(dir) != "node_modules" {
			cand := filepath.Join(dir, "node_modules", filepath.FromSlash(dep))
			if st, err := os.Stat(cand); err == nil && st.IsDir() {
				return cand, nil
			}
		}

		parent := filepath.Dir(dir)
		if filepath.Clean(dir) == filepath.Clean(webDir) || parent == dir {
			break
		}

		dir = parent
	}

	return "", fmt.Errorf(
		"npm dependency %q of %s is not installed: run bun install --frozen-lockfile in web/",
		dep,
		from,
	)
}

func npmComponent(dep, dir string, pj packageJSON) (Component, error) {
	name := pj.Name
	if name == "" {
		name = dep
	}

	spdx := pj.licenseField()
	_, texts, _ := readLicenseFiles(dir)

	var (
		files     []string
		copyright string
	)

	for n := range texts {
		files = append(files, n)
	}

	slices.Sort(files)

	for _, n := range files {
		if !isNotice(n) {
			copyright = copyrightLine(texts[n])
			break
		}
	}

	if spdx == "" {
		detected, cr, _, err := licenseOf("npm package", name, dir)
		if err != nil {
			return Component{}, err
		}

		spdx, copyright = detected, cr
	}

	if copyright == "" {
		copyright = pj.author()
	}

	return Component{
		Group: groupNPM, Name: name, Version: pj.Version, SPDX: spdx, Copyright: copyright,
		Sources: absAll(dir, files), Licenses: outputPaths("npm", name, pj.Version, files),
	}, nil
}

// InventoryEntry is one record of .config/third-party.json.
type InventoryEntry struct {
	Group     string   `json:"group"`
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	SPDX      string   `json:"spdx"`
	Copyright string   `json:"copyright"`
	Files     []string `json:"files"`
	Note      string   `json:"note"`
	ShippedIn []string `json:"shipped_in"`
}

// inventory reads .config/third-party.json. A version "env:NAME" is read from
// .config/ffmpeg.env or .config/whisper.env. Each file must exist, and when its text matches a known
// licence the id must be one of the entry's SPDX ids.
func inventory(root string) ([]Component, error) {
	raw, err := os.ReadFile(filepath.Join(root, ".config", "third-party.json"))
	if err != nil {
		return nil, err
	}

	var entries []InventoryEntry

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&entries); err != nil {
		return nil, fmt.Errorf(".config/third-party.json: %w", err)
	}

	env := map[string]string{}

	for _, name := range []string{"ffmpeg.env", "whisper.env"} {
		part, err := readEnvFile(filepath.Join(root, ".config", name))
		if err != nil {
			return nil, err
		}

		maps.Copy(env, part)
	}

	out := make([]Component, 0, len(entries))

	for i := range entries {
		c, err := inventoryComponent(root, entries[i], env)
		if err != nil {
			return nil, err
		}

		out = append(out, c)
	}

	return out, nil
}

func inventoryComponent(root string, e InventoryEntry, env map[string]string) (Component, error) {
	version := e.Version

	if key, ok := strings.CutPrefix(version, "env:"); ok {
		v, found := env[key]
		if !found || v == "" {
			return Component{}, fmt.Errorf(
				"%s: %s is not set in .config/ffmpeg.env or .config/whisper.env",
				e.Name,
				key,
			)
		}

		version = v
	}

	if e.Name == "" || e.SPDX == "" || len(e.Files) == 0 {
		return Component{}, fmt.Errorf("inventory entry %q needs a name, an spdx and at least one file", e.Name)
	}

	switch e.Group {
	case groupFonts, groupPrograms, groupVendored:
	default:
		return Component{}, fmt.Errorf("inventory entry %q has the unknown group %q", e.Name, e.Group)
	}

	for _, f := range e.Files {
		if err := checkInventoryFile(root, e, f); err != nil {
			return Component{}, err
		}
	}

	return Component{
		Group: e.Group, Name: e.Name, Version: version, SPDX: e.SPDX, Copyright: e.Copyright, Note: e.Note,
		ShippedIn: e.ShippedIn, Licenses: slices.Clone(e.Files),
	}, nil
}

func checkInventoryFile(root string, e InventoryEntry, file string) error {
	if path.IsAbs(file) || strings.Contains(file, "..") {
		return fmt.Errorf("%s: the file %q must be a path inside the repository", e.Name, file)
	}

	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		return fmt.Errorf("%s: licence file %s: %w", e.Name, file, err)
	}

	if id := detect(string(raw)); id != "" && !strings.Contains(e.SPDX, id) {
		return fmt.Errorf("%s: the text of %s is %s but the inventory says %s", e.Name, file, id, e.SPDX)
	}

	return nil
}

// readEnvFile reads KEY="value" lines.
func readEnvFile(file string) (map[string]string, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]string{}, nil
		}

		return nil, err
	}

	out := map[string]string{}

	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		k, v, ok := strings.Cut(line, "=")
		if ok {
			out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}

	return out, nil
}
