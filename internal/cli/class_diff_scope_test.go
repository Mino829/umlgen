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

func TestClassDiffScopesRenames(t *testing.T) {
	for _, language := range []string{"java", "go"} {
		t.Run(language, func(t *testing.T) {
			root := packageDiffRepo(t, language)
			ext := "." + strings.ToUpper(language)
			source := func(name string) string {
				if language == "java" {
					return "package review; class " + name + " { int historical; }\n"
				}
				return "package review\ntype " + name + " struct { Historical int }\n"
			}
			type move struct{ name, from, to string }
			moves := []move{
				{"MovedOut", "watched/MovedOut", "outside/MovedOut"},
				{"MovedIn", "outside/MovedIn", "watched/MovedIn"},
				{"MovedInside", "watched/OldPath", "watched/NewPath"},
				{"IgnoredDestination", "watched/IgnoredDestination", "watched/ignored/IgnoredDestination"},
				{"IgnoredSource", "watched/ignored/IgnoredSource", "watched/IgnoredSource"},
				{"MovedOutside", "outside/OtherOld", "outside/OtherNew"},
			}
			for _, m := range moves {
				packageCLIWrite(t, filepath.Join(root, m.from+ext), source(m.name))
			}
			base := packageDiffCommit(t, root, "before rename")
			for _, m := range moves {
				runGit(t, root, "mv", "--", m.from+ext, m.to+ext)
			}
			head := packageDiffCommit(t, root, "after rename")
			packageCLIWrite(t, filepath.Join(root, moves[0].to+ext), source("DirtyOutside"))
			output := filepath.Join(t.TempDir(), "change.puml")
			render := func(target string) string {
				t.Helper()
				var stdout, stderr bytes.Buffer
				code, err := Run([]string{"diff", base + ".." + head, target, "--exclude", "ignored", "--no-cache", "--quiet", "-o", output}, &stdout, &stderr)
				if code != exitOK || err != nil {
					t.Fatalf("diff %q: %d %v\n%s\n%s", target, code, err, &stdout, &stderr)
				}
				return packageCLIRead(t, output)
			}
			checkColor := func(diagram, name, color string) {
				t.Helper()
				found := 0
				for line := range strings.SplitSeq(diagram, "\n") {
					if strings.Contains(line, `"`+name+`" as `) {
						found++
						if !strings.Contains(line, color) {
							t.Errorf("%s: expected %s in %q", name, color, line)
						}
					}
				}
				if found != 1 {
					t.Errorf("expected one declaration of %s, got %d:\n%s", name, found, diagram)
				}
			}
			diagram := render("watched")
			checkColor(diagram, "MovedOut", "#lightcoral")
			checkColor(diagram, "MovedIn", "#palegreen")
			checkColor(diagram, "MovedInside", "#lightyellow")
			checkColor(diagram, "IgnoredDestination", "#lightcoral")
			checkColor(diagram, "IgnoredSource", "#palegreen")
			if strings.Contains(diagram, "MovedOutside") || strings.Contains(diagram, "DirtyOutside") {
				t.Fatalf("out-of-scope sources in diagram:\n%s", diagram)
			}
			diagram = render(moves[0].from + ext)
			checkColor(diagram, "MovedOut", "#lightcoral")
			if !strings.Contains(strings.ToLower(diagram), "historical") {
				t.Errorf("base declaration missing for moved file:\n%s", diagram)
			}
		})
	}
}
