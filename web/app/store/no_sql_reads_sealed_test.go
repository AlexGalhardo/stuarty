package store

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// allowedSQLReads are the statements that name a sealed column in a position SQL reads, and why that is safe: each
// one runs only where the column holds clear text because there is no account.
var allowedSQLReads = map[string]string{
	"COALESCE(i.description, '')": "the index of the asset library of a workspace with no account; with one the term is ''",
}

// A sealed column is never read by SQL: no WHERE, ORDER BY, GROUP BY, HAVING, join condition, expression or index
// names one (ADR-0053, SDD-005 section 5.17.2). The value is opened by the repository after the read, so nothing is
// decrypted in SQL. A finding here is a STOP of epic 92: the column cannot be sealed.
func TestNoSQLReadsASealedColumn(t *testing.T) {
	// The first stage is epic 57's: its one SQL read, json_extract over jobs.input, is of the kinds that hold no words
	// (SDD-005 section 5.17.2). The second stage has none.
	cells := sealedCellsV2

	reads := regexp.MustCompile(
		`(?is)\b(where|order\s+by|group\s+by|having|on|json_extract|json_valid|length|substr|like|match|coalesce|lower|upper|trim|cast)\b`,
	)

	sources := []string{"."}

	var files []string

	for _, dir := range sources {
		list, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}

		for _, f := range list {
			if !strings.HasSuffix(f, "_test.go") {
				files = append(files, f)
			}
		}
	}

	checked := 0

	for _, file := range files {
		fset := token.NewFileSet()

		tree, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}

		ast.Inspect(tree, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}

			text, err := strconv.Unquote(lit.Value)
			if err != nil ||
				!strings.Contains(strings.ToUpper(text), "SELECT") && !strings.Contains(strings.ToUpper(text), "UPDATE") &&
					!strings.Contains(strings.ToUpper(text), "DELETE") &&
					!strings.Contains(strings.ToUpper(text), "INSERT") {
				return true
			}

			checked++

			// What SQL reads: from the first WHERE / ORDER BY / GROUP BY / HAVING on, up to a conflict clause (the
			// SET list of an upsert writes the column), plus every function call over a column in the select list.
			upper := strings.ToUpper(text)
			from := -1

			for _, kw := range []string{" WHERE ", " ORDER BY ", " GROUP BY ", " HAVING ", "\nWHERE ", "\nORDER BY "} {
				if i := strings.Index(upper, kw); i >= 0 && (from < 0 || i < from) {
					from = i
				}
			}

			tail := ""
			if from >= 0 {
				tail = text[from:]
				if i := strings.Index(strings.ToUpper(tail), "ON CONFLICT"); i >= 0 {
					tail = tail[:i]
				}
			}

			for _, c := range cells {
				if !strings.Contains(text, c.table) {
					continue
				}

				word := regexp.MustCompile(`(?i)\b` + c.column + `\b`)

				if word.MatchString(tail) {
					t.Errorf(
						"%s: SQL reads %s.%s in a condition or an order: %q",
						fset.Position(lit.Pos()),
						c.table,
						c.column,
						trim(text),
					)
				}

				call := regexp.MustCompile(`(?i)\w+\(\s*(\w+\.)?` + c.column + `\b`)
				if m := call.FindString(text); m != "" && reads.MatchString(m) {
					if _, ok := allowedSQLReads[m]; !ok {
						t.Errorf(
							"%s: SQL applies an expression to %s.%s: %q",
							fset.Position(lit.Pos()),
							c.table,
							c.column,
							m,
						)
					}
				}
			}

			return true
		})
	}

	if checked < 50 {
		t.Fatalf("only %d statements were checked: the scan is blind", checked)
	}
}

func trim(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 120 {
		return s[:120]
	}

	return s
}

// No index of the schema names a sealed column either: the migrations and the guards are read as text.
func TestNoIndexNamesASealedColumn(t *testing.T) {
	root := filepath.Join("..", "..", "database", "migrations")

	files, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil {
		t.Fatal(err)
	}

	create := regexp.MustCompile(`(?is)CREATE\s+(UNIQUE\s+)?INDEX\s+\w+\s+ON\s+(\w+)\s*\(([^)]*)\)`)

	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}

		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}

		for _, m := range create.FindAllStringSubmatch(string(raw), -1) {
			for _, c := range allSealedCells() {
				if m[2] != c.table {
					continue
				}

				for col := range strings.SplitSeq(m[3], ",") {
					if strings.EqualFold(strings.Fields(col)[0], c.column) {
						t.Errorf("%s: an index on %s names the sealed column %s", filepath.Base(f), c.table, c.column)
					}
				}
			}
		}
	}
}
