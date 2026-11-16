// Package results_test checks the shape of the screen reader result files
// (SDD-012 section 18.4 item 4, P3-QA-002): the template and every dated file
// have one table row per task of the checklist, and a finished file has no
// open row and no failure without a card number.
package results_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	idPattern   = regexp.MustCompile(`^SR-\d{2}$`)
	namePattern = regexp.MustCompile(
		`^screen-reader-\d{4}-\d{2}-\d{2}-(nvda-firefox|nvda-chrome|narrator-edge|voiceover-safari)\.md$`,
	)
	resultPattern = regexp.MustCompile(`^(pass|n/a|fail #\d+)$`)
)

func readLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
}

func cells(line string) []string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// tableRows returns the cells of the data rows of the first table whose header
// starts with the given first column, and fails when the header is missing.
func tableRows(t *testing.T, path, header string) [][]string {
	t.Helper()
	lines := readLines(t, path)
	for i, line := range lines {
		if !strings.HasPrefix(line, "|") || cells(line)[0] != header {
			continue
		}
		var rows [][]string
		for _, row := range lines[i+2:] {
			if !strings.HasPrefix(row, "|") {
				break
			}
			rows = append(rows, cells(row))
		}
		return rows
	}
	t.Fatalf("%s: no table with the header %q", path, header)
	return nil
}

func checklistIDs(t *testing.T) []string {
	t.Helper()
	var ids []string
	for _, row := range tableRows(t, filepath.Join("..", "screen-reader-checklist.md"), "Id") {
		if len(row) != 9 {
			t.Fatalf("checklist row %q has %d cells, want 9", row[0], len(row))
		}
		if !idPattern.MatchString(row[0]) {
			t.Fatalf("checklist id %q is not SR-NN", row[0])
		}
		ids = append(ids, row[0])
	}
	if len(ids) == 0 {
		t.Fatal("the checklist has no task rows")
	}
	return ids
}

func checkFile(t *testing.T, path string, ids []string, finished bool) {
	t.Helper()
	rows := tableRows(t, path, "Id")
	if len(rows) != len(ids) {
		t.Fatalf("%s: %d rows, want %d", path, len(rows), len(ids))
	}
	for i, row := range rows {
		if len(row) != 3 {
			t.Errorf("%s: row %q has %d cells, want 3", path, row[0], len(row))
			continue
		}
		if row[0] != ids[i] {
			t.Errorf("%s: row %d is %q, want %q", path, i+1, row[0], ids[i])
		}
		switch {
		case finished && !resultPattern.MatchString(row[1]):
			t.Errorf("%s: %s result %q must be pass, n/a or fail #<card>", path, row[0], row[1])
		case !finished && row[1] != "todo":
			t.Errorf("%s: %s result %q must be todo in the template", path, row[0], row[1])
		}
		if row[1] == "n/a" && row[2] == "" {
			t.Errorf("%s: %s is n/a without a reason", path, row[0])
		}
	}
}

func TestTemplateHasOneOpenRowPerChecklistTask(t *testing.T) {
	checkFile(t, "TEMPLATE.md", checklistIDs(t), false)
}

func TestResultFilesAreNamedAndComplete(t *testing.T) {
	ids := checklistIDs(t)
	files, err := filepath.Glob("screen-reader-*.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if !namePattern.MatchString(file) {
			t.Errorf("%s: want screen-reader-<YYYY-MM-DD>-<reader>.md", file)
			continue
		}
		checkFile(t, file, ids, true)
	}
}

func TestCheckerRejectsBrokenRows(t *testing.T) {
	for _, bad := range []string{"todo", "fail", "fail #", "ok", ""} {
		if resultPattern.MatchString(bad) {
			t.Errorf("%q must not be a finished result", bad)
		}
	}
	for _, good := range []string{"pass", "n/a", "fail #123"} {
		if !resultPattern.MatchString(good) {
			t.Errorf("%q must be a finished result", good)
		}
	}
}
