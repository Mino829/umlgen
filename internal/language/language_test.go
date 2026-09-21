package language

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectDetectsGoModule(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	selected, err := Select("auto", []string{root}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Name() != "go" {
		t.Fatalf("language = %s, want go", selected.Name())
	}
}

func TestSelectRejectsMixedSources(t *testing.T) {
	_, err := Select("auto", []string{"."}, nil, []string{"User.java", "user.go"})
	if err == nil {
		t.Fatal("expected mixed-language error")
	}
}

func TestGoParserIgnoresTests(t *testing.T) {
	selected, err := ForName("go")
	if err != nil {
		t.Fatal(err)
	}
	if selected.MatchesPath("service_test.go") || !selected.MatchesPath("service.go") {
		t.Fatal("unexpected Go file matching")
	}
}
