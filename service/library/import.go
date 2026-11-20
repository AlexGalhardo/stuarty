package library

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

// The limits of an import (SDD-018 section 8), on top of those of a skill.
const (
	maxArchiveEntries = 2000       // entries of an archive, of every kind
	maxInflated       = 16 << 20   // bytes a whole archive may inflate to, junk included
	maxSourceText     = 200        // characters of the source text
	junkDir           = "__MACOSX" // a folder the Finder adds to an archive
)

// ImportOptions says how a skill is imported.
type ImportOptions struct {
	// Source is the text recorded as where the skill came from; empty records the
	// base name of the folder or archive.
	Source string
	// BaseName is the base name of the folder or file that was read, for the source.
	BaseName string
	// Replace allows a skill of the same name to be replaced.
	Replace bool
}

// ImportResult is what an import wrote.
type ImportResult struct {
	Entry    Entry
	Findings []core.Finding
	Files    int
	Bytes    int
}

// ImportSkillDir imports the skill held by a folder: SKILL.md in it, or in the
// only folder it holds. root is the folder, opened by the caller after the path
// rules of SDD-007 section 7.1; nothing outside it is read.
func (m *Manager) ImportSkillDir(
	ctx context.Context,
	c Caller,
	root *os.Root,
	opts ImportOptions,
) (ImportResult, error) {
	var entries []core.File

	count := 0
	total := int64(0)

	err := walkSkill(root, ".", func(rel string, info fs.FileInfo) error {
		count++
		total += info.Size()

		if count > maxArchiveEntries || total > maxInflated {
			return errors.New("the folder is far over the size limits of a skill")
		}

		if isJunk(rel) {
			return nil
		}

		// A file over the limit is refused, not left out: a skill imported without one of its files would be
		// another skill than the one the person chose. A file next to the skill folder is refused anyway (one
		// skill per source).
		if info.Size() > core.MaxSkillFile {
			return fmt.Errorf(
				"the file %s is %d bytes; the limit is %d",
				printable(rel),
				info.Size(),
				core.MaxSkillFile,
			)
		}

		data, _, err := readSmall(root, rel, core.MaxSkillFile)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}

		entries = append(entries, core.File{Path: rel, Data: data})

		return nil
	})
	if err != nil {
		return ImportResult{}, invalid("source", "%v", err)
	}

	return m.importEntries(ctx, c, entries, opts)
}

// ImportSkillArchive imports the skill held by a .zip or .tar.gz file. The
// format is told by the first bytes and never by the name. The archive is read
// in memory and checked entry by entry before anything is written.
func (m *Manager) ImportSkillArchive(
	ctx context.Context,
	c Caller,
	f *os.File,
	opts ImportOptions,
) (ImportResult, error) {
	info, err := f.Stat()
	if err != nil {
		return ImportResult{}, err
	}

	if info.Size() > core.MaxArchiveBytes {
		return ImportResult{}, &ValidationError{Field: "source", Msg: fmt.Sprintf(
			"the archive is %d bytes; the limit is %d", info.Size(), core.MaxArchiveBytes)}
	}

	var magic [4]byte
	if _, err := io.ReadFull(io.NewSectionReader(f, 0, 4), magic[:]); err != nil {
		return ImportResult{}, invalid("source", "the file is not a .zip or a .tar.gz archive")
	}

	var entries []core.File

	switch {
	case magic[0] == 'P' && magic[1] == 'K':
		entries, err = readZip(f, info.Size())
	case magic[0] == 0x1f && magic[1] == 0x8b:
		entries, err = readTarGz(io.NewSectionReader(f, 0, info.Size()))
	default:
		return ImportResult{}, invalid("source", "the file is not a .zip or a .tar.gz archive")
	}

	if err != nil {
		return ImportResult{}, err
	}

	return m.importEntries(ctx, c, entries, opts)
}

// checkEntryName cleans the name of an archive entry and refuses a name that is
// not a plain relative path: absolute, with a drive letter, a backslash, a "."
// or ".." component, an empty one, a character that is not printable ASCII, a
// Windows device name.
func checkEntryName(raw string) (clean string, isDir bool, err error) {
	name := raw

	for strings.HasPrefix(name, "./") {
		name = name[2:]
	}

	if strings.HasSuffix(name, "/") {
		isDir = true
		name = strings.TrimRight(name, "/")
	}

	if name == "" || name == "." {
		return "", isDir, nil
	}

	if err := core.ValidFilePath(name); err != nil {
		return "", false, invalid("source", "the entry %q is refused: %v", printable(raw), err)
	}

	return name, isDir, nil
}

// archiveCollector gathers the files of an archive, counting what it inflates.
type archiveCollector struct {
	files    []core.File
	seen     map[string]string // folded name to the name as written
	entries  int
	inflated int64
}

func newCollector() *archiveCollector { return &archiveCollector{seen: map[string]string{}} }

// count counts an entry of any kind against the limit on entries.
func (a *archiveCollector) count() error {
	a.entries++
	if a.entries > maxArchiveEntries {
		return invalid("source", "the archive has more than %d entries", maxArchiveEntries)
	}

	return nil
}

// add reads a regular entry. The read is bounded by what is left of the limits
// and not by the size the header declares, so a header that lies cannot make it
// inflate past them.
func (a *archiveCollector) add(name string, declared int64, open func() (io.ReadCloser, error)) error {
	folded := strings.ToLower(name)
	if other, dup := a.seen[folded]; dup {
		return invalid("source", "the entries %q and %q are the same name", printable(other), printable(name))
	}

	a.seen[folded] = name

	if declared > core.MaxSkillFile*2 && !isJunk(name) {
		return invalid(
			"source",
			"the entry %q is %d bytes; the limit for a file is %d",
			name,
			declared,
			core.MaxSkillFile,
		)
	}

	rc, err := open()
	if err != nil {
		return invalid("source", "the entry %q cannot be read: %v", name, err)
	}

	defer func() { _ = rc.Close() }()

	limit := min(int64(core.MaxSkillFile*2), maxInflated-a.inflated)

	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return invalid("source", "the entry %q cannot be read: %v", name, err)
	}

	if int64(len(data)) > limit {
		return invalid("source", "the entry %q inflates past the limits", name)
	}

	a.inflated += int64(len(data))
	a.files = append(a.files, core.File{Path: name, Data: data})

	return nil
}

func isJunk(name string) bool {
	return name == junkDir || strings.HasPrefix(name, junkDir+"/") || path.Base(name) == ".DS_Store"
}

func readZip(r io.ReaderAt, size int64) ([]core.File, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, invalid("source", "the archive cannot be read as a .zip")
	}

	if len(zr.File) > maxArchiveEntries {
		return nil, invalid("source", "the archive has more than %d entries", maxArchiveEntries)
	}

	col := newCollector()

	for _, e := range zr.File {
		if err := col.count(); err != nil {
			return nil, err
		}

		name, isDir, err := checkEntryName(e.Name)
		if err != nil {
			return nil, err
		}

		mode := e.Mode()

		switch {
		case e.Flags&0x1 != 0:
			return nil, invalid("source", "the entry %q is encrypted", printable(e.Name))
		case mode&fs.ModeSymlink != 0:
			return nil, invalid("source", "the entry %q is a symbolic link", printable(e.Name))
		case mode&(fs.ModeDevice|fs.ModeNamedPipe|fs.ModeSocket|fs.ModeCharDevice|fs.ModeIrregular) != 0:
			return nil, invalid("source", "the entry %q is not a regular file", printable(e.Name))
		case isDir || mode.IsDir() || name == "":
			continue
		}

		if err := col.add(name, declaredSize(e.UncompressedSize64), e.Open); err != nil {
			return nil, err
		}
	}

	return col.files, nil
}

func readTarGz(r io.Reader) ([]core.File, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, invalid("source", "the archive cannot be read as a .tar.gz")
	}

	defer func() { _ = gz.Close() }()

	// The whole stream, junk and skipped entries included, is bounded.
	tr := tar.NewReader(io.LimitReader(gz, maxInflated+1<<20))
	col := newCollector()

	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, invalid("source", "the archive cannot be read as a .tar.gz: %v", err)
		}

		if err := col.count(); err != nil {
			return nil, err
		}

		switch h.Typeflag {
		case tar.TypeXGlobalHeader:
			continue
		case tar.TypeSymlink, tar.TypeLink:
			return nil, invalid("source", "the entry %q is a link", printable(h.Name))
		case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			return nil, invalid("source", "the entry %q is not a regular file", printable(h.Name))
		case tar.TypeDir:
			if _, _, err := checkEntryName(h.Name); err != nil {
				return nil, err
			}

			continue
		case tar.TypeReg:
		default:
			return nil, invalid("source", "the entry %q has a type that is not allowed", printable(h.Name))
		}

		name, _, err := checkEntryName(h.Name)
		if err != nil {
			return nil, err
		}

		if name == "" {
			continue
		}

		if err := col.add(name, h.Size, func() (io.ReadCloser, error) { return io.NopCloser(tr), nil }); err != nil {
			return nil, err
		}
	}

	return col.files, nil
}

// selectSkill picks the one skill of the files of an archive or a folder:
// SKILL.md at the root, or inside the only top-level folder. Anything outside
// that folder is refused, and a path used both as a file and as a folder is
// refused.
func selectSkill(entries []core.File) ([]core.File, error) {
	kept := make([]core.File, 0, len(entries))

	for i := range entries {
		if !isJunk(entries[i].Path) {
			kept = append(kept, entries[i])
		}
	}

	if err := checkTree(kept); err != nil {
		return nil, err
	}

	for i := range kept {
		if kept[i].Path == core.SkillFile {
			return kept, nil
		}
	}

	tops := map[string]bool{}
	loose := ""

	for i := range kept {
		top, _, nested := strings.Cut(kept[i].Path, "/")
		if nested {
			tops[top] = true
		} else if loose == "" {
			loose = kept[i].Path
		}
	}

	switch {
	case loose != "" && len(tops) == 0:
		return nil, invalid("source", "the source holds no %s at its root and no folder", core.SkillFile)
	case loose != "":
		return nil, invalid("source", "the entry %q is outside the skill folder", printable(loose))
	case len(tops) != 1:
		return nil, invalid(
			"source",
			"the source holds no skill, or more than one: it must hold %s, or one folder that holds it",
			core.SkillFile,
		)
	}

	var top string
	for t := range tops {
		top = t
	}

	out := make([]core.File, 0, len(kept))
	hasMain := false

	for i := range kept {
		rest := strings.TrimPrefix(kept[i].Path, top+"/")
		if rest == kept[i].Path {
			return nil, invalid("source", "the entry %q is outside the skill folder", printable(kept[i].Path))
		}

		hasMain = hasMain || rest == core.SkillFile

		out = append(out, core.File{Path: rest, Data: kept[i].Data})
	}

	if !hasMain {
		return nil, invalid("source", "the folder %q has no %s", printable(top), core.SkillFile)
	}

	return out, nil
}

// checkTree refuses a path that is a file and also the folder of another entry.
func checkTree(files []core.File) error {
	set := make(map[string]bool, len(files))
	for i := range files {
		set[files[i].Path] = true
	}

	for i := range files {
		p := files[i].Path

		for {
			dir := path.Dir(p)
			if dir == "." || dir == "/" {
				break
			}

			if set[dir] {
				return invalid("source", "the path %q is both a file and a folder", printable(dir))
			}

			p = dir
		}
	}

	return nil
}

// importEntries turns the files of a source into a skill of the library.
func (m *Manager) importEntries(
	ctx context.Context,
	c Caller,
	entries []core.File,
	opts ImportOptions,
) (ImportResult, error) {
	if c.ProjectID != "" {
		return ImportResult{}, fmt.Errorf("%w: a key limited to a project does not import skills", ErrForbidden)
	}

	files, err := selectSkill(entries)
	if err != nil {
		return ImportResult{}, err
	}

	total := 0

	for _, f := range files {
		total += len(f.Data)

		limit := core.MaxSkillFile
		if f.Path == core.SkillFile {
			limit = core.MaxSkillMD
		}

		if len(f.Data) > limit {
			return ImportResult{}, invalid(
				"source",
				"the file %q is %d bytes; the limit is %d",
				f.Path,
				len(f.Data),
				limit,
			)
		}
	}

	if len(files) > core.MaxSkillFiles || total > core.MaxSkillBytes {
		return ImportResult{}, invalid("source", "the skill has %d files and %d bytes; the limits are %d and %d",
			len(files), total, core.MaxSkillFiles, core.MaxSkillBytes)
	}

	skill, err := core.ParseSkill(files)
	if err != nil {
		return ImportResult{}, fromCore(err)
	}

	if err := core.ValidName(skill.Name); err != nil {
		return ImportResult{}, invalid("name", "the skill name: %v", err)
	}

	if findings := structural(core.LintSkill(skill.Name, files)); len(findings) > 0 {
		return ImportResult{}, &ValidationError{
			Field:    "skill",
			Msg:      "the skill cannot be read as a skill",
			Findings: findings,
		}
	}

	rows, err := m.refresh(ctx)
	if err != nil {
		return ImportResult{}, err
	}

	if err := m.checkImportTarget(c, rows, skill.Name, opts.Replace); err != nil {
		return ImportResult{}, err
	}

	source := strings.TrimSpace(opts.Source)
	if source == "" {
		source = opts.BaseName
	}

	source = cutRunes(printable(source), maxSourceText)

	skill.Metadata = withoutProduct(skill.Metadata)
	skill.SetOrigin(core.OriginImported, source)

	all, err := skill.AllFiles()
	if err != nil {
		return ImportResult{}, invalid("", "%v", err)
	}

	if err := m.writeSkillFiles(ctx, skill.Name, all); err != nil {
		return ImportResult{}, err
	}

	_, entry, err := m.ReadSkill(ctx, skill.Name)
	if err != nil {
		return ImportResult{}, err
	}

	return ImportResult{Entry: entry, Findings: core.LintSkill(skill.Name, all), Files: len(all), Bytes: total}, nil
}

func (m *Manager) checkImportTarget(c Caller, rows []Entry, name string, replace bool) error {
	for i := range rows {
		if rows[i].Key != core.Key(core.KindSkill, name) {
			continue
		}

		if !replace {
			return fmt.Errorf("%w: a skill named %s exists; import with replace to overwrite it", ErrExists, name)
		}

		if rows[i].Origin != core.OriginImported && rows[i].Reviewed && !c.Person {
			return fmt.Errorf("%w: only a person replaces a skill that is kept", ErrForbidden)
		}

		return nil
	}

	if n := countKind(rows, core.KindSkill); n >= core.MaxSkills {
		return &LimitError{Msg: fmt.Sprintf("the workspace already has %d skills, the most it keeps", n)}
	}

	return nil
}

// structural keeps the findings that make a folder not a skill at all.
func structural(findings []core.Finding) []core.Finding {
	var out []core.Finding

	for i := range findings {
		switch findings[i].Code {
		case core.LintNoSkillMD, core.LintFrontMatter, core.LintName, core.LintNameMismatch, core.LintSize:
			out = append(out, findings[i])
		}
	}

	return out
}

// withoutProduct drops the keys of the product from a metadata map: what a file
// says about its own origin or review is never believed.
func withoutProduct(meta map[string]string) map[string]string {
	out := map[string]string{}

	for k, v := range meta {
		if !strings.HasPrefix(k, "stuarty-") {
			out[k] = v
		}
	}

	return out
}

func cutRunes(s string, limit int) string {
	rs := []rune(s)
	if len(rs) <= limit {
		return s
	}

	return string(rs[:limit])
}

// declaredSize reads the size a header declares, with a size that does not fit
// as the largest one, so that it is refused as over the limit.
func declaredSize(n uint64) int64 {
	if n > uint64(maxInflated) {
		return maxInflated + 1
	}

	return int64(n)
}
