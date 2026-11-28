package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Mino829/umlgen/internal/fsidentity"
	"github.com/Mino829/umlgen/internal/gitdiff"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/scanner"
)

type classDiffTarget struct {
	path, physical string
	file, missing  bool
	deleted        bool
}

// Match Git paths against the physical target, but apply traversal exclusions
// to the spelling the live scanner would have seen from that target.
func (t classDiffTarget) relative(path string) (string, bool) {
	rel, err := filepath.Rel(t.physical, path)
	return rel, err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && (!t.file || rel == ".")
}

func (t classDiffTarget) includes(relative string, excludes []string) bool {
	if relative == "." {
		return true // Explicit source targets bypass traversal exclusions.
	}
	if scanner.ExcludedPath(filepath.Join(t.path, relative), excludes) {
		return false
	}
	for dir := filepath.Dir(relative); dir != "."; dir = filepath.Dir(dir) {
		if scanner.ExcludedDirectory(filepath.Join(t.path, dir), excludes) {
			return false
		}
	}
	return true
}

func scopeClassDiff(selection *gitdiff.Result, targets, excludes []string) (live, changed, deleted []string, err error) {
	root, err := fsidentity.Resolve(selection.Root)
	if err != nil {
		return nil, nil, nil, err
	}
	scopes := make([]classDiffTarget, 0, len(targets))
	for _, path := range targets {
		info, statErr := os.Stat(path)
		missing := os.IsNotExist(statErr)
		if statErr != nil && !missing {
			return nil, nil, nil, statErr
		}
		physical, err := fsidentity.Resolve(path)
		if err != nil {
			return nil, nil, nil, err
		}
		scopes = append(scopes, classDiffTarget{path: path, physical: physical, file: !missing && !info.IsDir(), missing: missing})
		if !missing {
			live = append(live, path)
		}
	}
	// A base path also establishes that a now-missing explicit target existed.
	includesBase := func(path string) bool {
		included := false
		for i := range scopes {
			target := &scopes[i]
			if rel, ok := target.relative(path); ok {
				target.deleted = true
				included = included || target.includes(rel, excludes)
			}
		}
		return included
	}
	for _, name := range selection.Deleted {
		path := filepath.Join(root, filepath.FromSlash(name))
		if includesBase(path) {
			deleted = append(deleted, name)
			changed = append(changed, path)
		}
	}
	deletionCount := len(deleted)
	for name := range selection.Current {
		path := filepath.Join(root, filepath.FromSlash(name))
		included := false
		for _, target := range scopes {
			if rel, ok := target.relative(path); !target.missing && ok && target.includes(rel, excludes) {
				included = true
				changed = append(changed, path)
				break
			}
		}
		if from, ok := selection.RenamedFrom[name]; ok {
			basePath := filepath.Join(root, filepath.FromSlash(from))
			wasIncluded := includesBase(basePath)
			switch {
			case wasIncluded && !included:
				deleted = append(deleted, from)
				changed = append(changed, basePath)
			case !wasIncluded && included:
				selection.Current[name] = model.Added
			}
		}
	}
	if len(deleted) != deletionCount {
		slices.Sort(deleted)
	}
	for _, target := range scopes {
		if target.missing && !target.deleted {
			return nil, nil, nil, fmt.Errorf("target does not exist: %s", target.path)
		}
	}
	return live, changed, deleted, nil
}
