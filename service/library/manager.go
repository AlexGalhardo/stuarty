package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

// Manager owns the files of the library of one workspace and keeps the index in
// step with them.
type Manager struct {
	files     files
	idx       Index
	now       func() time.Time
	exportDir string

	mu sync.Mutex

	keyMu sync.Mutex // guards key; taken after mu when both are held, never before
	key   []byte     // the review key, once read or made
}

// New returns the manager of the library folder dir (the library/ folder of the
// workspace). now is the clock; nil is time.Now.
func New(dir string, idx Index, now func() time.Time) *Manager {
	if now == nil {
		now = time.Now
	}

	return &Manager{files: files{dir: dir}, idx: idx, now: now, exportDir: exportDirOf(dir)}
}

// Stats says what a scan or a rebuild found.
type Stats struct {
	Memory   int      `json:"memory"`
	Skills   int      `json:"skills"`
	Prompts  int      `json:"prompts"`
	Invalid  int      `json:"invalid"`
	Removed  int      `json:"removed"`  // rows whose file is gone
	Ignored  []string `json:"ignored"`  // files whose name is not a valid name, at most 20
	Rebuilt  bool     `json:"rebuilt"`  // every file was read again
	Examined int      `json:"examined"` // files and folders that were read again
}

const maxIgnoredShown = 20

// Scan brings the index in step with the files: what is new or changed is read
// again, what is gone is dropped (SDD-018 section 9).
func (m *Manager) Scan(ctx context.Context) (Stats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.scan(ctx, false)
}

// Reindex empties the index and reads every file again.
func (m *Manager) Reindex(ctx context.Context) (Stats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.scan(ctx, true)
}

func (m *Manager) scan(ctx context.Context, rebuild bool) (Stats, error) {
	st := Stats{Rebuilt: rebuild}

	if rebuild {
		if err := m.idx.Clear(ctx); err != nil {
			return st, err
		}
	}

	rows, err := m.idx.All(ctx)
	if err != nil {
		return st, err
	}

	have := make(map[string]Entry, len(rows))
	for i := range rows {
		have[rows[i].Key] = rows[i]
	}

	root, err := m.files.open()
	if err != nil {
		return st, err
	}

	seen := map[string]bool{}

	if root != nil {
		defer func() { _ = root.Close() }()

		if err := m.scanFlat(ctx, root, core.KindMemory, memoryDir, have, seen, &st); err != nil {
			return st, err
		}

		if err := m.scanFlat(ctx, root, core.KindPrompt, promptDir, have, seen, &st); err != nil {
			return st, err
		}

		if err := m.scanSkills(ctx, root, have, seen, &st); err != nil {
			return st, err
		}
	}

	for key := range have {
		if seen[key] {
			continue
		}

		if err := m.idx.Delete(ctx, key); err != nil {
			return st, err
		}

		st.Removed++
	}

	rows, err = m.idx.All(ctx)
	if err != nil {
		return st, err
	}

	for i := range rows {
		if rows[i].State == StateInvalid {
			st.Invalid++
		}
	}

	return st, nil
}

func (m *Manager) count(kind core.Kind, st *Stats) {
	switch kind {
	case core.KindMemory:
		st.Memory++
	case core.KindSkill:
		st.Skills++
	case core.KindPrompt:
		st.Prompts++
	}
}

func (m *Manager) ignore(st *Stats, rel string) {
	if len(st.Ignored) < maxIgnoredShown {
		st.Ignored = append(st.Ignored, printable(rel))
	}
}

// printable makes a file name safe to show: invalid UTF-8 and control or hidden
// characters become "?".
func printable(s string) string {
	s = strings.ToValidUTF8(s, "?")

	return strings.Map(func(r rune) rune {
		if core.HasHiddenText(string(r)) {
			return '?'
		}

		return r
	}, s)
}

func (m *Manager) scanFlat(
	ctx context.Context,
	root *os.Root,
	kind core.Kind,
	dir string,
	have map[string]Entry,
	seen map[string]bool,
	st *Stats,
) error {
	list, err := listDir(root, dir)
	if err != nil {
		return err
	}

	for _, e := range list {
		file := e.Name()

		name, ok := stem(file)
		if !ok {
			if !strings.HasPrefix(file, ".") {
				m.ignore(st, path.Join(dir, file))
			}

			continue
		}

		if !e.Type().IsRegular() {
			m.ignore(st, path.Join(dir, file))

			continue
		}

		if core.ValidName(name) != nil {
			m.ignore(st, path.Join(dir, file))

			continue
		}

		key := core.Key(kind, name)
		seen[key] = true
		m.count(kind, st)

		info, err := root.Lstat(path.Join(dir, file))
		if err != nil {
			return err
		}

		if row, ok := have[key]; ok && row.Sig == fileSig(info) {
			continue
		}

		st.Examined++

		entry, body := m.loadFlat(root, kind, name)
		if err := m.idx.Put(ctx, entry, body); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) scanSkills(
	ctx context.Context,
	root *os.Root,
	have map[string]Entry,
	seen map[string]bool,
	st *Stats,
) error {
	list, err := listDir(root, skillDir)
	if err != nil {
		return err
	}

	for _, e := range list {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		if !e.IsDir() || core.ValidName(name) != nil {
			m.ignore(st, path.Join(skillDir, name))

			continue
		}

		key := core.Key(core.KindSkill, name)
		seen[key] = true
		m.count(core.KindSkill, st)

		sig, serr := skillSignature(root, path.Join(skillDir, name))
		if row, ok := have[key]; ok && serr == nil && row.Sig == sig {
			continue
		}

		st.Examined++

		entry, body := m.loadSkillEntry(root, name)
		if err := m.idx.Put(ctx, entry, body); err != nil {
			return err
		}
	}

	return nil
}

// loadFlat reads and checks a note or a prompt file and builds its row and its
// searchable text. A file that does not pass is an invalid row, never skipped.
func (m *Manager) loadFlat(root *os.Root, kind core.Kind, name string) (Entry, string) {
	rel := entryPath(kind, name)
	entry := Entry{Key: core.Key(kind, name), Kind: kind, Name: name, Path: rel, State: StateOK}

	data, info, err := readSmall(root, rel, maxEntryFile)
	if err != nil {
		return entry.fail(err), ""
	}

	entry.Sig = fileSig(info)
	mtime := info.ModTime().UnixMilli()

	if !utf8.Valid(data) {
		return entry.fail(errors.New("the file is not valid UTF-8")), ""
	}

	switch kind {
	case core.KindMemory:
		mem, err := core.ParseMemory(string(data))
		if err == nil && mem.Name != name {
			err = fmt.Errorf("the name in the front matter (%s) is not the file name", printable(mem.Name))
		}

		if err == nil {
			err = mem.Validate()
		}

		if err != nil {
			return entry.fail(err), ""
		}

		entry.Description, entry.Scope, entry.Actor, entry.Kept, entry.Tags = mem.Description, mem.Scope, mem.Actor, mem.Kept, mem.Tags
		entry.CreatedMs, entry.UpdatedMs = timesOf(mem.Created, mem.Updated, mtime)

		return entry, mem.Body
	case core.KindPrompt:
		p, err := core.ParsePrompt(string(data))
		if err == nil && p.Name != name {
			err = fmt.Errorf("the name in the front matter (%s) is not the file name", printable(p.Name))
		}

		if err == nil {
			err = p.Validate()
		}

		if err != nil {
			return entry.fail(err), ""
		}

		entry.Description, entry.Actor, entry.Tags = p.Description, p.Actor, p.Tags
		entry.CreatedMs, entry.UpdatedMs = timesOf(p.Created, p.Updated, mtime)

		return entry, p.Body
	case core.KindSkill:
	}

	return entry.fail(errors.New("unknown kind")), ""
}

func (e Entry) fail(err error) Entry {
	e.State = StateInvalid
	e.Error = printable(err.Error())
	e.Description, e.Scope, e.Origin, e.Actor, e.Tags = "", "", "", "", nil
	e.Reviewed, e.Kept = false, false

	return e
}

// timesOf reads the created and updated times of a file, with the modification
// time of the file where it records none.
func timesOf(created, updated string, mtime int64) (createdMs, updatedMs int64) {
	parse := func(s string, fallback int64) int64 {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UnixMilli()
		}

		return fallback
	}

	createdMs = parse(created, mtime)

	return createdMs, parse(updated, createdMs)
}

// loadSkillEntry reads a skill folder and builds its row and its searchable text.
func (m *Manager) loadSkillEntry(root *os.Root, name string) (Entry, string) {
	entry, body, _ := m.loadSkill(root, name)

	return entry, body
}

// loadSkill reads a skill folder: its row, its searchable text, the skill and
// its files. The row is an invalid one, and the skill empty, when the folder
// cannot be read as a skill.
func (m *Manager) loadSkill(root *os.Root, name string) (Entry, string, core.Skill) {
	rel := entryPath(core.KindSkill, name)
	entry := Entry{Key: core.Key(core.KindSkill, name), Kind: core.KindSkill, Name: name, Path: rel, State: StateOK}

	all, sig, err := readSkillFiles(root, rel)
	if err != nil {
		return entry.fail(err), "", core.Skill{}
	}

	entry.Sig = sig

	skill, err := core.ParseSkill(all)
	if err != nil {
		return entry.fail(err), "", core.Skill{}
	}

	findings := core.LintSkill(name, all)

	entry.Description = skill.Description
	entry.Origin = skill.Origin()
	// A file can claim to be built in; it is, only while it is the text the product installed (SDD-018 section 16).
	if entry.Origin == core.OriginBuiltIn && skill.ContentHash() != builtInHash(root, name) {
		entry.Origin = core.OriginUser
	}
	entry.Reviewed = m.isReviewed(&skill) && !core.HasError(findings)
	entry.LintErrors, entry.LintWarnings = countFindings(findings)
	entry.HasScripts = slices.ContainsFunc(findings, func(f core.Finding) bool { return f.Code == core.LintScript })

	if info, err := root.Lstat(path.Join(rel, core.SkillFile)); err == nil {
		entry.CreatedMs, entry.UpdatedMs = info.ModTime().UnixMilli(), info.ModTime().UnixMilli()
	}

	return entry, searchText(skill), skill
}

func countFindings(findings []core.Finding) (errs, warns int) {
	for _, f := range findings {
		if f.Severity == core.SeverityError {
			errs++
		} else {
			warns++
		}
	}

	return errs, warns
}

// searchText is the body of a skill and the text of its markdown files, up to a
// limit, for the full-text index.
func searchText(s core.Skill) string {
	var b strings.Builder

	b.WriteString(s.Body)

	for _, f := range s.Files {
		if b.Len() >= maxSearchText {
			break
		}

		if (strings.HasSuffix(f.Path, ".md") || strings.HasSuffix(f.Path, ".txt")) && utf8.Valid(f.Data) {
			b.WriteString("\n")
			b.Write(f.Data)
		}
	}

	return truncateText(b.String(), maxSearchText)
}

func truncateText(s string, limit int) string {
	if len(s) <= limit {
		return s
	}

	return strings.ToValidUTF8(s[:limit], "")
}

// refresh scans, then returns the rows. Every read of the library starts here,
// so a file edited outside the product is found at the next call (RF-391).
func (m *Manager) refresh(ctx context.Context) ([]Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := m.scan(ctx, false); err != nil {
		return nil, err
	}

	return m.idx.All(ctx)
}

// Filter narrows a list: every member that is set must match.
type Filter struct {
	Kinds             []core.Kind
	Scope             string // memory: "workspace" or a project id
	Tags              []string
	Origin            string
	IncludeUnreviewed bool // skills that are not reviewed, and notes no person kept
	// ProjectID is the project a key is limited to: it sees the notes of the workspace and of
	// that project, and every skill and prompt.
	ProjectID string
}

func (f Filter) match(e Entry) bool {
	if len(f.Kinds) > 0 && !slices.Contains(f.Kinds, e.Kind) {
		return false
	}

	if !visibleTo(&e, f.ProjectID) {
		return false
	}

	if f.Scope != "" && e.Kind == core.KindMemory && e.Scope != f.Scope {
		return false
	}

	if f.Origin != "" && e.Origin != f.Origin {
		return false
	}

	for _, t := range f.Tags {
		if !slices.Contains(e.Tags, t) {
			return false
		}
	}

	if !f.IncludeUnreviewed && e.State == StateOK &&
		(e.Kind == core.KindSkill && !e.Reviewed || e.Kind == core.KindMemory && !e.Kept) {
		return false
	}

	return true
}

// List returns the entries of one kind that pass the filter, by name.
func (m *Manager) List(ctx context.Context, kind core.Kind, f Filter) ([]Entry, error) {
	rows, err := m.refresh(ctx)
	if err != nil {
		return nil, err
	}

	f.Kinds = []core.Kind{kind}

	out := make([]Entry, 0, len(rows))

	for i := range rows {
		if rows[i].Kind == kind && f.match(rows[i]) {
			out = append(out, rows[i])
		}
	}

	slices.SortFunc(out, func(a, b Entry) int { return strings.Compare(a.Name, b.Name) })

	return out, nil
}

// Entry returns the row of one entry.
func (m *Manager) Entry(ctx context.Context, kind core.Kind, name string) (Entry, error) {
	if err := core.ValidName(name); err != nil {
		return Entry{}, invalid("name", "%v", err)
	}

	rows, err := m.refresh(ctx)
	if err != nil {
		return Entry{}, err
	}

	key := core.Key(kind, name)

	for i := range rows {
		if rows[i].Key == key {
			return rows[i], nil
		}
	}

	return Entry{}, ErrNotFound
}

// putRow reads the entry that was just written and records it in the index.
// The caller holds the lock.
func (m *Manager) putRow(ctx context.Context, root *os.Root, kind core.Kind, name string) error {
	var (
		entry Entry
		body  string
	)

	if kind == core.KindSkill {
		entry, body = m.loadSkillEntry(root, name)
	} else {
		entry, body = m.loadFlat(root, kind, name)
	}

	return m.idx.Put(ctx, entry, body)
}

// isMissing reports whether err means that the file or folder does not exist.
func isMissing(err error) bool { return errors.Is(err, fs.ErrNotExist) }
