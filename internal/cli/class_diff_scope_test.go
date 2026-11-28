package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestClassDiffScopesDeletedSourcesAndAutoLanguage(t *testing.T) {
	for _, language := range []string{"java", "go"} {
		t.Run(language, func(t *testing.T) {
			root := packageDiffRepo(t, language)
			var extension, live, deleted, outside string
			if language == "java" {
				extension = ".java"
				live = "package review; class Context {}\n"
				deleted = "package review; class Removed { Context historical; }\n"
				outside = "outside/other.go"
			} else {
				extension = ".go"
				live = "package review\ntype Context struct{}\n"
				deleted = "package review\ntype Removed struct { Historical Context }\n"
				outside = "outside/Other.java"
			}
			file := func(dir, name string) string { return dir + "/" + name + extension }
			wanted := file("watched", "Removed")
			gone := file("gone", "EntirelyDeleted")
			ignored := file("watched/ignored", "Ignored")
			generated := file("watched/vendor", "Generated")
			neighbor := file("watched-extra", "Neighbor")
			for name, content := range map[string]string{
				file("watched", "Context"): live, wanted: deleted,
				gone:      strings.ReplaceAll(deleted, "Removed", "EntirelyDeleted"),
				ignored:   strings.ReplaceAll(deleted, "Removed", "Ignored"),
				generated: strings.ReplaceAll(deleted, "Removed", "Generated"),
				neighbor:  strings.ReplaceAll(deleted, "Removed", "Neighbor"),
				outside:   "",
			} {
				packageCLIWrite(t, filepath.Join(root, name), content)
			}
			base := packageDiffCommit(t, root, "before deletion")
			runGit(t, root, "rm", "--", wanted, gone, ignored, generated, neighbor, outside)
			head := packageDiffCommit(t, root, "after deletion")
			packageCLIWrite(t, filepath.Join(root, "outside", "new"+filepath.Ext(outside)), "")
			output := filepath.Join(t.TempDir(), "change.puml")
			run := func(target string) (int, error, string) {
				var stdout, stderr bytes.Buffer
				code, err := Run([]string{"diff", base + ".." + head, target, "--exclude", "ignored", "--explain", "--quiet", "-o", output}, &stdout, &stderr)
				return code, err, stdout.String() + stderr.String()
			}
			code, err, explanation := run("watched")
			if code != exitOK || err != nil {
				t.Fatalf("selected scope: %d %v\n%s", code, err, explanation)
			}
			diagram := packageCLIRead(t, output)
			for _, want := range []string{`"Removed"`, `"Context"`, "#lightcoral"} {
				if !strings.Contains(diagram, want) {
					t.Errorf("diagram missing %q:\n%s", want, diagram)
				}
			}
			for _, want := range []string{"Git base", "Context"} {
				if !strings.Contains(explanation, want) {
					t.Errorf("explanation missing %q:\n%s", want, explanation)
				}
			}
			for _, unwanted := range []string{"EntirelyDeleted", "Ignored", "Generated", "Neighbor", "other.go", "Other.java"} {
				if strings.Contains(diagram, unwanted) || strings.Contains(explanation, unwanted) {
					t.Errorf("out-of-scope %q leaked:\n%s\n%s", unwanted, diagram, explanation)
				}
			}
			for _, target := range []string{"gone", gone} {
				code, err, explanation := run(target)
				if code != exitOK || err != nil {
					t.Fatalf("deleted target %q: %d %v\n%s", target, code, err, explanation)
				}
				if got := packageCLIRead(t, output); !strings.Contains(got, `"EntirelyDeleted"`) || !strings.Contains(got, "#lightcoral") || strings.Contains(got, `"Removed"`) {
					t.Errorf("deleted target %q:\n%s", target, got)
				}
			}
			before := packageCLIRead(t, output)
			if code, err, _ := run("missing"); code != exitError || err == nil || !strings.Contains(err.Error(), "target does not exist") || packageCLIRead(t, output) != before {
				t.Errorf("unknown target did not fail and preserve prior output: %d %v", code, err)
			}
		})
	}
}

func TestClassDiffJudgesMovesAgainstSelectedScope(t *testing.T) {
	for _, language := range []string{"java", "go"} {
		t.Run(language, func(t *testing.T) {
			root := packageDiffRepo(t, language)
			extension, header := ".java", "package review; "
			declaration := func(name string) string { return header + "class " + name + " {}\n" }
			if language == "go" {
				extension, header = ".go", "package review\n"
				declaration = func(name string) string { return header + "type " + name + " struct{}\n" }
			}
			file := func(dir, name string) string { return dir + "/" + name + extension }
			moves := [][2]string{
				{file("selected", "Removed"), file("other", "Removed")},
				{file("other", "Incoming"), file("selected", "Incoming")},
				{file("selected", "Stay"), file("selected/nested", "Stay")},
				{file("other", "Away"), file("elsewhere", "Away")},
				{file("selected/ignored", "Hidden"), file("other", "Hidden")},
				{file("other", "Shadowed"), file("selected/ignored", "Shadowed")},
			}
			packageCLIWrite(t, filepath.Join(root, file("selected", "Context")), declaration("Context"))
			for _, move := range moves {
				name := strings.TrimSuffix(filepath.Base(move[0]), extension)
				packageCLIWrite(t, filepath.Join(root, move[0]), declaration(name))
			}
			base := packageDiffCommit(t, root, "before moves")
			for _, move := range moves {
				if err := os.MkdirAll(filepath.Dir(filepath.Join(root, move[1])), 0o755); err != nil {
					t.Fatal(err)
				}
				runGit(t, root, "mv", "--", move[0], move[1])
			}
			head := packageDiffCommit(t, root, "after moves")
			output := filepath.Join(t.TempDir(), "moves.puml")
			run := func(target string) string {
				t.Helper()
				var stdout, stderr bytes.Buffer
				code, err := Run([]string{"diff", base + ".." + head, target, "--exclude", "ignored", "--quiet", "-o", output}, &stdout, &stderr)
				if code != exitOK || err != nil {
					t.Fatalf("diff %s: %d %v\n%s%s", target, code, err, stdout.String(), stderr.String())
				}
				return packageCLIRead(t, output)
			}
			diagram := run("selected")
			for name, color := range map[string]string{"Removed": "#lightcoral", "Incoming": "#palegreen", "Stay": "#lightyellow"} {
				if !regexp.MustCompile(`"` + name + `".*` + color).MatchString(diagram) {
					t.Errorf("%s should be drawn with %s:\n%s", name, color, diagram)
				}
			}
			for _, unwanted := range []string{"Away", "Hidden", "Shadowed"} {
				if strings.Contains(diagram, unwanted) {
					t.Errorf("out-of-scope move %q leaked:\n%s", unwanted, diagram)
				}
			}
			for target, want := range map[string]string{
				moves[0][0]: `"Removed".*#lightcoral`,
				moves[0][1]: `"Removed".*#palegreen`,
				moves[2][1]: `"Stay".*#palegreen`,
			} {
				if got := run(target); !regexp.MustCompile(want).MatchString(got) {
					t.Errorf("target %s should match %q:\n%s", target, want, got)
				}
			}
		})
	}
}
