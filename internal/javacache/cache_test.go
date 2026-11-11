package javacache

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/golang"
	"github.com/Mino829/umlgen/internal/java"
	"github.com/Mino829/umlgen/internal/model"
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
	if err != nil || first.Hit || len(first.Unit.Types) != 1 || first.Unit.Types[0].Name != "First" {
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
	if err != nil || !second.Hit || second.Unit.Types[0].Source != source {
		t.Fatalf("second = %#v, err = %v", second, err)
	}

	writeSource(t, source, "package sample; class Second {}")
	changed, err := cache.ParseFile(source)
	if err != nil || changed.Hit || changed.Unit.Types[0].Name != "Second" {
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
	if err != nil || first.Hit || len(first.Unit.Types) != 1 || first.Unit.Types[0].Name != "User" {
		t.Fatalf("first = %#v, err = %v", first, err)
	}
	second, err := cache.ParseFile(source)
	if err != nil || !second.Hit || second.Unit.Types[0].Source != source {
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
	if err != nil || result.Hit || result.CacheError == nil || len(result.Unit.Types) != 1 {
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
		got := relations.Analyze(relations.Input{Types: result.Unit.Types}, true).Relations
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
	if err != nil || snapshot.Hit || len(snapshot.Unit.Types) != 1 || snapshot.Unit.Types[0].Name != "Captured" {
		t.Fatalf("snapshot = %#v err=%v", snapshot, err)
	}
	current, err := cache.ParseFile(path)
	if err != nil || current.Hit || len(current.Unit.Types) != 1 || current.Unit.Types[0].Name != "Current" {
		t.Fatalf("current = %#v err=%v", current, err)
	}
	warm, err := cache.ParseSource(path, captured)
	if err != nil || !warm.Hit || len(warm.Unit.Types) != 1 || warm.Unit.Types[0].Name != "Captured" {
		t.Fatalf("warm snapshot = %#v err=%v", warm, err)
	}
}

func TestSourceUnitCachePreservesFileScopeAtAnotherPath(t *testing.T) {
	for _, scenario := range []struct {
		name, language, schema, source string
		parse                          func(string, []byte) (*model.SourceUnit, error)
	}{
		{
			name: "Go functions only", language: "go", schema: golang.SchemaVersion,
			source: "package demo\r\nimport alias \"example.com/dependency\"\r\nfunc Run() { /* source body must not persist */ }",
			parse:  golang.ParseSource,
		},
		{
			name: "Go aliases only", language: "go", schema: golang.SchemaVersion,
			source: "package demo\nimport dep \"example.com/dependency\"\ntype Handler = func(dep.Input)\ntype ID = string",
			parse:  golang.ParseSource,
		},
		{
			name: "Go generic scopes and receiver projections", language: "go", schema: golang.SchemaVersion,
			source: "package demo\ntype ID = string\ntype Named string\ntype Box[T any] struct { Value T }\nfunc (b Box[U]) Get(value U) U { return value }\nfunc (o Other[V]) Take(value V) {}",
			parse:  golang.ParseSource,
		},
		{
			name: "Java package only", language: "java", schema: java.SchemaVersion,
			source: "package demo;\r\nimport example.dependency.Outer.Inner;",
			parse:  java.ParseSource,
		},
		{
			name: "Java multiple types", language: "java", schema: java.SchemaVersion,
			source: "package demo;\nimport example.dependency.Target;\nclass A { Target value; }\nclass B {}",
			parse:  java.ParseSource,
		},
		{
			name: "Java generic declaration and member scopes", language: "java", schema: java.SchemaVersion,
			source: "package demo; class Box<T extends Bound<T>> { T value; <U> Box(U value) {} <V> V get(V value) { return value; } class Inner<U> { T outer; U own; } static class Static { T concrete; } }",
			parse:  java.ParseSource,
		},
		{
			name: "Java module diagnostic", language: "java", schema: java.SchemaVersion,
			source: "open module demo { requires java.base; }",
			parse:  java.ParseSource,
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Setenv(cacheDirectoryEnv, filepath.Join(t.TempDir(), "cache"))
			cache, err := OpenFor("source-units", scenario.language, scenario.schema, nil, scenario.parse)
			if err != nil {
				t.Fatal(err)
			}
			source := []byte(scenario.source)
			firstPath := filepath.Join(t.TempDir(), "first")
			first, err := cache.ParseSource(firstPath, source)
			if err != nil || first.Hit {
				t.Fatalf("initial=%#v err=%v", first, err)
			}
			uncachedFirst, err := scenario.parse(firstPath, source)
			if err != nil || !reflect.DeepEqual(first.Unit, uncachedFirst) {
				t.Fatalf("cold cache differs from uncached syntax: got %#v, want %#v, err=%v", first.Unit, uncachedFirst, err)
			}
			secondPath := filepath.Join(t.TempDir(), "second")
			want, err := scenario.parse(secondPath, source)
			if err != nil {
				t.Fatal(err)
			}
			second, err := cache.ParseSource(secondPath, source)
			if err != nil || !second.Hit || second.CacheError != nil {
				t.Fatalf("cached=%#v err=%v", second, err)
			}
			if !reflect.DeepEqual(second.Unit, want) {
				t.Fatalf("cached unit differs from current-path syntax:\n got %#v\nwant %#v", second.Unit, want)
			}
			for _, typ := range second.Unit.Types {
				if typ.Source != secondPath || !reflect.DeepEqual(typ.Imports, second.Unit.Imports) {
					t.Fatalf("lost declaration file scope: %#v", typ)
				}
			}
		})
	}
}

func TestOpenPreservesExistingLanguageDirectoryPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not implement POSIX directory permission bits")
	}
	for _, mode := range []os.FileMode{0o750, 0o500} {
		t.Run(mode.String(), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "cache")
			dir := filepath.Join(root, "java")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, mode); err != nil {
				t.Fatal(err)
			}
			t.Setenv(cacheDirectoryEnv, root)
			if _, err := Open("permissions", nil); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(dir)
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Mode().Perm(); got != mode {
				t.Fatalf("existing cache directory permissions = %04o, want %04o", got, mode)
			}
		})
	}
}

func TestOpenCreatesPrivateLanguageDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not implement POSIX directory permission bits")
	}
	root := filepath.Join(t.TempDir(), "cache")
	t.Setenv(cacheDirectoryEnv, root)
	if _, err := Open("permissions", nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "java"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("new cache directory is not private: %s", info.Mode())
	}
}
