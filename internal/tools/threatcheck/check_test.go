package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sdd = "## 1. Intro\n\n## 2. Threat model\n\n" +
	"| ID | Threat | How | Impact | Mitigation | Test |\n| --- | --- | --- | --- | --- | --- |\n" +
	"| T-01 | a | b | c | a mitigation | `TestSecurity_T01_One`, `TestSecurity_T01_Two` |\n" +
	"| T-02 | a | b | c | a mitigation | `TestSecurity_T02_Missing` |\n" +
	"\n## 3. Local mode\n\n## 5. Scope matrix\n\n| `ProjectGet` | x |\n\n## 6. Next\n"

const serviceGo = "package service\n\ntype Service interface {\n" +
	"\tProjectGet(ctx context.Context, req R) (*Res, error)\n" +
	"\tExportFile(ctx context.Context, req R) (*Res, error)\n}\n"

func repo(t *testing.T, tests string) string {
	t.Helper()

	root := t.TempDir()
	put := func(rel, content string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	put("docs/sdds/007-security-model.md", sdd)
	put("service/service.go", serviceGo)
	put("pkg/sec_test.go", "package pkg\n\n"+tests)
	put("web/node_modules/x/x_test.go", "package x\n\nfunc TestSecurity_T09_Ignored(t *testing.T) {}\n")

	return root
}

func TestEveryProblemIsReported(t *testing.T) {
	t.Parallel()

	root := repo(t, "func TestSecurity_T01_One(t *testing.T) {}\nfunc TestSecurity_T01_Extra(t *testing.T) {}\n")

	problems, err := check(root, "docs/sdds/007-security-model.md")
	if err != nil {
		t.Fatal(err)
	}

	got := strings.Join(problems, "\n")

	for _, want := range []string{
		"T-01: the test TestSecurity_T01_Two does not exist",
		"T-02: the test TestSecurity_T02_Missing does not exist",
		"T-02 has no test that exists",
		"TestSecurity_T01_Extra is named by no threat row",
		"the operation ExportFile is missing from the scope matrix",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no problem %q in:\n%s", want, got)
		}
	}

	// The test of node_modules is not read, and a documented operation is not reported.
	for _, unwanted := range []string{"T09", "ProjectGet"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("%q is reported:\n%s", unwanted, got)
		}
	}
}

func TestCleanTreeExitsZero(t *testing.T) {
	t.Parallel()

	root := repo(t, "func TestSecurity_T01_One(t *testing.T) {}\nfunc TestSecurity_T01_Two(t *testing.T) {}\n"+
		"func TestSecurity_T02_Missing(t *testing.T) {}\n")
	put := filepath.Join(root, "service", "service.go")

	if err := os.WriteFile(
		put,
		[]byte(strings.Replace(serviceGo, "\tExportFile(ctx context.Context, req R) (*Res, error)\n", "", 1)),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-root", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
}

func TestMalformedTableIsAnError(t *testing.T) {
	t.Parallel()

	if _, err := parseThreats("## 2. Threat model\n\n| T-01 | only | three |\n\n## 3. x\n"); err == nil {
		t.Error("a row with the wrong number of cells was accepted")
	}

	if _, err := parseThreats("no table here"); err == nil {
		t.Error("a document with no threat section was accepted")
	}
}

func TestTheRealModelParses(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../../docs/sdds/007-security-model.md")
	if err != nil {
		t.Fatal(err)
	}

	threats, err := parseThreats(strings.ReplaceAll(string(raw), "\r\n", "\n"))
	if err != nil {
		t.Fatal(err)
	}

	// T-52 to T-55 are the AI provider threats of epic 09; T-56 to T-59 are the publishing threats of epic 18;
	// T-60 and T-61 are the Meta threats of epic 19; T-62 to T-66 are the threats of a file sent from the
	// browser into an image, epic 48; T-70 to T-74 are the hosted agent threats of epic 14; T-75 to T-78 are
	// the AI provider sign-in threats of epic 12; T-80 to T-85 are the Google sign-in threats of epic 24;
	// T-90 and T-91 are the processed-sound threats of epic 51; T-92 to T-95 and T-99 are the blog threats of
	// epic 23.
	if len(threats) != 88 {
		t.Errorf("the model has %d threats, want 88 (T-99, the files destination of a blog)", len(threats))
	}
}

func TestSecuritySkipNeedsItsPlatformOrADocumentedCondition(t *testing.T) {
	t.Parallel()

	src := "package x\n\n" +
		"func TestSecurity_T01_Bare(t *testing.T) { t.Skip(\"x\") }\n" +
		"func TestSecurity_T01_Closure(t *testing.T) { t.Run(\"a\", func(t *testing.T) { t.Skipf(\"x\") }) }\n" +
		"func TestSecurity_T01_Now(t *testing.T) { t.SkipNow() }\n" +
		"func TestSecurity_T01_Linux(t *testing.T) { t.Skip(\"x\") }\n" +
		"func TestSecurity_T01_Windows_Paths(t *testing.T) { t.Skip(\"x\") }\n" +
		"func TestSecurity_T01_Marked(t *testing.T) {\n\t// skip-env: pinned-ffmpeg\n\tt.Skip(\"x\")\n}\n" +
		"func TestSecurity_T01_Unknown(t *testing.T) {\n\t// skip-env: because\n\tt.Skip(\"x\")\n}\n" +
		"func TestSecurity_T01_NoSkip(t *testing.T) {}\n" +
		"func TestOther(t *testing.T) { t.Skip(\"x\") }\n"

	got := skipProblems("x_test.go", []byte(src))
	if len(got) != 4 {
		t.Fatalf("got %d problems, want 4:\n%s", len(got), strings.Join(got, "\n"))
	}

	for _, name := range []string{"_Bare", "_Closure", "_Now", "_Unknown"} {
		if !strings.Contains(strings.Join(got, "\n"), name) {
			t.Errorf("no problem for %s", name)
		}
	}
}

func TestTheRepositoryHasNoUnmarkedSecuritySkip(t *testing.T) {
	t.Parallel()

	problems, err := skipsOf("../../..")
	if err != nil {
		t.Fatal(err)
	}

	if len(problems) > 0 {
		t.Errorf("%s", strings.Join(problems, "\n"))
	}
}

func TestThreatIDsHaveTwoOrThreeDigits(t *testing.T) {
	t.Parallel()

	row := func(id string) string {
		return "## 1. Intro\n\n## 2. Threat model\n\n| " + id + " | a | b | c | m | `TestSecurity_T100_X` |\n\n## 3. x\n"
	}

	for _, id := range []string{"T-01", "T-99", "T-100", "T-999"} {
		if _, err := parseThreats(row(id)); err != nil {
			t.Errorf("%s was refused: %v", id, err)
		}
	}

	for _, id := range []string{"T-1", "T-1000", "T-050", "T-099"} {
		if _, err := parseThreats(row(id)); err == nil {
			t.Errorf("%s was accepted", id)
		}
	}
}

func TestThreeDigitSecurityTestNames(t *testing.T) {
	t.Parallel()

	sdd := "## 1. Intro\n\n## 2. Threat model\n\n" +
		"| T-100 | a | b | c | m | `TestSecurity_T100_One` |\n" +
		"| T-101 | a | b | c | m | `TestSecurity_T100_Wrong` |\n" +
		"\n## 3. Local mode\n\n## 5. Scope matrix\n\n## 6. Next\n"

	root := t.TempDir()
	put := func(rel, body string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	put("docs/sdds/007-security-model.md", sdd)
	put("service/service.go", "package service\n\ntype Service interface {\n}\n")
	put(
		"x/x_test.go",
		"package x\n\nfunc TestSecurity_T100_One(t *testing.T) {}\nfunc TestSecurity_T100_Wrong(t *testing.T) {}\n"+
			"func TestSecurity_T1000_Four(t *testing.T) {}\nfunc TestSecurity_T050_Padded(t *testing.T) {}\n"+
			"func TestSecurity_T7_One(t *testing.T) {}\nfunc TestSecurity_ActorCannotImpersonate(t *testing.T) {}\n",
	)

	problems, err := check(root, "docs/sdds/007-security-model.md")
	if err != nil {
		t.Fatal(err)
	}

	got := strings.Join(problems, "\n")
	if !strings.Contains(got, "TestSecurity_T100_Wrong is named by the row T-101") {
		t.Errorf("a three-digit test under the wrong row is not reported:\n%s", got)
	}

	// A security test whose id is not a threat id would escape the check altogether (D-102).
	for _, name := range []string{"TestSecurity_T1000_Four", "TestSecurity_T050_Padded", "TestSecurity_T7_One"} {
		if !strings.Contains(got, name+" has no valid threat id") {
			t.Errorf("the misnamed %s is not reported:\n%s", name, got)
		}
	}

	// A security test with no threat id at all is a test of a rule, not of a threat row, and stays allowed.
	if strings.Contains(got, "TestSecurity_ActorCannotImpersonate") {
		t.Errorf("a security test with no threat id is reported:\n%s", got)
	}

	if strings.Contains(got, "TestSecurity_T100_One") {
		t.Errorf("a correct three-digit test is reported:\n%s", got)
	}
}
