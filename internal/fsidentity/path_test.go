package fsidentity_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/fsidentity"
)

func TestResolveExistingPrefixBeforeParentTraversal(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "physical", "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "physical", "child"), filepath.Join(root, "alias")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	t.Chdir(root)
	for _, suffix := range []string{"alias/..", "alias/../future/nested/", "alias/../future/../nested"} {
		for _, absolute := range []bool{false, true} {
			path := suffix
			if absolute {
				// Join would erase the traversal this test must exercise.
				path = root + string(os.PathSeparator) + suffix
			}
			want := filepath.Join(root, "physical")
			switch suffix {
			case "alias/../future/nested/":
				want = filepath.Join(want, "future", "nested")
			case "alias/../future/../nested":
				want = filepath.Join(want, "nested")
			}
			got, err := fsidentity.Resolve(path)
			if err != nil || got != want {
				t.Fatalf("Resolve(%q) = %q, %v; want %q", path, got, err, want)
			}
			if runtime.GOOS == "windows" {
				path = strings.ReplaceAll(path, "/", "\\")
				got, err = fsidentity.Resolve(path)
				if err != nil || got != want {
					t.Fatalf("Resolve(%q) = %q, %v; want %q", path, got, err, want)
				}
			}
		}
	}
}

func TestResolveFuturePathsPreservesCaseAndLiteralNames(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	for _, path := range []string{"Future [api] (v1)/Child", "future [api] (v1)/child"} {
		got, err := fsidentity.Resolve(path)
		want := filepath.Join(root, path)
		if err != nil || got != want {
			t.Fatalf("Resolve(%q) = %q, %v; want exact identity %q", path, got, err, want)
		}
	}
}

func TestResolveRejectsNonDirectoryPrefix(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if path, err := fsidentity.Resolve(file + string(os.PathSeparator) + "child"); err == nil {
		t.Fatalf("Resolve accepted a non-directory prefix as %q", path)
	}
}
