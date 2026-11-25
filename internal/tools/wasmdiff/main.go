// Command wasmdiff compares two WebAssembly modules section by section (epic 93). It prints the size
// and SHA-256 of each section, the first byte where the modules differ and the section, function
// body or data segment that holds it, and the text of the differing data segments (panic locations
// and source paths are the usual cause of a module that is not the same on two systems).
//
//	go run ./internal/tools/wasmdiff a.wasm b.wasm
//
// The exit status is 0 when the modules are byte for byte equal, 1 when they differ and 2 on a
// usage or format error. It uses only the standard library.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(stderr, "usage: wasmdiff <a.wasm> <b.wasm>")
		return 2
	}
	a, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintln(stderr, "wasmdiff:", err)
		return 2
	}
	b, err := os.ReadFile(args[1])
	if err != nil {
		fmt.Fprintln(stderr, "wasmdiff:", err)
		return 2
	}
	equal, err := compare(a, b, stdout)
	if err != nil {
		fmt.Fprintln(stderr, "wasmdiff:", err)
		return 2
	}
	if equal {
		return 0
	}
	return 1
}

// section is one section of a module: its id, the name of a custom section, and where its payload is.
type section struct {
	id         byte
	name       string
	start, end int // the payload, in the file
}

func (s section) label() string {
	if s.id == 0 {
		return "custom:" + s.name
	}
	if n, ok := sectionNames[s.id]; ok {
		return n
	}
	return fmt.Sprintf("section-%d", s.id)
}

var sectionNames = map[byte]string{
	1: "type", 2: "import", 3: "function", 4: "table", 5: "memory", 6: "global", 7: "export",
	8: "start", 9: "element", 10: "code", 11: "data", 12: "datacount", 13: "tag",
}

type reader struct {
	b   []byte
	pos int
}

var errShort = errors.New("truncated module")

func (r *reader) u32() (uint32, error) {
	var v uint64
	for shift := uint(0); shift < 35; shift += 7 {
		if r.pos >= len(r.b) {
			return 0, errShort
		}
		c := r.b[r.pos]
		r.pos++
		v |= uint64(c&0x7f) << shift
		if c&0x80 == 0 {
			return uint32(v), nil
		}
	}
	return 0, errors.New("LEB128 too long")
}

func (r *reader) skipLEB() error {
	for r.pos < len(r.b) {
		c := r.b[r.pos]
		r.pos++
		if c&0x80 == 0 {
			return nil
		}
	}
	return errShort
}

func (r *reader) bytes(n uint32) ([]byte, error) {
	if uint64(r.pos)+uint64(n) > uint64(len(r.b)) {
		return nil, errShort
	}
	out := r.b[r.pos : r.pos+int(n)]
	r.pos += int(n)
	return out, nil
}

func parseSections(m []byte) ([]section, error) {
	if len(m) < 8 || !bytes.Equal(m[:4], []byte{0, 'a', 's', 'm'}) {
		return nil, errors.New("not a WebAssembly module")
	}
	r := &reader{b: m, pos: 8}
	var out []section
	for r.pos < len(m) {
		id := m[r.pos]
		r.pos++
		size, err := r.u32()
		if err != nil {
			return nil, err
		}
		s := section{id: id, start: r.pos, end: r.pos + int(size)}
		if s.end > len(m) {
			return nil, errShort
		}
		if id == 0 {
			sr := &reader{b: m[:s.end], pos: s.start}
			n, err := sr.u32()
			if err != nil {
				return nil, err
			}
			nb, err := sr.bytes(n)
			if err != nil {
				return nil, err
			}
			s.name = string(nb)
		}
		out = append(out, s)
		r.pos = s.end
	}
	return out, nil
}

// entry is one function body or data segment: its bytes and where they are in the file.
type entry struct {
	start int
	data  []byte
}

func codeEntries(m []byte, s section) ([]entry, error) {
	r := &reader{b: m[:s.end], pos: s.start}
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	var out []entry
	for i := uint32(0); i < n; i++ {
		size, err := r.u32()
		if err != nil {
			return nil, err
		}
		start := r.pos
		body, err := r.bytes(size)
		if err != nil {
			return nil, err
		}
		out = append(out, entry{start, body})
	}
	return out, nil
}

func dataEntries(m []byte, s section) ([]entry, error) {
	r := &reader{b: m[:s.end], pos: s.start}
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	var out []entry
	for i := uint32(0); i < n; i++ {
		flag, err := r.u32()
		if err != nil {
			return nil, err
		}
		if flag == 2 {
			if _, err := r.u32(); err != nil {
				return nil, err
			}
		}
		if flag == 0 || flag == 2 {
			if err := skipConstExpr(r); err != nil {
				return nil, err
			}
		}
		size, err := r.u32()
		if err != nil {
			return nil, err
		}
		start := r.pos
		data, err := r.bytes(size)
		if err != nil {
			return nil, err
		}
		out = append(out, entry{start, data})
	}
	return out, nil
}

// skipConstExpr skips the offset expression of an active data segment: i32.const, i64.const or
// global.get, then end.
func skipConstExpr(r *reader) error {
	for r.pos < len(r.b) {
		op := r.b[r.pos]
		r.pos++
		switch op {
		case 0x0b:
			return nil
		case 0x41, 0x42, 0x23:
			if err := r.skipLEB(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported opcode 0x%02x in a data segment offset", op)
		}
	}
	return errShort
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func compare(a, b []byte, w io.Writer) (bool, error) {
	sa, err := parseSections(a)
	if err != nil {
		return false, fmt.Errorf("first module: %w", err)
	}
	sb, err := parseSections(b)
	if err != nil {
		return false, fmt.Errorf("second module: %w", err)
	}
	fmt.Fprintf(w, "A: %d bytes, sha256 %s\nB: %d bytes, sha256 %s\n\n", len(a), sum(a), len(b), sum(b))
	fmt.Fprintf(w, "%-22s %9s %9s  %s\n", "section", "A size", "B size", "payload")
	for i := 0; i < len(sa) || i < len(sb); i++ {
		var la, lb string
		var na, nb int
		var pa, pb []byte
		if i < len(sa) {
			la, na, pa = sa[i].label(), sa[i].end-sa[i].start, a[sa[i].start:sa[i].end]
		}
		if i < len(sb) {
			lb, nb, pb = sb[i].label(), sb[i].end-sb[i].start, b[sb[i].start:sb[i].end]
		}
		label := la
		if la != lb {
			label = la + " | " + lb
		}
		verdict := "equal"
		if !bytes.Equal(pa, pb) || la != lb {
			verdict = "DIFFERS"
		}
		fmt.Fprintf(w, "%-22s %9d %9d  %s (A %.12s, B %.12s)\n", label, na, nb, verdict, sumOrDash(pa), sumOrDash(pb))
	}
	if bytes.Equal(a, b) {
		fmt.Fprintln(w, "\nthe modules are identical")
		return true, nil
	}

	off := 0
	for off < len(a) && off < len(b) && a[off] == b[off] {
		off++
	}
	fmt.Fprintf(w, "\nfirst difference at byte %d (0x%x)\n", off, off)
	for i, s := range sa {
		if off >= s.start-1 && off < s.end {
			fmt.Fprintf(w, "in A section %d %s (payload bytes %d..%d)\n", i, s.label(), s.start, s.end)
		}
	}
	for _, s := range sa {
		if i := indexSection(sb, s); i >= 0 && !bytes.Equal(a[s.start:s.end], b[sb[i].start:sb[i].end]) {
			switch s.id {
			case 10:
				describe(w, "function body", a, b, s, sb[i], codeEntries, false)
			case 11:
				describe(w, "data segment", a, b, s, sb[i], dataEntries, true)
			}
		}
	}
	return false, nil
}

func sumOrDash(p []byte) string {
	if p == nil {
		return "-"
	}
	return sum(p)
}

func indexSection(list []section, s section) int {
	for i, o := range list {
		if o.id == s.id && o.name == s.name {
			return i
		}
	}
	return -1
}

func describe(w io.Writer, kind string, a, b []byte, sa, sb section, parse func([]byte, section) ([]entry, error), text bool) {
	ea, err := parse(a, sa)
	if err != nil {
		fmt.Fprintf(w, "A %s section: %v\n", kind, err)
		return
	}
	eb, err := parse(b, sb)
	if err != nil {
		fmt.Fprintf(w, "B %s section: %v\n", kind, err)
		return
	}
	fmt.Fprintf(w, "\n%ss: A has %d, B has %d\n", kind, len(ea), len(eb))
	var differing []int
	for i := 0; i < len(ea) && i < len(eb); i++ {
		if !bytes.Equal(ea[i].data, eb[i].data) {
			differing = append(differing, i)
		}
	}
	fmt.Fprintf(w, "%d %ss differ by index\n", len(differing), kind)
	if len(differing) > 0 {
		i := differing[0]
		d := 0
		for d < len(ea[i].data) && d < len(eb[i].data) && ea[i].data[d] == eb[i].data[d] {
			d++
		}
		fmt.Fprintf(w, "first: %s %d, byte %d inside it (file byte %d in A, %d in B)\n", kind, i, d, ea[i].start+d, eb[i].start+d)
	}
	if len(ea) == len(eb) && len(differing) > 0 && setOf(ea) == setOf(eb) {
		fmt.Fprintf(w, "the %ss are the same set in a different order\n", kind)
	}
	if !text {
		return
	}
	for n, i := range differing {
		if n == 5 {
			fmt.Fprintf(w, "... %d more\n", len(differing)-n)
			break
		}
		fmt.Fprintf(w, "segment %d\n  A: %s\n  B: %s\n", i, printable(ea[i].data), printable(eb[i].data))
	}
}

func setOf(es []entry) string {
	h := make([]string, len(es))
	for i, e := range es {
		h[i] = sum(e.data)
	}
	sort.Strings(h)
	return strings.Join(h, ",")
}

// printable shows the readable runs of a segment, so a source path in it can be read.
func printable(b []byte) string {
	var sb strings.Builder
	run := 0
	for _, c := range b {
		if c >= 0x20 && c < 0x7f {
			sb.WriteByte(c)
			run++
		} else if run > 0 {
			sb.WriteString(" | ")
			run = 0
		}
		if sb.Len() > 300 {
			sb.WriteString("...")
			break
		}
	}
	return fmt.Sprintf("%q", sb.String())
}
