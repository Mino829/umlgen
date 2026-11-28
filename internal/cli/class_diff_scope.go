package cli

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
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

// scopeClassDiff limits a Git class diff to the selected targets. A rename is
// judged at both ends: leaving the selection shows the base declaration as
// removed, entering it shows the destination as added.
func scopeClassDiff(selection gitdiff.Result, targets, excludes []string) (scoped gitdiff.Result, live, changed []string, err error) {
	root, err := fsidentity.Resolve(selection.Root)
	if err != nil {
		return gitdiff.Result{}, nil, nil, err
	}
	scopes := make([]classDiffTarget, 0, len(targets))
	for _, path := range targets {
		info, statErr := os.Stat(path)
		missing := os.IsNotExist(statErr)
		if statErr != nil && !missing {
			return gitdiff.Result{}, nil, nil, statErr
		}
		physical, err := fsidentity.Resolve(path)
		if err != nil {
			return gitdiff.Result{}, nil, nil, err
		}
		scopes = append(scopes, classDiffTarget{path: path, physical: physical, file: !missing && !info.IsDir(), missing: missing})
		if !missing {
			live = append(live, path)
		}
	}
	// Base paths may belong to targets that no longer exist in the working tree.
	baseScope := func(name string) (string, bool) {
		path := filepath.Join(root, filepath.FromSlash(name))
		included := false
		for i := range scopes {
			target := &scopes[i]
			if rel, ok := target.relative(path); ok {
				target.deleted = true
				included = included || target.includes(rel, excludes)
			}
		}
		return path, included
	}
	currentScope := func(name string) (string, bool) {
		path := filepath.Join(root, filepath.FromSlash(name))
		for _, target := range scopes {
			if rel, ok := target.relative(path); !target.missing && ok && target.includes(rel, excludes) {
				return path, true
			}
		}
		return path, false
	}
	scoped = selection
	scoped.Deleted = nil
	scoped.Current = make(map[string]model.ChangeKind, len(selection.Current))
	maps.Copy(scoped.Current, selection.Current)
	for _, name := range selection.Deleted {
		if path, included := baseScope(name); included {
			scoped.Deleted = append(scoped.Deleted, name)
			changed = append(changed, path)
		}
	}
	for _, rename := range selection.Renamed {
		source, sourceIncluded := baseScope(rename.From)
		_, destinationIncluded := currentScope(rename.To)
		switch {
		case sourceIncluded && !destinationIncluded:
			scoped.Deleted = append(scoped.Deleted, rename.From)
			changed = append(changed, source)
		case destinationIncluded && !sourceIncluded:
			scoped.Current[rename.To] = model.Added
		}
	}
	for _, target := range scopes {
		if target.missing && !target.deleted {
			return gitdiff.Result{}, nil, nil, fmt.Errorf("target does not exist: %s", target.path)
		}
	}
	for name := range scoped.Current {
		if path, included := currentScope(name); included {
			changed = append(changed, path)
		}
	}
	return scoped, live, changed, nil
}
