package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJavaFilesRecursesAndExcludes(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{
		"src/main/User.java",
		"src/main/readme.txt",
		"src/test/UserTest.java",
		"target/Generated.java",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("class X {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := JavaFiles([]string{root}, []string{"src/test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != "User.java" {
		t.Fatalf("unexpected files: %#v", files)
	}
}

func TestSourceFilesSupportsGo(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"main.go", "service_test.go", "User.java", "vendor/dependency.go"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package sample"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := SourceFiles([]string{root}, nil, ".go")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %#v", files)
	}
}
