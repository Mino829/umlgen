package gitdiff

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Mino829/umlgen/internal/model"
)

type Result struct {
	Root    string
	Current map[string]model.ChangeKind
	// Deleted holds repository-relative Git paths; their blobs are loaded on demand.
	Deleted []string
	base    string
}

func Analyze(workingDir, rangeSpec string) (Result, error) {
	rootOutput, err := runGit(workingDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Result{}, fmt.Errorf("not inside a Git repository: %w", err)
	}
	root := string(bytes.TrimSuffix(rootOutput, []byte{'\n'}))
	statusOutput, err := runGit(root, "diff", "--name-status", "-z", "--find-renames", rangeSpec, "--", "*.java", "*.go")
	if err != nil {
		return Result{}, fmt.Errorf("failed to inspect Git diff %q: %w", rangeSpec, err)
	}
	base, err := baseRevision(root, rangeSpec)
	if err != nil {
		return Result{}, err
	}

	result := Result{Root: root, Current: map[string]model.ChangeKind{}, base: base}
	for len(statusOutput) != 0 {
		status, rest, statusOK := bytes.Cut(statusOutput, []byte{0})
		path, rest, pathOK := bytes.Cut(rest, []byte{0})
		if !statusOK || !pathOK || len(status) == 0 || len(path) == 0 {
			return Result{}, fmt.Errorf("invalid Git diff status for %q", rangeSpec)
		}
		statusOutput = rest
		switch status[0] {
		case 'A':
			result.Current[relative(string(path))] = model.Added
		case 'M', 'T':
			result.Current[relative(string(path))] = model.Modified
		case 'R', 'C':
			destination, rest, ok := bytes.Cut(statusOutput, []byte{0})
			if !ok || len(destination) == 0 {
				return Result{}, fmt.Errorf("missing Git diff destination for %q", rangeSpec)
			}
			statusOutput = rest
			result.Current[relative(string(destination))] = model.Modified
		case 'D':
			result.Deleted = append(result.Deleted, string(path))
		}
	}
	untracked, err := runGit(root, "ls-files", "-z", "--others", "--exclude-standard", "--", "*.java", "*.go")
	if err != nil {
		return Result{}, fmt.Errorf("failed to inspect untracked source files: %w", err)
	}
	for path := range bytes.SplitSeq(untracked, []byte{0}) {
		if len(path) != 0 {
			result.Current[relative(string(path))] = model.Added
		}
	}
	if len(result.Current) == 0 && len(result.Deleted) == 0 {
		return Result{}, fmt.Errorf("no changed Java or Go files found in Git diff: %s", rangeSpec)
	}
	return result, nil
}

func (r Result) ChangeFor(path string) (model.ChangeKind, bool) {
	if change, ok := r.changeFor(r.Root, path); ok {
		return change, true
	}

	root, rootErr := filepath.EvalSymlinks(r.Root)
	source, sourceErr := filepath.EvalSymlinks(path)
	if rootErr != nil || sourceErr != nil {
		return "", false
	}
	return r.changeFor(root, source)
}

func (r Result) changeFor(root, path string) (model.ChangeKind, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", false
	}
	change, ok := r.Current[relative(rel)]
	return change, ok
}

func baseRevision(root, rangeSpec string) (string, error) {
	if before, after, ok := strings.Cut(rangeSpec, "..."); ok {
		output, err := runGit(root, "merge-base", before, after)
		if err != nil {
			return "", fmt.Errorf("failed to find merge base for %q: %w", rangeSpec, err)
		}
		return strings.TrimSpace(string(output)), nil
	}
	if before, _, ok := strings.Cut(rangeSpec, ".."); ok {
		return before, nil
	}
	return rangeSpec, nil
}

func relative(path string) string {
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
}

// ReadDeleted loads the base blob for a path from Deleted.
func (r Result) ReadDeleted(path string) ([]byte, error) {
	content, err := runGit(r.Root, "show", r.base+":"+path)
	if err != nil {
		return nil, fmt.Errorf("failed to load deleted source file %q: %w", path, err)
	}
	return content, nil
}

func runGit(dir string, args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	command.Dir = dir
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return nil, fmt.Errorf("%s", message)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}
