package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func packageDiffCommit(t *testing.T, root, message string) string {
	t.Helper()
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", message)
	data, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

func packageDiffRepo(t *testing.T, language string) string {
	t.Helper()
	// Git snapshots use the physical repository root. Windows TEMP can use an
	// 8.3 alias (RUNNER~1), so fixture evidence must use that same file identity.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "umlgen@example.test")
	runGit(t, root, "config", "user.name", "umlgen test")
	if language == "go" {
		packageCLIWrite(t, filepath.Join(root, "go.mod"), "module example.test/diff\n\ngo 1.25\n")
	}
	t.Setenv("UMLGEN_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))
	t.Chdir(root)
	return root
}

func packageDiffSource(t *testing.T, root, language, pkg, name, target string) string {
	t.Helper()
	var path, source string
	if language == "java" {
		path = filepath.Join(root, pkg, name+".java")
		source = "package " + pkg + ";\n"
		if target == "" {
			source += "class " + name + " {}\n"
		} else {
			source += fmt.Sprintf("import %s;\nclass %s {\n    %s dependency;\n}\n", target, name, target)
		}
	} else {
		path = filepath.Join(root, pkg, strings.ToLower(name)+".go")
		source = "package " + pkg + "\n"
		if target == "" {
			source += "type " + name + " struct{}\n"
		} else {
			owner, typ, ok := strings.Cut(target, ".")
			if !ok {
				t.Fatalf("invalid fixture target %q", target)
			}
			source += fmt.Sprintf("import \"example.test/diff/%s\"\ntype %s struct {\n    Dependency %s.%s\n}\n", owner, name, owner, typ)
		}
	}
	packageCLIWrite(t, path, source)
	return path
}

func TestPackageDiffUsesCompleteImmutableSnapshots(t *testing.T) {
	for _, language := range []string{"java", "go"} {
		t.Run(language, func(t *testing.T) {
			root := packageDiffRepo(t, language)
			first := packageDiffSource(t, root, language, "a", "First", "b.Model")
			second := packageDiffSource(t, root, language, "a", "Second", "b.Model")
			packageDiffSource(t, root, language, "b", "Model", "")
			base := packageDiffCommit(t, root, "two independent dependency proofs")
			if err := os.Remove(first); err != nil {
				t.Fatal(err)
			}
			head := packageDiffCommit(t, root, "remove only the first proof")
			// None of these bytes belong to the selected immutable head.
			packageCLIWrite(t, second, "WORKTREE_ONLY invalid syntax")
			staged := packageDiffSource(t, root, language, "staged", "Staged", "")
			runGit(t, root, "add", staged)
			packageDiffSource(t, root, language, "untracked", "Untracked", "")
			if language == "go" {
				packageCLIWrite(t, filepath.Join(root, "go.mod"), "module example.test/dirty\n\ngo 1.25\n")
			}
			output := filepath.Join(t.TempDir(), "diff.puml")
			args := []string{"diff", base + ".." + head, "--diagram", "package", "--language", language, "--dependency-source", "both", "--quiet", "--explain", "-o", output}
			cold := packageCLISuccess(t, args...)
			diagram := packageCLIRead(t, output)
			prefix := ""
			if language == "go" {
				prefix = "example.test/diff/"
			}
			packageCLIRequireNodes(t, diagram, prefix+"a", prefix+"b")
			if strings.Count(diagram, "]-> PKG_") != 1 ||
				!strings.Contains(diagram, "[modified evidence; edge retained]; before: types=2; imports=2; after: types=1; imports=1") {
				t.Fatalf("remaining unchanged-file evidence was not retained with correct before/after counts:\n%s", diagram)
			}
			for _, absent := range []string{"WORKTREE_ONLY", "example.test/dirty", "staged.Staged", "untracked.Untracked"} {
				if strings.Contains(cold.stdout+diagram, absent) {
					t.Errorf("current checkout leaked into committed comparison: %q\n%s", absent, cold.stdout)
				}
			}
			for _, required := range []string{base, head, first, second} {
				if !strings.Contains(cold.stdout, required) {
					t.Errorf("missing pinned revision or original-file evidence %q:\n%s", required, cold.stdout)
				}
			}
			warm := packageCLISuccess(t, args...)
			uncached := packageCLISuccess(t, append(args, "--no-cache")...)
			if cold.stdout != warm.stdout || cold.stdout != uncached.stdout || diagram != packageCLIRead(t, output) {
				t.Fatal("syntax cache changed pinned comparison or explanation")
			}
			plain := append([]string(nil), args...)
			for n, arg := range plain {
				if arg == "--explain" {
					plain = append(plain[:n], plain[n+1:]...)
					break
				}
			}
			packageCLISuccess(t, plain...)
			if diagram != packageCLIRead(t, output) {
				t.Fatal("explanation changed the diff diagram")
			}
			// A single revision means revision..HEAD, not the class diff's
			// working-tree convention. Dirty sources above must remain excluded.
			single := append([]string(nil), args...)
			single[1] = base
			if got := packageCLISuccess(t, single...).stdout; got != cold.stdout {
				t.Fatal("single revision was not pinned to HEAD")
			}
			runGit(t, root, "reset", "--hard", head)
			runGit(t, root, "clean", "-fd")
			if err := os.Remove(second); err != nil {
				t.Fatal(err)
			}
			last := packageDiffCommit(t, root, "remove final proof")
			args[1] = head + ".." + last
			removed := packageCLISuccess(t, args...)
			if !strings.Contains(removed.stdout, second) || !strings.Contains(packageCLIRead(t, output), "removed") {
				t.Fatalf("last proof removal was not explained from Git source:\n%s\n%s", removed.stdout, packageCLIRead(t, output))
			}
		})
	}
}

func TestPackageDiffIncompleteCycleDoesNotClaimResolution(t *testing.T) {
	for _, language := range []string{"java", "go"} {
		t.Run(language, func(t *testing.T) {
			root := packageDiffRepo(t, language)
			first := packageDiffSource(t, root, language, "a", "First", "b.Model")
			second := packageDiffSource(t, root, language, "b", "Model", "a.First")
			base := packageDiffCommit(t, root, "cycle")
			broken := "package a; class First {"
			if language == "go" {
				broken = "package a\ntype First struct {\n"
			}
			packageCLIWrite(t, first, broken)
			head := packageDiffCommit(t, root, "unparseable cycle member")
			output := filepath.Join(t.TempDir(), "incomplete.puml")
			args := []string{"diff", base + ".." + head, "--diagram=package", "--language", language, "--quiet", "--explain", "-o", output}
			result := packageCLIRun(args...)
			if result.code != exitParse || result.err == nil {
				t.Fatalf("partial failure: code=%d err=%v stderr=%s", result.code, result.err, result.stderr)
			}
			diagram := packageCLIRead(t, output)
			if !strings.Contains(diagram, "uncertain") || !strings.Contains(result.stdout, first) || !strings.Contains(result.stderr, head) {
				t.Fatalf("failure was hidden or cycle removal declared certain:\n%s\n%s\n%s", diagram, result.stdout, result.stderr)
			}
			packageCLIWrite(t, second, broken)
			last := packageDiffCommit(t, root, "all selected sources broken")
			args[1] = base + ".." + last
			const prior = "previous diagram must survive\n"
			packageCLIWrite(t, output, prior)
			result = packageCLIRun(args...)
			if result.code != exitParse || result.err == nil || packageCLIRead(t, output) != prior {
				t.Fatalf("total failure modified prior output: code=%d err=%v", result.code, result.err)
			}
			if !strings.Contains(result.stdout, first) || !strings.Contains(result.stdout, second) {
				t.Fatalf("total-failure source diagnostics absent:\n%s", result.stdout)
			}
		})
	}
}

func TestPackageDiffSelectionAndDefaultOutput(t *testing.T) {
	root := packageDiffRepo(t, "java")
	packageDiffSource(t, root, "java", "a", "First", "b.Model")
	packageDiffSource(t, root, "java", "b", "Model", "")
	base := packageDiffCommit(t, root, "base")
	packageDiffSource(t, root, "java", "a", "Added", "b.Model")
	head := packageDiffCommit(t, root, "head")
	packageCLIWrite(t, "class-diagram.puml", "keep class diagram\n")
	packageCLIWrite(t, "change-diagram.puml", "keep class diff\n")
	packageCLIWrite(t, "package-diagram.puml", "keep package diagram\n")
	packageCLISuccess(t, "diff", base+".."+head, "--diagram=package", "--quiet", "--title", "--diagram=class")
	packageCLIRequireNodes(t, packageCLIRead(t, "package-change-diagram.puml"), "a", "b")
	for path, expected := range map[string]string{"class-diagram.puml": "keep class diagram\n", "change-diagram.puml": "keep class diff\n", "package-diagram.puml": "keep package diagram\n"} {
		if packageCLIRead(t, path) != expected {
			t.Errorf("default package diff overwrote %s", path)
		}
	}
	for _, flag := range []string{"--diagram=unknown", "--diagram=package --depth=1", "--diagram=package --hide-fields", "--diagram=package --dependency-source=imports --relations=field"} {
		args := append([]string{"diff", base + ".." + head}, strings.Fields(flag)...)
		args = append(args, "-o", "package-change-diagram.puml")
		before := packageCLIRead(t, "package-change-diagram.puml")
		result := packageCLIRun(args...)
		if result.code != exitArgs || result.err == nil || packageCLIRead(t, "package-change-diagram.puml") != before {
			t.Errorf("invalid selection changed output: %q code=%d err=%v", flag, result.code, result.err)
		}
	}
	plain := []string{"diff", base + ".." + head, "--quiet", "-o", filepath.Join(t.TempDir(), "class.puml")}
	packageCLISuccess(t, plain...)
	want := packageCLIRead(t, plain[len(plain)-1])
	packageCLISuccess(t, append(plain, "--diagram=class")...)
	if got := packageCLIRead(t, plain[len(plain)-1]); got != want {
		t.Fatal("explicit class diagram changed legacy class diff")
	}
	packageCLISuccess(t, "diff", head+".."+head, "--diagram=package", "--quiet", "-o", "equal.puml")
	packageCLIRequireNodes(t, packageCLIRead(t, "equal.puml"), "a", "b")
}

func TestPackageDiffUnresolvedTargetKeepsCycleChangeUncertain(t *testing.T) {
	for _, language := range []string{"java", "go"} {
		t.Run(language, func(t *testing.T) {
			root := packageDiffRepo(t, language)
			first := packageDiffSource(t, root, language, "a", "First", "b.Model")
			model := packageDiffSource(t, root, language, "b", "Model", "a.First")
			base := packageDiffCommit(t, root, "resolved cycle")
			replacement := "package b; class Renamed {}\n"
			if language == "go" {
				replacement = "package b\ntype Renamed struct{}\n"
			}
			packageCLIWrite(t, model, replacement)
			head := packageDiffCommit(t, root, "target declaration no longer resolves")
			output := filepath.Join(t.TempDir(), "uncertain.puml")
			args := []string{"diff", base + ".." + head, "--diagram=package", "--language", language, "--quiet", "--explain", "-o", output}
			result := packageCLISuccess(t, args...)
			diagram := packageCLIRead(t, output)
			if !strings.Contains(diagram, "[uncertain; incomplete analysis]") ||
				!strings.Contains(diagram, "Cycle group: declared; uncertain;") ||
				!strings.Contains(result.stdout, `reference="b.Model"`) {
				t.Fatalf("unresolved reference was mistaken for proven dependency/cycle removal:\n%s\n%s", diagram, result.stdout)
			}
			// Once the unresolved reference itself is removed, the selected
			// source graph can establish that both original edges disappeared.
			source := "package a; class First {}\n"
			if language == "go" {
				source = "package a\ntype First struct{}\n"
			}
			packageCLIWrite(t, first, source)
			last := packageDiffCommit(t, root, "remove unresolved declaration")
			args[1] = base + ".." + last
			packageCLISuccess(t, args...)
			if got := packageCLIRead(t, output); strings.Contains(got, "[uncertain;") || !strings.Contains(got, "Cycle group: declared; removed;") {
				t.Fatalf("resolved analysis did not distinguish actual removal:\n%s", got)
			}
		})
	}
}
