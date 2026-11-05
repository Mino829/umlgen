// Package gitsnapshot loads complete source sets from immutable Git commits.
// It never reads source or module content from the index or working tree, and
// disables mutable replacement refs when reading pinned objects.
package gitsnapshot

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Mino829/umlgen/internal/golang"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/scanner"
)

// File contains repository-stable absolute paths, not extraction paths. Content
// is owned by the pair and must be treated as read-only; identical blobs can share
// storage. Context is the selected source root (used for Java package identity).
type File struct {
	Path    string
	Content []byte
	Context string
}

// Snapshot contains one complete selected source set and its immutable commit.
type Snapshot struct {
	Revision string
	Files    []File
	root     string
	modules  map[string]string
}

type Pair struct {
	Root          string
	Before, After Snapshot
}

type entry struct {
	mode, kind, oid string
}

type target struct {
	path     string // slash-separated repository-relative path; empty denotes root
	scanPath string // invocation spelling, matching scanner exclusion semantics
}

type selection struct {
	files   []File
	oids    []string
	modules map[string]string // absolute module directory to blob ID
}

// Load interprets rev as rev..HEAD, A..B as explicit endpoints, and A...B as
// merge-base(A,B)..B. Omitted range endpoints mean HEAD. Targets are checked in
// both trees, so deleted paths work without consulting the current filesystem.
// Directory scanning follows scanner exclusions, including its explicit-file
// override. Selected source symlinks and submodules are rejected, never followed.
func Load(rangeSpec string, targets, excludes []string) (Pair, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Pair{}, fmt.Errorf("find invocation directory: %w", err)
	}
	rootOutput, err := runGit(cwd, "--no-replace-objects", "rev-parse", "--show-toplevel")
	if err != nil {
		return Pair{}, fmt.Errorf("not inside a Git working tree: %w", err)
	}
	root := strings.TrimSuffix(string(rootOutput), "\n")
	if !filepath.IsAbs(root) || root == "" {
		return Pair{}, fmt.Errorf("Git returned an invalid repository root %q", root)
	}
	// Resolve only the existing invocation/root directories. Target identities
	// themselves are lexical Git paths and may not exist in the working tree.
	physicalCwd, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return Pair{}, fmt.Errorf("resolve invocation directory: %w", err)
	}
	physicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Pair{}, fmt.Errorf("resolve repository root: %w", err)
	}
	selectedTargets, err := normalizeTargets(cwd, physicalCwd, root, physicalRoot, targets)
	if err != nil {
		return Pair{}, err
	}
	root = physicalRoot
	before, after, err := resolveRange(root, rangeSpec)
	if err != nil {
		return Pair{}, err
	}
	beforeTree, err := readTree(root, before)
	if err != nil {
		return Pair{}, err
	}
	afterTree := beforeTree
	if after != before {
		afterTree, err = readTree(root, after)
		if err != nil {
			return Pair{}, err
		}
	}
	for _, target := range selectedTargets {
		_, inBefore := beforeTree[target.path]
		_, inAfter := afterTree[target.path]
		if !inBefore && !inAfter {
			return Pair{}, fmt.Errorf("target does not exist in either Git snapshot: %s", target.scanPath)
		}
	}
	oldSelection, err := selectFiles(root, beforeTree, selectedTargets, excludes)
	if err != nil {
		return Pair{}, fmt.Errorf("snapshot %s: %w", before, err)
	}
	newSelection, err := selectFiles(root, afterTree, selectedTargets, excludes)
	if err != nil {
		return Pair{}, fmt.Errorf("snapshot %s: %w", after, err)
	}
	needed := map[string]bool{}
	for _, selected := range []selection{oldSelection, newSelection} {
		for _, oid := range selected.oids {
			needed[oid] = true
		}
		for _, oid := range selected.modules {
			needed[oid] = true
		}
	}
	blobs, err := readBlobs(root, needed)
	if err != nil {
		return Pair{}, err
	}
	makeSnapshot := func(revision string, selected selection) Snapshot {
		snapshot := Snapshot{Revision: revision, Files: selected.files, root: root, modules: map[string]string{}}
		for i := range snapshot.Files {
			snapshot.Files[i].Content = blobs[selected.oids[i]]
		}
		for directory, oid := range selected.modules {
			snapshot.modules[directory] = golang.ModuleName(blobs[oid])
		}
		return snapshot
	}
	return Pair{Root: root, Before: makeSnapshot(before, oldSelection), After: makeSnapshot(after, newSelection)}, nil
}

func normalizeTargets(cwd, physicalCwd, root, physicalRoot string, targets []string) ([]target, error) {
	if len(targets) == 0 {
		targets = []string{"."}
	}
	result := make([]target, 0, len(targets))
	for _, path := range targets {
		absolute := path
		if !filepath.IsAbs(path) {
			absolute = filepath.Join(physicalCwd, path)
		} else {
			// Accommodate an invocation through a symlinked repository root
			// without resolving possibly deleted target paths on disk.
			for _, alias := range [][2]string{{cwd, physicalCwd}, {root, physicalRoot}} {
				if relative, ok := within(alias[0], absolute); ok {
					absolute = filepath.Join(alias[1], relative)
					break
				}
			}
		}
		relative, ok := within(physicalRoot, absolute)
		if !ok {
			relative, ok = throughRootAlias(physicalRoot, absolute)
		}
		if !ok {
			return nil, fmt.Errorf("target is outside the Git repository: %s", path)
		}
		if relative == "." {
			relative = ""
		}
		result = append(result, target{path: filepath.ToSlash(relative), scanPath: path})
	}
	return result, nil
}

func within(root, path string) (string, bool) {
	relative, err := filepath.Rel(root, path)
	return relative, err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// Absolute configuration paths can use a different alias of the repository
// root (for example /var versus /private/var on macOS). Resolve only an alias
// of the entire repository, preserving every Git-relative component below it.
// Check parents first so a selected source symlink back to the root is still
// selected as that symlink, not silently widened to the entire repository.
func throughRootAlias(root, absolute string) (string, bool) {
	for ancestor := filepath.Dir(absolute); ; ancestor = filepath.Dir(ancestor) {
		if physical, err := filepath.EvalSymlinks(ancestor); err == nil && physical == root {
			relative, err := filepath.Rel(ancestor, absolute)
			return relative, err == nil
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
	if physical, err := filepath.EvalSymlinks(absolute); err == nil && physical == root {
		return ".", true
	}
	return "", false
}

func resolveRange(root, spec string) (string, string, error) {
	if spec == "" {
		return "", "", fmt.Errorf("Git revision range must not be empty")
	}
	left, right, mergeBase := spec, "HEAD", false
	if a, b, ok := strings.Cut(spec, "..."); ok {
		left, right, mergeBase = a, b, true
	} else if a, b, ok := strings.Cut(spec, ".."); ok {
		left, right = a, b
	}
	if strings.Contains(left, "..") || strings.Contains(right, "..") {
		return "", "", fmt.Errorf("invalid Git revision range %q", spec)
	}
	if left == "" {
		left = "HEAD"
	}
	if right == "" {
		right = "HEAD"
	}
	resolved := map[string]string{}
	resolve := func(revision string) (string, error) {
		if sha, ok := resolved[revision]; ok {
			return sha, nil
		}
		output, err := runGit(root, "--no-replace-objects", "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
		if err != nil {
			return "", fmt.Errorf("resolve Git revision %q: %w", revision, err)
		}
		sha := strings.TrimSuffix(string(output), "\n")
		if !validOID(sha) {
			return "", fmt.Errorf("Git returned an invalid commit ID for %q", revision)
		}
		resolved[revision] = sha
		return sha, nil
	}
	before, err := resolve(left)
	if err != nil {
		return "", "", err
	}
	after, err := resolve(right)
	if err != nil {
		return "", "", err
	}
	if mergeBase {
		output, err := runGit(root, "--no-replace-objects", "merge-base", "--all", before, after)
		if err != nil {
			return "", "", fmt.Errorf("find Git merge base for %q: %w", spec, err)
		}
		before = strings.TrimSuffix(string(output), "\n")
		if !validOID(before) {
			return "", "", fmt.Errorf("Git range %q does not have one unambiguous merge base", spec)
		}
	}
	return before, after, nil
}

func validOID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func readTree(root, revision string) (map[string]entry, error) {
	output, err := runGit(root, "--no-replace-objects", "ls-tree", "-r", "-t", "-z", "--full-tree", revision)
	if err != nil {
		return nil, fmt.Errorf("read Git tree %s: %w", revision, err)
	}
	tree := map[string]entry{"": {mode: "040000", kind: "tree"}}
	for len(output) > 0 {
		record, rest, ok := bytes.Cut(output, []byte{0})
		if !ok {
			return nil, fmt.Errorf("invalid unterminated Git tree entry")
		}
		output = rest
		header, name, ok := bytes.Cut(record, []byte{'\t'})
		fields := bytes.Fields(header)
		if !ok || len(fields) != 3 || len(name) == 0 || !validOID(string(fields[2])) {
			return nil, fmt.Errorf("invalid Git tree entry %q", record)
		}
		tree[string(name)] = entry{mode: string(fields[0]), kind: string(fields[1]), oid: string(fields[2])}
	}
	return tree, nil
}

func supported(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".java") || strings.HasSuffix(lower, ".go") && !strings.HasSuffix(lower, "_test.go")
}

func selectFiles(root string, tree map[string]entry, targets []target, excludes []string) (selection, error) {
	selected := map[string]bool{}
	treePaths := make([]string, 0, len(tree))
	for path := range tree {
		treePaths = append(treePaths, path)
	}
	sort.Strings(treePaths)
	var roots []string
	for _, target := range targets {
		item, exists := tree[target.path]
		if !exists {
			continue
		}
		absolute := filepath.Join(root, filepath.FromSlash(target.path))
		if item.kind != "tree" {
			if item.mode != "100644" && item.mode != "100755" {
				return selection{}, fmt.Errorf("unsupported Git input %q (mode %s; symlinks and submodules are not supported)", target.scanPath, item.mode)
			}
			// Explicit _test.go files are valid targets, but never source inputs.
			extension := strings.ToLower(filepath.Ext(target.path))
			if extension != ".java" && extension != ".go" {
				return selection{}, fmt.Errorf("target is not a supported source file or directory: %s", target.scanPath)
			}
			roots = append(roots, filepath.Dir(absolute))
			if supported(target.path) {
				selected[target.path] = true
			}
			continue
		}
		roots = append(roots, absolute)
		prefix := target.path
		if prefix != "" {
			prefix += "/"
		}
		for _, path := range treePaths {
			child := tree[path]
			if path == target.path || !strings.HasPrefix(path, prefix) || child.kind == "tree" {
				continue
			}
			relative := strings.TrimPrefix(path, prefix)
			scanPath := filepath.Join(target.scanPath, filepath.FromSlash(relative))
			if scanner.ExcludedPath(scanPath, excludes) {
				continue
			}
			ignored := false
			for directory := filepath.Dir(filepath.FromSlash(relative)); directory != "."; directory = filepath.Dir(directory) {
				if scanner.ExcludedDirectory(filepath.Join(target.scanPath, directory), excludes) {
					ignored = true
					break
				}
			}
			if ignored {
				continue
			}
			if child.kind == "commit" || supported(path) {
				if child.mode != "100644" && child.mode != "100755" {
					return selection{}, fmt.Errorf("unsupported Git input %q (mode %s; symlinks and submodules are not supported)", path, child.mode)
				}
				selected[path] = true
			}
		}
	}
	sort.Slice(roots, func(i, j int) bool {
		if len(roots[i]) != len(roots[j]) {
			return len(roots[i]) < len(roots[j])
		}
		return roots[i] < roots[j]
	})
	paths := make([]string, 0, len(selected))
	for path := range selected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := selection{modules: map[string]string{}}
	for _, path := range paths {
		absolute := filepath.Join(root, filepath.FromSlash(path))
		context := filepath.Dir(absolute)
		for _, candidate := range roots {
			if _, ok := within(candidate, absolute); ok {
				context = candidate
				break
			}
		}
		result.files = append(result.files, File{Path: absolute, Context: context})
		result.oids = append(result.oids, tree[path].oid)
		if strings.EqualFold(filepath.Ext(path), ".go") {
			for directory := filepath.Dir(filepath.FromSlash(path)); ; directory = filepath.Dir(directory) {
				modulePath := filepath.ToSlash(filepath.Join(directory, "go.mod"))
				if module, ok := tree[modulePath]; ok {
					if module.mode != "100644" && module.mode != "100755" {
						return selection{}, fmt.Errorf("unsupported Git module file %q (mode %s)", modulePath, module.mode)
					}
					result.modules[filepath.Join(root, directory)] = module.oid
				}
				if directory == "." {
					break
				}
			}
		}
	}
	return result, nil
}

// readBlobs uses object IDs only, so newline/tab bytes in paths never enter the
// line-oriented batch protocol. Both snapshots share a single Git process and
// a single allocation for each distinct selected source/module blob.
func readBlobs(root string, needed map[string]bool) (map[string][]byte, error) {
	blobs := make(map[string][]byte, len(needed))
	if len(needed) == 0 {
		return blobs, nil
	}
	oids := make([]string, 0, len(needed))
	for oid := range needed {
		oids = append(oids, oid)
	}
	sort.Strings(oids)
	command := exec.Command("git", "--no-replace-objects", "cat-file", "--batch")
	command.Dir = root
	command.Stdin = strings.NewReader(strings.Join(oids, "\n") + "\n")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open Git blob reader: %w", err)
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start Git blob reader: %w", err)
	}
	fail := func(err error) (map[string][]byte, error) {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, fmt.Errorf("read Git blobs: %w", err)
	}
	reader := bufio.NewReader(stdout)
	for _, oid := range oids {
		header, err := reader.ReadString('\n')
		if err != nil {
			return fail(err)
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[0] != oid || fields[1] != "blob" {
			return fail(fmt.Errorf("unexpected object response %q", header))
		}
		size, err := strconv.ParseInt(fields[2], 10, 0)
		if err != nil || size < 0 {
			return fail(fmt.Errorf("invalid blob size %q", fields[2]))
		}
		content := make([]byte, int(size))
		if _, err := io.ReadFull(reader, content); err != nil {
			return fail(err)
		}
		terminator, err := reader.ReadByte()
		if err != nil || terminator != '\n' {
			return fail(fmt.Errorf("invalid blob terminator for %s", oid))
		}
		blobs[oid] = content
	}
	if err := command.Wait(); err != nil {
		return nil, fmt.Errorf("read Git blobs: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return blobs, nil
}

// BindContext applies snapshot module identities after parsing/cache retrieval.
// It is pure: no disk reads, current go.mod probing, or temporary extraction.
// An empty/malformed nearest module masks parent modules, like the Go parser.
func (s Snapshot) BindContext(unit *model.SourceUnit) {
	if unit == nil {
		return
	}
	unit.PackagePath = unit.Package.Name
	unit.PackageContext = ""
	if unit.Language == "go" {
		directory := filepath.Dir(unit.Path)
		unit.PackageContext = directory
		if _, ok := within(s.root, directory); s.root != "" && ok {
			for current := directory; ; current = filepath.Dir(current) {
				if module, exists := s.modules[current]; exists {
					if module != "" {
						relative, _ := filepath.Rel(current, directory)
						unit.PackagePath, unit.PackageContext = module, current
						if relative != "." {
							unit.PackagePath = strings.TrimSuffix(module, "/") + "/" + filepath.ToSlash(relative)
						}
					}
					break
				}
				if current == s.root {
					break
				}
			}
		}
	}
	for i := range unit.Types {
		unit.Types[i].Package = unit.PackagePath
	}
}

func runGit(root string, args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	command.Dir = root
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
