package cli

import (
	"bytes"
	"path/filepath"
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
