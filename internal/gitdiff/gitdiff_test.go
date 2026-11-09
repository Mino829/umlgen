package gitdiff

import (
	"os"
	"os/exec"
	"path/filepath"
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
	if len(result.Deleted) != 1 || string(result.Deleted[0].Content) != "class Delete {}" {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
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
				if len(result.Deleted) != 1 || string(result.Deleted[0].Content) != deletedContent {
					t.Fatalf("deleted files = %#v, want only %s with content %q", result.Deleted, deleted, deletedContent)
				}
				relative, err := filepath.Rel(result.Root, result.Deleted[0].Path)
				if err != nil || relative != deleted {
					t.Fatalf("deleted path %q is not %q within repository %q: %v", result.Deleted[0].Path, deleted, result.Root, err)
				}
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
