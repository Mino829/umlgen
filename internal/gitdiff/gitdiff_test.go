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

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)

	result, err := Analyze("HEAD")
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
