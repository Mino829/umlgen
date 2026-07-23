package gitdiff

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Mino829/umlgen/internal/model"
)

type DeletedSource struct {
	Path    string
	Content []byte
}

type Result struct {
	Root    string
	Current map[string]model.ChangeKind
	Deleted []DeletedSource
}

func Analyze(rangeSpec string) (Result, error) {
	rootOutput, err := runGit("", "rev-parse", "--show-toplevel")
	if err != nil {
		return Result{}, fmt.Errorf("not inside a Git repository: %w", err)
	}
	root := strings.TrimSpace(string(rootOutput))
	statusOutput, err := runGit(root, "diff", "--name-status", "--find-renames", rangeSpec, "--", "*.java")
	if err != nil {
		return Result{}, fmt.Errorf("failed to inspect Git diff %q: %w", rangeSpec, err)
	}
	base, err := baseRevision(root, rangeSpec)
	if err != nil {
		return Result{}, err
	}

	result := Result{Root: root, Current: map[string]model.ChangeKind{}}
	for _, line := range strings.Split(strings.TrimSpace(string(statusOutput)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		status := fields[0]
		switch status[0] {
		case 'A':
			result.Current[absolute(root, fields[1])] = model.Added
		case 'M', 'T':
			result.Current[absolute(root, fields[1])] = model.Modified
		case 'R', 'C':
			if len(fields) >= 3 {
				result.Current[absolute(root, fields[2])] = model.Modified
			}
		case 'D':
			content, showErr := runGit(root, "show", base+":"+filepath.ToSlash(fields[1]))
			if showErr != nil {
				return Result{}, fmt.Errorf("failed to load deleted Java file %s: %w", fields[1], showErr)
			}
			result.Deleted = append(result.Deleted, DeletedSource{
				Path: absolute(root, fields[1]), Content: content,
			})
		}
	}
	untracked, err := runGit(root, "ls-files", "--others", "--exclude-standard", "--", "*.java")
	if err != nil {
		return Result{}, fmt.Errorf("failed to inspect untracked Java files: %w", err)
	}
	for _, path := range strings.Split(strings.TrimSpace(string(untracked)), "\n") {
		if path != "" {
			result.Current[absolute(root, path)] = model.Added
		}
	}
	if len(result.Current) == 0 && len(result.Deleted) == 0 {
		return Result{}, fmt.Errorf("no changed Java files found in Git diff: %s", rangeSpec)
	}
	return result, nil
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

func absolute(root, path string) string {
	result, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		return filepath.Clean(filepath.Join(root, filepath.FromSlash(path)))
	}
	return filepath.Clean(result)
}

func runGit(dir string, args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	if dir != "" {
		command.Dir = dir
	}
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
