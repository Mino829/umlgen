package javacache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/golang"
	"github.com/Mino829/umlgen/internal/relations"
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

func TestOpenForCachesGoSource(t *testing.T) {
	t.Setenv(cacheDirectoryEnv, filepath.Join(t.TempDir(), "cache"))
	source := filepath.Join(t.TempDir(), "user.go")
	writeSource(t, source, "package sample\ntype User struct{}\n")

	cache, err := OpenFor("1.0.0", "go", golang.SchemaVersion, nil, golang.ParseSource)
	if err != nil {
		t.Fatal(err)
	}
	first, err := cache.ParseFile(source)
	if err != nil || first.Hit || len(first.Types) != 1 || first.Types[0].Name != "User" {
		t.Fatalf("first = %#v, err = %v", first, err)
	}
	second, err := cache.ParseFile(source)
	if err != nil || !second.Hit || second.Types[0].Source != source {
		t.Fatalf("second = %#v, err = %v", second, err)
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

	newSchema, err := OpenFor("1.0.0", "java", "new-parser-schema", map[string]bool{"private": true}, initial.parse)
	if err != nil {
		t.Fatal(err)
	}
	if result, parseErr := newSchema.ParseFile(source); parseErr != nil || result.Hit {
		t.Fatalf("schema result = %#v, err = %v", result, parseErr)
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

func TestCacheRebindsExplanationLocationsForIdenticalFiles(t *testing.T) {
	t.Setenv(cacheDirectoryEnv, filepath.Join(t.TempDir(), "cache"))
	dir := t.TempDir()
	const content = `package sample;
class Target {}
interface Contract {}
class Service extends Target implements Contract {
    Target target;
    Target load(
        Target input) { return null; }
}`
	cache, err := Open("explain", nil)
	if err != nil {
		t.Fatal(err)
	}
	wantLines := map[relations.Kind]int{
		relations.Inheritance: 4, relations.Implementation: 4,
		relations.Field: 5, relations.Parameter: 7, relations.Return: 6,
	}
	for i, name := range []string{"First.java", "Second.java"} {
		path := filepath.Join(dir, name)
		writeSource(t, path, content)
		result, err := cache.ParseFile(path)
		if err != nil || result.Hit != (i == 1) {
			t.Fatalf("%s: hit=%v err=%v", name, result.Hit, err)
		}
		got := relations.Analyze(result.Types, nil).Relations
		if len(got) != len(wantLines) {
			t.Fatalf("%s relations = %#v", name, got)
		}
		for _, relation := range got {
			if len(relation.Evidence) != 1 {
				t.Fatalf("%s evidence = %#v", name, relation.Evidence)
			}
			evidence := relation.Evidence[0]
			if evidence.Source != path || evidence.Line != wantLines[relation.Kind] {
				t.Errorf("%s %s location = %s:%d, want %s:%d", name, relation.Kind,
					evidence.Source, evidence.Line, path, wantLines[relation.Kind])
			}
		}
	}
}

func TestCapturedSourceAndCurrentFileDoNotShareStaleCacheContent(t *testing.T) {
	t.Setenv(cacheDirectoryEnv, filepath.Join(t.TempDir(), "cache"))
	path := filepath.Join(t.TempDir(), "Type.java")
	captured := []byte("package sample; class Captured {}")
	writeSource(t, path, "package sample; class Current {}")
	cache, err := Open("snapshots", nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := cache.ParseSource(path, captured)
	if err != nil || snapshot.Hit || len(snapshot.Types) != 1 || snapshot.Types[0].Name != "Captured" {
		t.Fatalf("snapshot = %#v err=%v", snapshot, err)
	}
	current, err := cache.ParseFile(path)
	if err != nil || current.Hit || len(current.Types) != 1 || current.Types[0].Name != "Current" {
		t.Fatalf("current = %#v err=%v", current, err)
	}
	warm, err := cache.ParseSource(path, captured)
	if err != nil || !warm.Hit || len(warm.Types) != 1 || warm.Types[0].Name != "Captured" {
		t.Fatalf("warm snapshot = %#v err=%v", warm, err)
	}
}
