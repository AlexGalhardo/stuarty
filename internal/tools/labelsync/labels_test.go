package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    []label
		wantErr string // substring; empty means valid
	}{
		{
			name: "valid with comment and blank line",
			in:   "# type\n\nbug|d73a4a|Something broke\n",
			want: []label{{Name: "bug", Color: "d73a4a", Description: "Something broke"}},
		},
		{
			name: "description with colon and spaces",
			in:   "priority:p1|b60205|Fix: before the next release\n",
			want: []label{{Name: "priority:p1", Color: "b60205", Description: "Fix: before the next release"}},
		},
		{name: "two fields", in: "bug|d73a4a\n", wantErr: "line 1"},
		{name: "four fields", in: "# c\nbug|d73a4a|a|b\n", wantErr: "line 2"},
		{name: "invalid color letters", in: "bug|zzzzzz|x\n", wantErr: "line 1"},
		{name: "invalid color length", in: "bug|12345|x\n", wantErr: "color"},
		{name: "empty name", in: "|d73a4a|x\n", wantErr: "name"},
		{name: "duplicate name", in: "bug|d73a4a|x\nbug|d73a4a|y\n", wantErr: "line 2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parse(strings.NewReader(tt.in))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parse() error = %v, want substring %q", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("parse() error = %v", err)
			}

			if !slices.Equal(tt.want, got) {
				t.Errorf("parse() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCommands(t *testing.T) {
	t.Parallel()

	l := label{Name: "area:orm", Color: "c5def5", Description: "orm stuff"}
	want := []string{"label", "create", "area:orm", "--color", "c5def5", "--description", "orm stuff", "--force"}

	if got := args(l); !slices.Equal(want, got) {
		t.Errorf("args() = %v, want %v", got, want)
	}
}

// repoFile reads a file of the repository, relative to its root.
func repoFile(t *testing.T, rel ...string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join(append([]string{"..", "..", ".."}, rel...)...))
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}

func areaLabels(t *testing.T) []string {
	t.Helper()

	labels, err := parse(strings.NewReader(repoFile(t, ".github", "labels.txt")))
	if err != nil {
		t.Fatal(err)
	}

	var areas []string

	for _, l := range labels {
		if scope, ok := strings.CutPrefix(l.Name, "area:"); ok {
			areas = append(areas, scope)
		}
	}

	slices.Sort(areas)

	return areas
}

// The area labels are the commit scopes: the pull request title check
// (.github/workflows/pr-title.yml) and the table of SDD-009 section 6.2 name
// the same list, so a scope added in one place must be added in all three.
func TestAreaLabelsAreTheCommitScopes(t *testing.T) {
	t.Parallel()

	areas := areaLabels(t)

	m := regexp.MustCompile(`scopes='([^']+)'`).FindStringSubmatch(repoFile(t, ".github", "workflows", "pr-title.yml"))
	if m == nil {
		t.Fatal("pr-title.yml: no scopes='...' line")
	}

	// A pattern entry such as sdd-[0-9]{3} is a spelling of another scope
	// (SDD-009 section 6.2), not a scope with a label of its own.
	titleScopes := slices.DeleteFunc(strings.Split(m[1], "|"), func(s string) bool {
		return strings.ContainsAny(s, "[]{}")
	})
	slices.Sort(titleScopes)

	if !slices.Equal(areas, titleScopes) {
		t.Errorf("area: labels = %v\npr-title.yml scopes = %v", areas, titleScopes)
	}

	sdd := repoFile(t, "docs", "sdds", "009-releases-and-ci-cd.md")
	_, table, _ := strings.Cut(sdd, "### 6.2 Allowed scopes")
	table, _, _ = strings.Cut(table, "### 6.3")

	var sddScopes []string

	for _, row := range regexp.MustCompile("(?m)^\\| `([a-z-]+)` \\|").FindAllStringSubmatch(table, -1) {
		sddScopes = append(sddScopes, row[1])
	}

	slices.Sort(sddScopes)

	if !slices.Equal(areas, sddScopes) {
		t.Errorf("area: labels = %v\nSDD-009 section 6.2 scopes = %v", areas, sddScopes)
	}
}
