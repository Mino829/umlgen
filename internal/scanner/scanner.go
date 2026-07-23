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
	"target": true, "out": true, "node_modules": true,
}

func JavaFiles(targets, excludes []string) ([]string, error) {
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
			if strings.EqualFold(filepath.Ext(target), ".java") {
				abs, _ := filepath.Abs(target)
				if !seen[abs] {
					files, seen[abs] = append(files, abs), true
				}
				continue
			}
			return nil, fmt.Errorf("target is not a Java file or directory: %s", target)
		}
		err = filepath.WalkDir(target, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if path != target && (defaultExcluded[entry.Name()] || excludedPath(path, excludes)) {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.EqualFold(filepath.Ext(entry.Name()), ".java") && !excludedPath(path, excludes) {
				abs, _ := filepath.Abs(path)
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

func excludedPath(path string, excludes []string) bool {
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
