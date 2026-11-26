package gitdiff

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
)

func TestAnalyzeAddedModifiedAndDeletedFiles(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init")
	git(t, root, "config", "user.email", "umlgen@example.test")
	git(t, root, "config", "user.name", "umlgen test")
	write(t, root, "Keep.java", "class Keep {}")
	write(t, root, "Delete.java", "class Delete {}")
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "base")

	write(t, root, "Keep.java", "class Keep { int value; }")
	write(t, root, "Added.java", "class Added {}")
	write(t, root, "Untracked.java", "class Untracked {}")
	if err := os.Remove(filepath.Join(root, "Delete.java")); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "-A")
	git(t, root, "reset", "Untracked.java")

	result, err := Analyze(root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if result.Current["Keep.java"] != model.Modified {
		t.Fatalf("modified = %#v", result.Current)
	}
	if result.Current["Added.java"] != model.Added {
		t.Fatalf("added = %#v", result.Current)
	}
	if result.Current["Untracked.java"] != model.Added {
		t.Fatalf("untracked = %#v", result.Current)
	}
	if change, ok := result.ChangeFor(filepath.Join(root, "Keep.java")); !ok || change != model.Modified {
		t.Fatalf("ChangeFor(Keep.java) = %q, %v", change, ok)
	}
	checkDeleted(t, result, "Delete.java", "class Delete {}")
}

func TestAnalyzePreservesGitPathBytes(t *testing.T) {
	modified := "変更.java"
	renamedTo := "新しい.go"
	untracked := " 未追跡.go"
	if runtime.GOOS != "windows" {
		modified = "変更\t.java"
		renamedTo = "新しい\n.go"
		untracked = " 未追跡\n.go"
	}
	const deleted = " 削除.java"
	const added = "追加.java"
	const renameSource = "旧.go"
	const deletedSource = "class Removed {}"

	root := t.TempDir()
	git(t, root, "init")
	git(t, root, "config", "user.email", "umlgen@example.test")
	git(t, root, "config", "user.name", "umlgen test")
	git(t, root, "config", "core.quotePath", "true")
	write(t, root, modified, "class Changed {}")
	write(t, root, deleted, deletedSource)
	write(t, root, renameSource, "package project\n\ntype Moved struct{}\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "base")
	write(t, root, modified, "class Changed { int value; }")
	write(t, root, added, "class Added {}")
	git(t, root, "add", "--", added)
	git(t, root, "rm", "--", deleted)
	git(t, root, "mv", "--", renameSource, renamedTo)
	write(t, root, untracked, "package project\n\ntype Untracked struct{}\n")

	result, err := Analyze(root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]model.ChangeKind{
		modified: model.Modified, added: model.Added, renamedTo: model.Modified, untracked: model.Added,
	} {
		if got, ok := result.ChangeFor(filepath.Join(root, path)); !ok || got != want {
			t.Errorf("ChangeFor(%q) = %q, %v; want %q", path, got, ok, want)
		}
	}
	if _, ok := result.ChangeFor(filepath.Join(root, renameSource)); ok {
		t.Errorf("rename source %q should not be a changed current path", renameSource)
	}
	checkDeleted(t, result, deleted, deletedSource)
}

func checkDeleted(t *testing.T, result Result, name, want string) {
	t.Helper()
	if len(result.Deleted) != 1 || result.Deleted[0] != name {
		t.Fatalf("deleted files = %q; want only %q", result.Deleted, name)
	}
	content, err := result.ReadDeleted(result.Deleted[0])
	if err != nil || string(content) != want {
		t.Fatalf("deleted content = %q, %v; want %q", content, err, want)
	}
}

func TestAnalyzeUsesCommitSourcesForOmittedRangeEndpoints(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "base")
	git(t, root, "config", "user.email", "umlgen@example.test")
	git(t, root, "config", "user.name", "umlgen test")
	const name = "Removed.java"
	write(t, root, name, "class MergeBase {}")
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "common")
	git(t, root, "switch", "-c", "without-source")
	git(t, root, "rm", name)
	git(t, root, "commit", "-m", "delete")
	git(t, root, "switch", "base")
	write(t, root, name, "class BaseHead {}")
	git(t, root, "commit", "-am", "diverge")
	write(t, root, name, "class Staged {}")
	git(t, root, "add", name)
	write(t, root, name, "class WorkingTree {}")

	for spec, want := range map[string]string{"..without-source": "class BaseHead {}", "...without-source": "class MergeBase {}"} {
		result, err := Analyze(root, spec)
		if err != nil {
			t.Fatalf("%s: %v", spec, err)
		}
		checkDeleted(t, result, name, want)
	}
	git(t, root, "switch", "--discard-changes", "without-source")
	result, err := Analyze(root, "base...")
	if err != nil {
		t.Fatal(err)
	}
	checkDeleted(t, result, name, "class MergeBase {}")
}

func TestAnalyzeConcurrentRepositories(t *testing.T) {
	for _, name := range []string{"First", "Second"} {
		root := t.TempDir()
		git(t, root, "init")
		git(t, root, "config", "user.email", "umlgen@example.test")
		git(t, root, "config", "user.name", "umlgen test")
		modified := name + ".java"
		deleted := name + "Deleted.java"
		deletedContent := "class " + name + "Deleted {}"
		write(t, root, modified, "class "+name+" {}")
		write(t, root, deleted, deletedContent)
		git(t, root, "add", ".")
		git(t, root, "commit", "-m", "base")
		write(t, root, modified, "class "+name+" { int revision; }")
		if err := os.Remove(filepath.Join(root, deleted)); err != nil {
			t.Fatal(err)
		}
		nested := filepath.Join(root, "nested")
		if err := os.Mkdir(nested, 0o755); err != nil {
			t.Fatal(err)
		}
		expectedRoot, err := os.Stat(root)
		if err != nil {
			t.Fatal(err)
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for range 10 {
				result, err := Analyze(nested, "HEAD")
				if err != nil {
					t.Fatal(err)
				}
				actualRoot, err := os.Stat(result.Root)
				if err != nil || !os.SameFile(expectedRoot, actualRoot) {
					t.Fatalf("root %q does not identify repository %q: %v", result.Root, root, err)
				}
				if len(result.Current) != 1 || result.Current[modified] != model.Modified {
					t.Fatalf("current files = %#v, want only %s modified", result.Current, modified)
				}
				checkDeleted(t, result, deleted, deletedContent)
			}
		})
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func write(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
