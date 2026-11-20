package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// threat is one row of the table of SDD-007 section 2.
type threat struct {
	id         string
	mitigation string
	tests      []string
}

var (
	rowID    = regexp.MustCompile(`^T-\d{2,3}$`)
	spanCode = regexp.MustCompile("`([^`]+)`")
	testFunc = regexp.MustCompile(`(?m)^func (TestSecurity_T\d{2,3}_\w+|Fuzz\w+)\(`)
	secTest  = regexp.MustCompile(`^TestSecurity_T(\d{2,3})_`)
	method   = regexp.MustCompile(`^\t([A-Z]\w*)\(ctx context\.Context`)
)

// section returns the text from the heading that starts with from up to the
// heading that starts with to.
func section(doc, from, to string) (string, error) {
	_, rest, found := strings.Cut(doc, "\n"+from)
	if !found {
		return "", fmt.Errorf("no section %q", from)
	}

	body, _, _ := strings.Cut(rest, "\n"+to)

	return body, nil
}

// parseThreats reads the rows of the threat table.
func parseThreats(doc string) ([]threat, error) {
	body, err := section(doc, "## 2. Threat model", "## 3.")
	if err != nil {
		return nil, err
	}

	var out []threat

	for line := range strings.SplitSeq(body, "\n") {
		if !strings.HasPrefix(line, "| T-") {
			continue
		}

		cells := strings.Split(strings.TrimSpace(line), " | ")
		if len(cells) != 6 {
			return nil, fmt.Errorf("a threat row has %d cells, want 6: %.60s", len(cells), line)
		}

		id := strings.TrimPrefix(cells[0], "| ")
		if !rowID.MatchString(id) {
			return nil, fmt.Errorf("a threat row has the id %q", id)
		}

		t := threat{id: id, mitigation: strings.TrimSpace(cells[4])}

		for _, m := range spanCode.FindAllStringSubmatch(cells[5], -1) {
			if strings.HasPrefix(m[1], "TestSecurity_") || strings.HasPrefix(m[1], "Fuzz") {
				t.tests = append(t.tests, m[1])
			}
		}

		out = append(out, t)
	}

	if len(out) == 0 {
		return nil, errors.New("the threat table has no rows")
	}

	return out, nil
}

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true, "coverage": true,
}

// testsOf lists the security tests and fuzz targets declared in the repository.
func testsOf(root string) (map[string]bool, error) {
	have := map[string]bool{}

	// os.Root keeps every read inside the repository, symbolic links included.
	repo, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}

	defer func() { _ = repo.Close() }()

	err = fs.WalkDir(repo.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if skipDirs[d.Name()] {
				return fs.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		raw, err := repo.ReadFile(path)
		if err != nil {
			return err
		}

		for _, m := range testFunc.FindAllStringSubmatch(string(raw), -1) {
			have[m[1]] = true
		}

		return nil
	})

	return have, err
}

// operationsOf lists the methods of the service.Service interface.
func operationsOf(root string) ([]string, error) {
	raw, err := os.ReadFile(filepath.Join(root, "service", "service.go"))
	if err != nil {
		return nil, err
	}

	_, rest, found := strings.Cut(string(raw), "type Service interface {")
	if !found {
		return nil, errors.New("service/service.go has no Service interface")
	}

	body, _, _ := strings.Cut(rest, "\n}")

	var ops []string

	for line := range strings.SplitSeq(body, "\n") {
		if m := method.FindStringSubmatch(line); m != nil {
			ops = append(ops, m[1])
		}
	}

	return ops, nil
}

// check returns the problems of the repository at root against the security model.
func check(root, sddPath string) ([]string, error) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(sddPath)))
	if err != nil {
		return nil, err
	}

	doc := strings.ReplaceAll(string(raw), "\r\n", "\n")

	threats, err := parseThreats(doc)
	if err != nil {
		return nil, err
	}

	have, err := testsOf(root)
	if err != nil {
		return nil, err
	}

	var problems []string

	named := map[string]bool{}

	for _, t := range threats {
		for _, name := range t.tests {
			named[name] = true
		}

		problems = append(problems, threatProblems(t, have)...)
	}

	// A security test no row names has no threat behind it; it is a defect of the
	// table or of the name.
	for name := range have {
		if secTest.MatchString(name) && !named[name] {
			problems = append(problems, name+" is named by no threat row")
		}
	}

	skips, err := skipsOf(root)
	if err != nil {
		return nil, err
	}

	problems = append(problems, skips...)

	ops, err := operationsOf(root)
	if err != nil {
		return nil, err
	}

	matrix, err := section(doc, "## 5. Scope matrix", "## 6.")
	if err != nil {
		return nil, err
	}

	for _, op := range ops {
		if !strings.Contains(matrix, "`"+op+"`") {
			problems = append(
				problems,
				fmt.Sprintf("the operation %s is missing from the scope matrix of section 5", op),
			)
		}
	}

	slices.Sort(problems)

	return problems, nil
}

func threatProblems(t threat, have map[string]bool) []string {
	var out []string

	if t.mitigation == "" || t.mitigation == "-" {
		out = append(out, t.id+" has no mitigation")
	}

	if len(t.tests) == 0 {
		return append(out, t.id+" names no test")
	}

	covered := false

	for _, name := range t.tests {
		if m := secTest.FindStringSubmatch(name); len(m) == 2 && "T-"+m[1] != t.id {
			out = append(out, fmt.Sprintf("%s is named by the row %s", name, t.id))
		}

		if !have[name] {
			out = append(out, fmt.Sprintf("%s: the test %s does not exist", t.id, name))

			continue
		}

		covered = true
	}

	if !covered {
		out = append(out, t.id+" has no test that exists")
	}

	return out
}
