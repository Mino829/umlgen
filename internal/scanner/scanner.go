package scanner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var defaultExcluded = map[string]bool{
	".git": true, ".idea": true, ".vscode": true, "build": true,
	"target": true, "out": true, "node_modules": true, "vendor": true,
}

func JavaFiles(targets, excludes []string) ([]string, error) {
	return SourceFiles(targets, excludes, ".java")
}

func SourceFiles(targets, excludes []string, extensions ...string) ([]string, error) {
	supported := make(map[string]bool, len(extensions))
	for _, extension := range extensions {
		supported[strings.ToLower(extension)] = true
	}
	var files []string
	seen := map[string]bool{}
	for _, target := range targets {
		info, err := os.Stat(target)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("target does not exist: %s", target)
			}
			return nil, err
		}
		if !info.IsDir() {
			if supported[strings.ToLower(filepath.Ext(target))] {
				abs, err := filepath.Abs(target)
				if err != nil {
					return nil, fmt.Errorf("resolve source path %q: %w", target, err)
				}
				if !seen[abs] {
					files, seen[abs] = append(files, abs), true
				}
				continue
			}
			return nil, fmt.Errorf("target is not a supported source file or directory: %s", target)
		}
		err = filepath.WalkDir(target, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if path != target && ExcludedDirectory(path, excludes) {
					return filepath.SkipDir
				}
				return nil
			}
			if supported[strings.ToLower(filepath.Ext(entry.Name()))] && !ExcludedPath(path, excludes) {
				abs, err := filepath.Abs(path)
				if err != nil {
					return fmt.Errorf("resolve source path %q: %w", path, err)
				}
				if !seen[abs] {
					files, seen[abs] = append(files, abs), true
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

// ExcludedDirectory shares traversal exclusions with immutable Git scanning.
// Callers preserve the explicit-target override before using this predicate.
func ExcludedDirectory(path string, excludes []string) bool {
	return defaultExcluded[filepath.Base(path)] || ExcludedPath(path, excludes)
}

// ExcludedPath matches path exclusions; package-like dotted selectors are
// intentionally left for the graph's package selection stage.
func ExcludedPath(path string, excludes []string) bool {
	slash := filepath.ToSlash(path)
	for _, ex := range excludes {
		ex = strings.Trim(strings.TrimSpace(filepath.ToSlash(ex)), "/")
		if ex == "" || strings.Contains(ex, ".") && !strings.Contains(ex, "/") {
			continue
		}
		if slash == ex || strings.Contains(slash, "/"+ex+"/") || strings.HasSuffix(slash, "/"+ex) {
			return true
		}
	}
	return false
}
