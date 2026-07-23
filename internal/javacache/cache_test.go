package javacache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFileCachesAndInvalidatesContent(t *testing.T) {
	t.Setenv(cacheDirectoryEnv, filepath.Join(t.TempDir(), "cache"))
	source := filepath.Join(t.TempDir(), "Type.java")
	writeSource(t, source, "package sample; class First {}")

	cache, err := Open("1.0.0", map[string]string{"language": "java"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := cache.ParseFile(source)
	if err != nil || first.Hit || len(first.Types) != 1 || first.Types[0].Name != "First" {
		t.Fatalf("first = %#v, err = %v", first, err)
	}
	entries, err := filepath.Glob(filepath.Join(os.Getenv(cacheDirectoryEnv), "java", "*.json"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %#v, err = %v", entries, err)
	}
	payload, err := os.ReadFile(entries[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), source) || strings.Contains(string(payload), "package sample") {
		t.Fatalf("cache contains source path or source text: %s", payload)
	}
	second, err := cache.ParseFile(source)
	if err != nil || !second.Hit || second.Types[0].Source != source {
		t.Fatalf("second = %#v, err = %v", second, err)
	}

	writeSource(t, source, "package sample; class Second {}")
	changed, err := cache.ParseFile(source)
	if err != nil || changed.Hit || changed.Types[0].Name != "Second" {
		t.Fatalf("changed = %#v, err = %v", changed, err)
	}
}

func TestCacheInvalidatesVersionAndSettings(t *testing.T) {
	t.Setenv(cacheDirectoryEnv, filepath.Join(t.TempDir(), "cache"))
	source := filepath.Join(t.TempDir(), "Type.java")
	writeSource(t, source, "package sample; class Type {}")

	initial, err := Open("1.0.0", map[string]bool{"private": true})
	if err != nil {
		t.Fatal(err)
	}
	if result, parseErr := initial.ParseFile(source); parseErr != nil || result.Hit {
		t.Fatalf("initial = %#v, err = %v", result, parseErr)
	}
	if result, parseErr := initial.ParseFile(source); parseErr != nil || !result.Hit {
		t.Fatalf("warm = %#v, err = %v", result, parseErr)
	}

	newVersion, err := Open("1.1.0", map[string]bool{"private": true})
	if err != nil {
		t.Fatal(err)
	}
	if result, parseErr := newVersion.ParseFile(source); parseErr != nil || result.Hit {
		t.Fatalf("version result = %#v, err = %v", result, parseErr)
	}

	newSettings, err := Open("1.0.0", map[string]bool{"private": false})
	if err != nil {
		t.Fatal(err)
	}
	if result, parseErr := newSettings.ParseFile(source); parseErr != nil || result.Hit {
		t.Fatalf("settings result = %#v, err = %v", result, parseErr)
	}
}

func TestCorruptEntryIsReparsed(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	t.Setenv(cacheDirectoryEnv, root)
	source := filepath.Join(t.TempDir(), "Type.java")
	writeSource(t, source, "package sample; class Type {}")
	cache, err := Open("1.0.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ParseFile(source); err != nil {
		t.Fatal(err)
	}
	entries, err := filepath.Glob(filepath.Join(root, "java", "*.json"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %#v, err = %v", entries, err)
	}
	if err := os.WriteFile(entries[0], []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := cache.ParseFile(source)
	if err != nil || result.Hit || result.CacheError == nil || len(result.Types) != 1 {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	result, err = cache.ParseFile(source)
	if err != nil || !result.Hit {
		t.Fatalf("recovered = %#v, err = %v", result, err)
	}
}

func TestCleanRemovesOnlyConfiguredRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	t.Setenv(cacheDirectoryEnv, root)
	if _, err := Open("1.0.0", nil); err != nil {
		t.Fatal(err)
	}
	writeSource(t, filepath.Join(root, "java", "entry.json"), "{}")

	got, err := Clean()
	if err != nil || got != root {
		t.Fatalf("Clean() = %q, %v", got, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("cache still exists: %v", err)
	}
}

func TestCleanRejectsUnrecognizedDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "documents")
	t.Setenv(cacheDirectoryEnv, root)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(root, "keep.txt")
	writeSource(t, keep, "keep")

	if _, err := Clean(); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("unrecognized directory was modified: %v", err)
	}
}

func writeSource(t *testing.T, path, source string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}
