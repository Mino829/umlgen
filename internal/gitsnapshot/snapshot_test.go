package gitsnapshot_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/gitsnapshot"
	"github.com/Mino829/umlgen/internal/golang"
	"github.com/Mino829/umlgen/internal/javacache"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/scanner"
)

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_DATE=2025-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2025-01-01T00:00:00Z")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %q: %v\n%s", args, err, output)
	}
	return strings.TrimSuffix(string(output), "\n")
}

func repository(t *testing.T) string {
	t.Helper()
	// POSIX also exercises root paths ending in LF; Windows forbids control
	// characters in filenames, but still covers spaces and Unicode below.
	name := "repository with space"
	if runtime.GOOS != "windows" {
		name += "\n"
	}
	root := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	git(t, physical, "init", "-q")
	git(t, physical, "config", "user.name", "Snapshot Test")
	git(t, physical, "config", "user.email", "snapshot@example.invalid")
	git(t, physical, "config", "commit.gpgsign", "false")
	t.Chdir(physical)
	return physical
}

func put(t *testing.T, root, path, content string) {
	t.Helper()
	absolute := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, root, message string) string {
	t.Helper()
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", message)
	return git(t, root, "rev-parse", "HEAD")
}

// Construct a real Git symlink entry without requiring OS symlink privileges.
// The loader must honor the pinned tree mode, not current working-tree shape.
func commitTreeSymlink(t *testing.T, root, path, target string) string {
	t.Helper()
	put(t, root, path, target)
	git(t, root, "add", "-A")
	blob := git(t, root, "hash-object", "-w", "--", path)
	git(t, root, "update-index", "--add", "--cacheinfo", "120000,"+blob+","+path)
	git(t, root, "commit", "-q", "-m", "snapshot symlink")
	return git(t, root, "rev-parse", "HEAD")
}

func files(snapshot gitsnapshot.Snapshot) map[string]string {
	result := map[string]string{}
	for _, file := range snapshot.Files {
		result[file.Path] = string(file.Content)
	}
	return result
}

func load(t *testing.T, spec string, targets, excludes []string) gitsnapshot.Pair {
	t.Helper()
	pair, err := gitsnapshot.Load(spec, targets, excludes)
	if err != nil {
		t.Fatal(err)
	}
	return pair
}

func TestCommittedSourcesAndModuleRenameIgnoreAllWorkingState(t *testing.T) {
	root := repository(t)
	const source = "package service\ntype Service struct{}\n"
	put(t, root, "go.mod", "module example.com/old\n")
	put(t, root, "service/service.go", source)
	put(t, root, "service/deleted.go", "package service\ntype Deleted struct{}\n")
	before := commit(t, root, "before")
	put(t, root, "go.mod", "module example.com/renamed\n")
	if err := os.Remove(filepath.Join(root, "service/deleted.go")); err != nil {
		t.Fatal(err)
	}
	put(t, root, "added/added.go", "package added\ntype Added struct{}\n")
	after := commit(t, root, "after")
	put(t, root, "go.mod", "module staged.invalid/module\n")
	put(t, root, "service/service.go", "package staged\n")
	put(t, root, "staged/new.go", "package staged\n")
	git(t, root, "add", "go.mod", "service/service.go", "staged/new.go")
	put(t, root, "go.mod", "module dirty.invalid/module\n")
	put(t, root, "service/service.go", "this is not committed Go")
	put(t, root, "untracked/untracked.go", "package untracked\n")
	pair := load(t, before, nil, []string{"go.mod"})
	if pair.Root != root || pair.Before.Revision != before || pair.After.Revision != after {
		t.Fatalf("unpinned identities: %#v", pair)
	}
	oldFiles, newFiles := files(pair.Before), files(pair.After)
	path := filepath.Join(root, "service/service.go")
	if oldFiles[path] != source || newFiles[path] != source || len(oldFiles) != 2 || len(newFiles) != 2 {
		t.Fatalf("source selection includes working state or omits unchanged files: before=%v after=%v", oldFiles, newFiles)
	}
	if _, ok := oldFiles[filepath.Join(root, "service/deleted.go")]; !ok {
		t.Fatal("deleted source not loaded from before tree")
	}
	if _, ok := newFiles[filepath.Join(root, "added/added.go")]; !ok {
		t.Fatal("new source not loaded from after tree")
	}
	// Bind after physically removing every source/module file. Snapshot identity
	// must remain usable independently of current disk state and parse caches.
	if err := os.RemoveAll(filepath.Join(root, "service")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "go.mod")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UMLGEN_CACHE_DIR", t.TempDir())
	cache, err := javacache.OpenFor("snapshot-test", "go", golang.SchemaVersion, nil, golang.ParseSource)
	if err != nil {
		t.Fatal(err)
	}
	for i, snapshot := range []gitsnapshot.Snapshot{pair.Before, pair.After} {
		wantPackage := []string{"example.com/old/service", "example.com/renamed/service"}[i]
		for _, file := range snapshot.Files {
			if file.Path != path {
				continue
			}
			direct, err := golang.ParseSource(file.Path, file.Content)
			if err != nil {
				t.Fatal(err)
			}
			snapshot.BindContext(direct)
			if direct.PackagePath != wantPackage || direct.PackageContext != root || direct.Types[0].Package != wantPackage {
				t.Fatalf("wrong immutable module binding: %#v", direct)
			}
			for pass := range 2 {
				result, err := cache.ParseSource(file.Path, file.Content)
				if err != nil || result.CacheError != nil {
					t.Fatalf("cache parse: %v, cache: %v", err, result.CacheError)
				}
				if result.Hit != (i > 0 || pass > 0) {
					t.Fatalf("unexpected cache hit on side %d pass %d: %v", i, pass, result.Hit)
				}
				snapshot.BindContext(result.Unit)
				if !reflect.DeepEqual(result.Unit, direct) {
					t.Fatalf("cold/warm/no-cache snapshot context differs: cached=%#v direct=%#v", result.Unit, direct)
				}
			}
		}
	}
	// A later commit cannot change bytes or contexts already returned by Load.
	put(t, root, "service/service.go", "package later\n")
	commit(t, root, "later")
	if pair.After.Revision != after || files(pair.After)[path] != source {
		t.Fatal("loaded snapshot changed after HEAD moved")
	}
}

func TestReplacementObjectsCannotRewritePinnedSnapshots(t *testing.T) {
	for _, kind := range []string{"commit", "tree", "source blob", "module blob"} {
		t.Run(kind, func(t *testing.T) {
			root := repository(t)
			const original = "package original\ntype Original struct{}\n"
			put(t, root, "go.mod", "module example.com/original\n")
			put(t, root, "source.go", original)
			before := commit(t, root, "original objects")
			put(t, root, "go.mod", "module example.com/replacement\n")
			put(t, root, "source.go", "package replacement\ntype Replacement struct{}\n")
			put(t, root, "Injected.java", "class Injected {}\n")
			after := commit(t, root, "replacement objects")
			oldObject, newObject := before, after
			switch kind {
			case "tree":
				oldObject = git(t, root, "rev-parse", before+"^{tree}")
				newObject = git(t, root, "rev-parse", after+"^{tree}")
			case "source blob":
				oldObject = git(t, root, "rev-parse", before+":source.go")
				newObject = git(t, root, "rev-parse", after+":source.go")
			case "module blob":
				oldObject = git(t, root, "rev-parse", before+":go.mod")
				newObject = git(t, root, "rev-parse", after+":go.mod")
			}
			git(t, root, "replace", oldObject, newObject)
			pair := load(t, before+".."+after, nil, nil)
			path := filepath.Join(root, "source.go")
			if pair.Before.Revision != before || len(pair.Before.Files) != 1 || files(pair.Before)[path] != original {
				t.Fatalf("replacement %s changed the pinned source tree: revision=%s files=%v", kind, pair.Before.Revision, files(pair.Before))
			}
			unit, err := golang.ParseSource(path, pair.Before.Files[0].Content)
			if err != nil {
				t.Fatal(err)
			}
			pair.Before.BindContext(unit)
			if unit.PackagePath != "example.com/original" || unit.PackageContext != root {
				t.Fatalf("replacement %s changed the pinned module identity: %#v", kind, unit)
			}
			if pair.After.Revision != after || len(pair.After.Files) != 2 {
				t.Fatalf("replacement %s changed the other endpoint: revision=%s files=%v", kind, pair.After.Revision, files(pair.After))
			}
		})
	}
}

func TestTargetsDeletedAndAbsentOnOneSide(t *testing.T) {
	root := repository(t)
	put(t, root, "old/Only.java", "package old; class Only {}")
	before := commit(t, root, "old target")
	if err := os.RemoveAll(filepath.Join(root, "old")); err != nil {
		t.Fatal(err)
	}
	put(t, root, "new/New.java", "package newer; class New {}")
	after := commit(t, root, "new target")
	for _, target := range []string{"old", "old/Only.java", filepath.Join(root, "old/Only.java")} {
		pair := load(t, before+".."+after, []string{target}, nil)
		if len(pair.Before.Files) != 1 || len(pair.After.Files) != 0 {
			t.Fatalf("deleted target %q not preserved: %#v", target, pair)
		}
		if pair.Before.Files[0].Context != filepath.Join(root, "old") {
			t.Fatalf("unstable deleted Java context: %q", pair.Before.Files[0].Context)
		}
		unit := &model.SourceUnit{Language: "java", Package: model.PackageDeclaration{Name: "old"}, Types: []model.Type{{Package: "stale"}}}
		pair.Before.BindContext(unit)
		if unit.PackagePath != "old" || unit.Types[0].Package != "old" {
			t.Fatalf("Java package not rebound: %#v", unit)
		}
	}
	pair := load(t, before+".."+after, []string{"new"}, nil)
	if len(pair.Before.Files) != 0 || len(pair.After.Files) != 1 {
		t.Fatalf("new-only target not preserved: %#v", pair)
	}
	if _, err := gitsnapshot.Load(before, []string{"missing"}, nil); err == nil || !strings.Contains(err.Error(), "either Git snapshot") {
		t.Fatalf("unknown target should fail: %v", err)
	}
	put(t, root, "untracked.java", "class Untracked {}")
	if _, err := gitsnapshot.Load(before, []string{"untracked.java"}, nil); err == nil {
		t.Fatal("untracked-only target unexpectedly accepted")
	}
}

func TestRangeEndpointsAndMergeBase(t *testing.T) {
	root := repository(t)
	put(t, root, "Base.java", "class Base {}")
	base := commit(t, root, "base")
	git(t, root, "checkout", "-q", "-b", "topic")
	put(t, root, "Topic.java", "class Topic {}")
	topic := commit(t, root, "topic")
	git(t, root, "checkout", "-q", "-b", "other", base)
	put(t, root, "Other.java", "class Other {}")
	other := commit(t, root, "other")
	git(t, root, "tag", "-a", "annotated", "-m", "annotated", topic)
	cases := []struct{ spec, before, after string }{
		{"topic", topic, other},
		{"topic..other", topic, other},
		{"topic...other", base, other},
		{"..topic", other, topic},
		{"topic..", topic, other},
		{"...topic", base, topic},
		{"topic...", base, other},
		{"..", other, other},
		{"...", other, other},
		{"annotated..other", topic, other},
	}
	for _, tc := range cases {
		t.Run(tc.spec, func(t *testing.T) {
			pair := load(t, tc.spec, nil, nil)
			if pair.Before.Revision != tc.before || pair.After.Revision != tc.after {
				t.Fatalf("range %q resolved to %s..%s, want %s..%s", tc.spec, pair.Before.Revision, pair.After.Revision, tc.before, tc.after)
			}
			if len(pair.Before.Files) == 0 || len(pair.After.Files) == 0 {
				t.Fatal("range unexpectedly selected only changed files")
			}
		})
	}
	for _, spec := range []string{"", "unknown-revision", "topic....other", "HEAD:Base.java", "--help"} {
		if _, err := gitsnapshot.Load(spec, nil, nil); err == nil {
			t.Fatalf("invalid/non-commit revision %q accepted", spec)
		}
	}
}

func TestBinarySafePathsAndContent(t *testing.T) {
	root := repository(t)
	contents := map[string]string{
		"dir with space/A file.java": "class A {}\n",
		"binary/Raw.java":            "class Raw {}\x00binary\n",
		"unicode/型.java":             "class 型 {}\n",
	}
	if runtime.GOOS != "windows" {
		contents["dir\nwith\ttabs/B\nname.java"] = "class B {}\n"
		contents["carriage\rreturn.go"] = "package carriage\n"
	}
	for path, content := range contents {
		put(t, root, path, content)
	}
	revision := commit(t, root, "binary safe paths")
	pair := load(t, revision, nil, nil)
	if len(pair.Before.Files) != len(contents) {
		t.Fatalf("lost special filenames: %#v", pair.Before.Files)
	}
	var paths []string
	for _, file := range pair.Before.Files {
		relative, err := filepath.Rel(root, file.Path)
		if err != nil || !bytes.Equal(file.Content, []byte(contents[filepath.ToSlash(relative)])) {
			t.Fatalf("corrupted file %q: %q, %v", file.Path, file.Content, err)
		}
		if !filepath.IsAbs(file.Path) || file.Context != root {
			t.Fatalf("non-stable path/context: %#v", file)
		}
		paths = append(paths, file.Path)
	}
	if !sort.StringsAreSorted(paths) {
		t.Fatalf("nondeterministic file order: %q", paths)
	}
	for path := range contents {
		single := load(t, revision, []string{path}, nil)
		if len(single.Before.Files) != 1 || string(single.Before.Files[0].Content) != contents[path] {
			t.Fatalf("explicit special target %q lost", path)
		}
	}
}

func TestExcludesMatchScannerAndExplicitOverrides(t *testing.T) {
	root := repository(t)
	for _, path := range []string{"Keep.java", "upper/UPPER.JAVA", "ok/main.go", "ok/main_test.go", "vendor/V.java", "build/B.java", "out/O.java", "node_modules/N.java", "skip/S.java", "nested/skip/T.java", "nested/deep/U.java"} {
		put(t, root, path, "package p\n")
	}
	revision := commit(t, root, "selection")
	excludes := []string{"skip", "nested/deep", "*.java", ".ignored"}
	pair := load(t, revision, []string{"."}, excludes)
	scanned, err := scanner.SourceFiles([]string{"."}, excludes, ".java", ".go")
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, path := range scanned {
		if !strings.HasSuffix(strings.ToLower(path), "_test.go") {
			want = append(want, path)
		}
	}
	var got []string
	for _, file := range pair.Before.Files {
		got = append(got, file.Path)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Git selection diverged from scanner: got %q want %q", got, want)
	}
	for _, target := range []string{"vendor", "skip", "vendor/V.java", "skip/S.java"} {
		pair := load(t, revision, []string{target}, excludes)
		if len(pair.Before.Files) != 1 {
			t.Fatalf("explicit target override differs from scanner for %q: %#v", target, pair.Before.Files)
		}
	}
	if pair := load(t, revision, []string{"ok/main_test.go"}, nil); len(pair.Before.Files) != 0 {
		t.Fatal("explicit Go test file should not enter source sets")
	}
	// Default target is the invocation directory, not the repository root.
	t.Chdir(filepath.Join(root, "ok"))
	pair = load(t, revision, nil, nil)
	if len(pair.Before.Files) != 1 || pair.Before.Files[0].Path != filepath.Join(root, "ok/main.go") {
		t.Fatalf("default target is not cwd: %#v", pair.Before.Files)
	}
}

func TestNestedSnapshotModulesAndOutsideParents(t *testing.T) {
	root := repository(t)
	put(t, filepath.Dir(root), "go.mod", "module outside.invalid/parent\n")
	put(t, root, "go.mod", "module example.com/root\n")
	put(t, root, "nested/go.mod", "module example.com/nested\n")
	put(t, root, "nested/pkg/a.go", "package pkg\ntype A struct{}\n")
	put(t, root, "broken/go.mod", "go 1.25\n")
	put(t, root, "broken/pkg/b.go", "package pkg\ntype B struct{}\n")
	before := commit(t, root, "nested modules")
	if err := os.Remove(filepath.Join(root, "go.mod")); err != nil {
		t.Fatal(err)
	}
	put(t, root, "plain/p.go", "package plain\ntype P struct{}\n")
	after := commit(t, root, "no root module")
	// A dirty root go.mod must not supply the deleted snapshot module.
	put(t, root, "go.mod", "module dirty.invalid/root\n")
	pair := load(t, before+".."+after, nil, []string{"go.mod"})
	for _, snapshot := range []gitsnapshot.Snapshot{pair.Before, pair.After} {
		for _, file := range snapshot.Files {
			unit, err := golang.ParseSource(file.Path, file.Content)
			if err != nil {
				t.Fatal(err)
			}
			snapshot.BindContext(unit)
			switch filepath.Base(file.Path) {
			case "a.go":
				if unit.PackagePath != "example.com/nested/pkg" || unit.PackageContext != filepath.Join(root, "nested") {
					t.Fatalf("nearest snapshot module not used: %#v", unit)
				}
			case "b.go":
				if unit.PackagePath != "pkg" || unit.PackageContext != filepath.Join(root, "broken/pkg") {
					t.Fatalf("malformed nearest module should mask ancestor: %#v", unit)
				}
			case "p.go":
				if unit.PackagePath != "plain" || unit.PackageContext != filepath.Join(root, "plain") {
					t.Fatalf("current/outside module leaked into snapshot: %#v", unit)
				}
			}
		}
	}
	// Modules outside an explicitly selected subtree are still required.
	narrow := load(t, before, []string{"nested/pkg/a.go"}, []string{"nested"})
	unit, err := golang.ParseSource(narrow.Before.Files[0].Path, narrow.Before.Files[0].Content)
	if err != nil {
		t.Fatal(err)
	}
	narrow.Before.BindContext(unit)
	if unit.PackagePath != "example.com/nested/pkg" {
		t.Fatalf("ancestor module omitted by source excludes: %#v", unit)
	}
}

func TestUnsupportedGitInputsFailClearly(t *testing.T) {
	t.Run("source symlink", func(t *testing.T) {
		root := repository(t)
		put(t, root, "Real.java", "class Real {}")
		revision := commitTreeSymlink(t, root, "Link.java", "Real.java")
		for _, targets := range [][]string{nil, {"Link.java"}} {
			if _, err := gitsnapshot.Load(revision, targets, nil); err == nil || !strings.Contains(err.Error(), "symlinks") {
				t.Fatalf("source symlink was not rejected: %v", err)
			}
		}
	})
	t.Run("directory symlink target", func(t *testing.T) {
		root := repository(t)
		put(t, root, "src/Real.java", "class Real {}")
		revision := commitTreeSymlink(t, root, "link", "src")
		if _, err := gitsnapshot.Load(revision, []string{"link"}, nil); err == nil || !strings.Contains(err.Error(), "symlinks") {
			t.Fatalf("directory symlink target was not rejected: %v", err)
		}
	})
	t.Run("submodule", func(t *testing.T) {
		root := repository(t)
		put(t, root, "Real.java", "class Real {}")
		base := commit(t, root, "base")
		git(t, root, "update-index", "--add", "--cacheinfo", "160000,"+base+",nested-module")
		git(t, root, "commit", "-q", "-m", "gitlink")
		for _, targets := range [][]string{nil, {"nested-module"}} {
			if _, err := gitsnapshot.Load("HEAD", targets, nil); err == nil || !strings.Contains(err.Error(), "submodules") {
				t.Fatalf("submodule was not rejected: %v", err)
			}
		}
		pair := load(t, "HEAD", nil, []string{"nested-module"})
		if len(pair.Before.Files) != 1 {
			t.Fatal("explicitly excluded submodule should not fail scanning")
		}
	})
	t.Run("module symlink", func(t *testing.T) {
		root := repository(t)
		put(t, root, "module.txt", "module example.com/unsafe\n")
		put(t, root, "a.go", "package p\n")
		revision := commitTreeSymlink(t, root, "go.mod", "module.txt")
		if _, err := gitsnapshot.Load(revision, []string{"a.go"}, nil); err == nil || !strings.Contains(err.Error(), "module file") {
			t.Fatalf("symlink module was not rejected: %v", err)
		}
	})
	t.Run("unsupported and outside targets", func(t *testing.T) {
		root := repository(t)
		put(t, root, "readme.txt", "text")
		revision := commit(t, root, "non-source")
		for _, target := range []string{"readme.txt", "../outside", filepath.Join(filepath.Dir(root), "outside")} {
			if _, err := gitsnapshot.Load(revision, []string{target}, nil); err == nil {
				t.Fatalf("invalid target %q accepted", target)
			}
		}
		pair := load(t, revision, nil, nil)
		if len(pair.Before.Files) != 0 || len(pair.After.Files) != 0 {
			t.Fatal("a valid tree with no source files should be a complete empty snapshot")
		}
	})
}

func TestAbsoluteRootAliasPreservesDeletedTargetsAndSourceSymlinkPolicy(t *testing.T) {
	root := repository(t)
	put(t, root, "src/A.java", "package sample; class A {}\n")
	before := commit(t, root, "selected source")
	if err := os.RemoveAll(filepath.Join(root, "src")); err != nil {
		t.Fatal(err)
	}
	put(t, root, "README.txt", "selected directory is now absent\n")
	after := commit(t, root, "delete selected directory")
	alias := filepath.Join(t.TempDir(), "repo-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("repository symlink unavailable: %v", err)
	}
	pair := load(t, before+".."+after, []string{filepath.Join(alias, "src")}, nil)
	path := filepath.Join(root, "src", "A.java")
	if len(pair.Before.Files) != 1 || pair.Before.Files[0].Path != path || len(pair.After.Files) != 0 {
		t.Fatalf("root alias changed immutable target selection: before=%v after=%v", files(pair.Before), files(pair.After))
	}
	pair = load(t, before+".."+after, []string{alias}, nil)
	if len(pair.Before.Files) != 1 || len(pair.After.Files) != 0 {
		t.Fatal("absolute repository alias did not select the complete source sets")
	}
	if err := os.Symlink(".", filepath.Join(root, "Bad.java")); err != nil {
		t.Fatal(err)
	}
	unsupported := commit(t, root, "source symlink back to root")
	if _, err := gitsnapshot.Load(after+".."+unsupported, []string{filepath.Join(alias, "Bad.java")}, nil); err == nil {
		t.Fatal("source symlink through a root alias was widened to a repository scan")
	}
}
