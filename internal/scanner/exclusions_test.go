package scanner

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSourceFilesExcludesLiteralDottedComponents(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{
		"src/User.java", "src/SuperUser.java", "src/User.java.bak.java",
		"src/.generated/Hidden.java", "src/.generated-copy/Keep.java",
		"src/generated.v1/Nested.java", "src/generated.v10/Keep.java",
	} {
		path = filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("class C {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := SourceFiles([]string{root}, []string{"User.java", ".generated", "src/generated.v1"}, ".java")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, path := range files {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, filepath.ToSlash(relative))
	}
	want := []string{"src/.generated-copy/Keep.java", "src/SuperUser.java", "src/User.java.bak.java", "src/generated.v10/Keep.java"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selected paths = %q, want %q", got, want)
	}
	// Explicit source targets retain the scanner's existing precedence.
	explicit := filepath.Join(root, "src", "User.java")
	selected, err := SourceFiles([]string{explicit}, []string{"User.java"}, ".java")
	if err != nil || !reflect.DeepEqual(selected, []string{explicit}) {
		t.Fatalf("explicit source target changed precedence: paths=%q err=%v", selected, err)
	}
}

func TestExcludedPathHonorsRelativeComponentBoundaries(t *testing.T) {
	for _, test := range []struct {
		path, exclude string
		want          bool
	}{
		{"root/src/generated.v1/A.java", "src/generated.v1", true},
		{"root/src/generated.v10/A.java", "src/generated.v1", false},
		{"User.java", "User.java", true},
		{"src/SuperUser.java", "User.java", false},
		{"src/User.java.bak", "User.java", false},
		{"src/com.example/A.java", "com.example", true},
		{"src/com.example.extra/A.java", "com.example", false},
	} {
		if got := ExcludedPath(filepath.FromSlash(test.path), []string{filepath.FromSlash(test.exclude)}); got != test.want {
			t.Errorf("ExcludedPath(%q, %q) = %v, want %v", test.path, test.exclude, got, test.want)
		}
	}
}
