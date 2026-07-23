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
	schemaVersion     = "java-tree-sitter-v1"
	markerFile        = ".umlgen-cache"
	markerContent     = "umlgen cache\n"
)

type Cache struct {
	dir       string
	signature []byte
}

type Result struct {
	Types      []model.Type
	Hit        bool
	CacheError error
}

type entry struct {
	Types []model.Type `json:"types"`
}

func Open(version string, settings any) (*Cache, error) {
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("encode cache settings: %w", err)
	}
	root, err := RootDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, "java")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create Java cache: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure Java cache: %w", err)
	}
	markerPath := filepath.Join(root, markerFile)
	if err := os.WriteFile(markerPath, []byte(markerContent), 0o600); err != nil {
		return nil, fmt.Errorf("mark umlgen cache: %w", err)
	}
	if err := os.Chmod(markerPath, 0o600); err != nil {
		return nil, fmt.Errorf("secure umlgen cache marker: %w", err)
	}
	signature := append([]byte(schemaVersion+"\x00"+version+"\x00"), settingsJSON...)
	return &Cache{dir: dir, signature: signature}, nil
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
	cachePath := filepath.Join(c.dir, c.key(source)+".json")
	cached, cacheErr := c.load(cachePath, path)
	if cacheErr == nil && cached != nil {
		return Result{Types: cached, Hit: true}, nil
	}

	types, parseErr := java.ParseSource(path, source)
	if parseErr != nil {
		return Result{}, parseErr
	}
	storeErr := c.store(cachePath, types)
	return Result{
		Types:      types,
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

func (c *Cache) load(path, sourcePath string) ([]model.Type, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read cache entry: %w", err)
	}
	var cached entry
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, fmt.Errorf("discard invalid cache entry: %w", err)
	}
	for i := range cached.Types {
		cached.Types[i].Source = sourcePath
	}
	return cached.Types, nil
}

func (c *Cache) store(path string, types []model.Type) error {
	cachedTypes := append([]model.Type(nil), types...)
	for i := range cachedTypes {
		cachedTypes[i].Source = ""
	}
	payload, err := json.Marshal(entry{Types: cachedTypes})
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
