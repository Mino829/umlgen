package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelationshipDiffComparesCommittedDeclarationEvidence(t *testing.T) {
	for _, language := range []string{"java", "go"} {
		t.Run(language, func(t *testing.T) {
			root := packageDiffRepo(t, language)
			var path, oldSource, newSource, owner string
			if language == "java" {
				path = "Owner.java"
				oldSource = "package review;\nclass Owner {\n    Target first;\n    Target retained;\n    Gone remove;\n}\n"
				newSource = "package review;\n\nclass Owner {\n    Target retained;\n    New add;\n}\n"
				owner = "review.Owner"
				packageCLIWrite(t, filepath.Join(root, "Targets.java"), "package review; class Target {} class Gone {} class New {}\n")
			} else {
				path = "owner.go"
				oldSource = "package review\ntype Owner struct {\n    First Target\n    Retained Target\n    Remove Gone\n}\n"
				newSource = "package review\n\ntype Owner struct {\n    Retained Target\n    Add New\n}\n"
				owner = "example.test/diff.Owner"
				packageCLIWrite(t, filepath.Join(root, "targets.go"), "package review\ntype Target struct{}\ntype Gone struct{}\ntype New struct{}\n")
			}
			file := filepath.Join(root, path)
			packageCLIWrite(t, file, oldSource)
			base := packageDiffCommit(t, root, "before")
			packageCLIWrite(t, file, "\n"+oldSource)
			moved := packageDiffCommit(t, root, "pure line movement")
			packageCLIWrite(t, file, newSource)
			head := packageDiffCommit(t, root, "evidence and edge changes")
			packageCLIWrite(t, file, "dirty checkout must not be parsed")
			runGit(t, root, "add", path)
			packageCLIWrite(t, filepath.Join(root, "go.mod"), "module example.test/dirty\n")

			run := func(left, right string, flags ...string) string {
				t.Helper()
				args := append([]string{"diff", left + ".." + right, "--report", "relationships", "--language", language, "--relations", "field", "--explain"}, flags...)
				code, err, stdout, stderr := runRelationshipCLI(args...)
				if code != exitOK || err != nil {
					t.Fatalf("relationship report failed: %d %v\n%s", code, err, stderr)
				}
				return stdout
			}
			if got := run(base, moved); !strings.Contains(got, "No relationship changes detected") {
				t.Fatalf("line movement reported as a change:\n%s", got)
			}
			got := run(moved, head)
			prefix := strings.TrimSuffix(owner, "Owner")
			for _, want := range []string{
				"removed " + owner + " -> " + prefix + "Gone [field] (evidence: 1 -> 0)",
				"added " + owner + " -> " + prefix + "New [field] (evidence: 0 -> 1)",
				"evidence-changed " + owner + " -> " + prefix + "Target [field] (evidence: 2 -> 1)",
				"Before: " + moved, "After: " + head, `before "` + path + `":4`, `after "` + path + `":4`,
			} {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q:\n%s", want, got)
				}
			}
			if !strings.Contains(got, "4 |     "+strings.Split(oldSource, "\n")[2][4:]) || strings.Contains(got, "dirty") {
				t.Errorf("wrong captured snapshot snippets:\n%s", got)
			}
			if warm, uncached := run(moved, head), run(moved, head, "--no-cache"); warm != got || uncached != got {
				t.Fatalf("cache state changed report\ncold:\n%s\nwarm:\n%s\nuncached:\n%s", got, warm, uncached)
			}
		})
	}
}

func TestRelationshipDiffDoesNotAssertUncertainRemovals(t *testing.T) {
	root := packageDiffRepo(t, "java")
	file := filepath.Join(root, "Owner.java")
	packageCLIWrite(t, file, "package p; class Owner { Target member; }\n")
	packageCLIWrite(t, filepath.Join(root, "Target.java"), "package p; class Target {}\n")
	base := packageDiffCommit(t, root, "base")
	packageCLIWrite(t, file, "package p; class Owner { Unknown member; }\n")
	unresolved := packageDiffCommit(t, root, "unresolved")
	packageCLIWrite(t, file, "package p; class Owner {\n")
	broken := packageDiffCommit(t, root, "parse failure")
	packageCLIWrite(t, file, "package p; class Owner {}\n")
	packageCLIWrite(t, filepath.Join(root, "Duplicate.java"), "package p; class Owner {}\n")
	duplicate := packageDiffCommit(t, root, "ambiguous")

	for _, tc := range []struct {
		revision string
		code     int
	}{{unresolved, exitOK}, {broken, exitParse}} {
		code, err, report, _ := runRelationshipCLI("diff", base+".."+tc.revision, "--report=relationships")
		if code != tc.code || (err != nil) != (tc.code != exitOK) || !strings.Contains(report, "uncertain p.Owner -> p.Target [field]") || strings.Contains(report, "removed p.Owner") {
			t.Errorf("false certain removal: code %d, error %v\n%s", code, err, report)
		}
	}
	code, err, report, _ := runRelationshipCLI("diff", base+".."+duplicate, "--report", "relationships")
	if code != exitParse || err == nil || !strings.Contains(err.Error(), `ambiguous type "p.Owner"`) || report != "" {
		t.Fatalf("ambiguous type joined snapshots: code %d, error %v\n%s", code, err, report)
	}
}

func TestRelationshipDiffExplainsUncertainty(t *testing.T) {
	for _, language := range []string{"java", "go"} {
		t.Run(language, func(t *testing.T) {
			root := packageDiffRepo(t, language)
			write := func(path, source string) {
				t.Helper()
				packageCLIWrite(t, filepath.Join(root, path), source)
			}
			var pkg, path, beforeBroken, afterBroken, broken string
			var beforeSource, afterSource string
			if language == "java" {
				pkg, path = "review", "review/Owner.java"
				beforeBroken, afterBroken = "broken/Before.java", "broken/After.java"
				broken = "package broken; class Broken {\n"
				write("review/Targets.java", "package review; class Target {} class Other { UnknownOwner field; }\n")
				write("outside/Hidden.java", "package outside; public class Hidden {}\n")
				beforeSource = "package review;\nimport outside.Hidden;\nclass Owner {\n    Target first;\n    UnknownBefore missing;\n    Hidden external;\n    void wrongKind(UnknownKind value) {}\n}\nclass Certain { Target first; }\n"
				afterSource = strings.NewReplacer("Target first", "Target second", "    UnknownBefore", "\n    UnknownAfter").Replace(beforeSource)
			} else {
				pkg, path = "example.test/diff/review", "review/owner.go"
				beforeBroken, afterBroken = "broken/before.go", "broken/after.go"
				broken = "package broken\ntype Broken struct {\n"
				write("review/targets.go", "package review\ntype Target struct{}\ntype Other struct { Field UnknownOwner }\n")
				write("outside/hidden.go", "package outside\ntype Hidden struct{}\n")
				beforeSource = "package review\nimport \"example.test/diff/outside\"\ntype Owner struct {\n    First Target\n    Missing UnknownBefore\n    External outside.Hidden\n}\nfunc (Owner) wrongKind(value UnknownKind) {}\ntype Certain struct { First Target }\n"
				afterSource = strings.NewReplacer("First Target", "Second Target", "    Missing UnknownBefore", "\n    Missing UnknownAfter").Replace(beforeSource)
			}
			write(path, beforeSource)
			write(beforeBroken, broken)
			base := packageDiffCommit(t, root, "before")
			runGit(t, root, "rm", beforeBroken)
			write(path, afterSource)
			write(afterBroken, broken)
			head := packageDiffCommit(t, root, "after")

			args := []string{"diff", base + ".." + head, "--report", "relationships", "--language", language, "--relations", "field,parameter"}
			run := func(wantCode int, flags ...string) string {
				t.Helper()
				code, err, report, stderr := runRelationshipCLI(append(args, flags...)...)
				if code != wantCode || (err != nil) != (wantCode != exitOK) {
					t.Fatalf("exit=%d error=%v, want exit=%d\n%s\n%s", code, err, wantCode, report, stderr)
				}
				return report
			}
			explained := run(exitOK, "--include", pkg, "--exclude", "broken", "--explain")
			plain := run(exitOK, "--include", pkg, "--exclude", "broken")
			ownerEdge := "uncertain " + pkg + ".Owner -> " + pkg + ".Target [field] (evidence: 1 -> 1)"
			certainEdge := "evidence-changed " + pkg + ".Certain -> " + pkg + ".Target [field] (evidence: 1 -> 1)"
			for _, want := range []string{ownerEdge, certainEdge} {
				if !strings.Contains(explained, want) || !strings.Contains(plain, want) {
					t.Errorf("missing %q:\n%s\n%s", want, explained, plain)
				}
			}
			_, ownerSection, _ := strings.Cut(explained, ownerEdge)
			for _, want := range []string{
				`before unresolved reference "UnknownBefore" at "` + path + `":5`,
				`after unresolved reference "UnknownAfter" at "` + path + `":6`,
			} {
				if strings.Count(explained, want) != 1 || !strings.Contains(ownerSection, want) {
					t.Errorf("expected %q only under the Owner edge:\n%s", want, explained)
				}
			}
			if strings.Count(explained, " unresolved reference ") != 2 || strings.Contains(explained, root) {
				t.Errorf("unrelated diagnostics or absolute paths leaked:\n%s", explained)
			}
			if strings.Contains(plain, " unresolved reference ") || strings.Contains(plain, path) || strings.Contains(plain, " | ") {
				t.Errorf("plain report exposed explanation details:\n%s", plain)
			}

			partial := run(exitParse, "--include", pkg, "--explain")
			failures := []string{`before failed to parse "` + beforeBroken + `"`, `after failed to parse "` + afterBroken + `"`}
			header, _, _ := strings.Cut(partial, "uncertain "+pkg+".Certain")
			if header == partial || !strings.Contains(header, "Parse failures affecting the entire comparison:") {
				t.Errorf("missing global uncertainty for the otherwise certain edge:\n%s", partial)
			}
			for _, want := range failures {
				if strings.Count(partial, want) != 1 || !strings.Contains(header, want) {
					t.Errorf("expected %q once, above the edges:\n%s", want, partial)
				}
			}
			if got := run(exitParse, "--include", pkg); strings.Contains(got, beforeBroken) || strings.Contains(got, afterBroken) {
				t.Errorf("plain report exposed failure paths:\n%s", got)
			}
			unusable := run(exitParse, "broken", "--explain")
			for _, want := range append(failures, "No comparison:") {
				if !strings.Contains(unusable, want) {
					t.Errorf("missing %q for all-failed comparison:\n%s", want, unusable)
				}
			}
		})
	}
}

func runRelationshipCLI(args ...string) (int, error, string, string) {
	var stdout, stderr bytes.Buffer
	code, err := Run(args, &stdout, &stderr)
	return code, err, stdout.String(), stderr.String()
}
