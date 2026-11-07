// Package fsidentity resolves physical filesystem paths without discarding symlink traversal.
package fsidentity

import (
	"os"
	"path/filepath"
)

// Resolve returns an absolute path with existing symlinks resolved. If a suffix
// does not exist, its existing prefix is resolved before that suffix is cleaned.
// Path casing is preserved; callers needing existing-file identity should also
// use os.SameFile.
func Resolve(path string) (string, error) {
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		// Abs and Join would clean link/.. before resolving the link.
		path = cwd + string(os.PathSeparator) + path
	}
	return resolve(path)
}

func resolve(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	// Both '/' and '\\' are separators on Windows, including trailing ones.
	for len(path) > 0 && os.IsPathSeparator(path[len(path)-1]) {
		path = path[:len(path)-1]
	}
	parent, base := filepath.Split(path)
	if parent == "" || base == "" {
		return "", err
	}
	resolved, err = resolve(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, base), nil
}
