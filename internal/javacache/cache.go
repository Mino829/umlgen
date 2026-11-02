package javacache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Mino829/umlgen/internal/java"
	"github.com/Mino829/umlgen/internal/model"
)

const (
	cacheDirectoryEnv = "UMLGEN_CACHE_DIR"
	markerFile        = ".umlgen-cache"
	markerContent     = "umlgen cache\n"
)

type Cache struct {
	dir       string
	signature []byte
	parse     func(string, []byte) (*model.SourceUnit, error)
}

type Result struct {
	Unit       *model.SourceUnit
	Hit        bool
	CacheError error
}

func Open(version string, settings any) (*Cache, error) {
	return OpenFor(version, "java", java.SchemaVersion, settings, java.ParseSource)
}

// OpenFor caches content-dependent syntax only. The caller must apply filesystem
// context after ParseFile or ParseSource returns, never inside parse.
func OpenFor(
	version, language, parserSchema string,
	settings any,
	parse func(string, []byte) (*model.SourceUnit, error),
) (*Cache, error) {
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("encode cache settings: %w", err)
	}
	root, err := RootDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, language)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create %s cache: %w", language, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure %s cache: %w", language, err)
	}
	markerPath := filepath.Join(root, markerFile)
	if err := os.WriteFile(markerPath, []byte(markerContent), 0o600); err != nil {
		return nil, fmt.Errorf("mark umlgen cache: %w", err)
	}
	if err := os.Chmod(markerPath, 0o600); err != nil {
		return nil, fmt.Errorf("secure umlgen cache marker: %w", err)
	}
	signature := append([]byte(parserSchema+"\x00"+version+"\x00"), settingsJSON...)
	return &Cache{dir: dir, signature: signature, parse: parse}, nil
}

func RootDir() (string, error) {
	if configured := os.Getenv(cacheDirectoryEnv); configured != "" {
		return filepath.Abs(configured)
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find user cache directory: %w", err)
	}
	return filepath.Join(base, "umlgen"), nil
}

func Clean() (string, error) {
	root, err := RootDir()
	if err != nil {
		return "", err
	}
	if root == "" || filepath.Clean(root) == string(filepath.Separator) {
		return "", errors.New("refusing to remove an unsafe cache path")
	}
	marker, err := os.ReadFile(filepath.Join(root, markerFile))
	if os.IsNotExist(err) {
		if _, statErr := os.Stat(root); os.IsNotExist(statErr) {
			return root, nil
		}
		return "", fmt.Errorf("refusing to remove unrecognized cache directory: %s", root)
	}
	if err != nil || string(marker) != markerContent {
		return "", fmt.Errorf("refusing to remove unrecognized cache directory: %s", root)
	}
	if err := os.RemoveAll(root); err != nil {
		return "", fmt.Errorf("remove cache %s: %w", root, err)
	}
	return root, nil
}

func (c *Cache) ParseFile(path string) (Result, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	return c.ParseSource(path, source)
}

// ParseSource uses the caller's captured bytes for both parsing and caching.
func (c *Cache) ParseSource(path string, source []byte) (Result, error) {
	cachePath := filepath.Join(c.dir, c.key(source)+".json")
	cached, cacheErr := c.load(cachePath, path)
	if cacheErr == nil && cached != nil {
		return Result{Unit: cached, Hit: true}, nil
	}

	unit, parseErr := c.parse(path, source)
	if parseErr != nil {
		return Result{Unit: unit}, parseErr
	}
	storeErr := c.store(cachePath, unit)
	return Result{
		Unit:       unit,
		CacheError: errors.Join(cacheErr, storeErr),
	}, nil
}

func (c *Cache) key(source []byte) string {
	hash := sha256.New()
	hash.Write(c.signature)
	hash.Write([]byte{0})
	hash.Write(source)
	return hex.EncodeToString(hash.Sum(nil))
}

func (c *Cache) load(path, sourcePath string) (*model.SourceUnit, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read cache entry: %w", err)
	}
	var cached model.SourceUnit
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, fmt.Errorf("discard invalid cache entry: %w", err)
	}
	if cached.Language == "" || cached.ContentHash == "" {
		return nil, fmt.Errorf("discard invalid cache entry: missing source unit metadata")
	}
	cached.RebindPath(sourcePath)
	return &cached, nil
}

func (c *Cache) store(path string, unit *model.SourceUnit) error {
	// Paths and package context are transient. Imports are stored once per file,
	// including files without types; load restores every declaration's scope.
	payload, err := json.Marshal(unit)
	if err != nil {
		return fmt.Errorf("encode cache entry: %w", err)
	}
	temp, err := os.CreateTemp(c.dir, ".entry-")
	if err != nil {
		return fmt.Errorf("create cache entry: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("secure cache entry: %w", err)
	}
	if _, err := temp.Write(payload); err != nil {
		temp.Close()
		return fmt.Errorf("write cache entry: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("write cache entry: %w", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace cache entry: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("publish cache entry: %w", err)
	}
	return nil
}
