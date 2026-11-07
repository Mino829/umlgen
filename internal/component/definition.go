package component

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mino829/umlgen/internal/fsidentity"

	"gopkg.in/yaml.v3"
)

// LoadDefinition reads data only. It does not execute display contracts or inspect
// source packages; context ownership is decided by Project from captured metadata.
func LoadDefinition(path string) (Definition, error) {
	var zero Definition
	content, err := os.ReadFile(path)
	if err != nil {
		return zero, fmt.Errorf("%w: %w", ErrBoundary, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return zero, fmt.Errorf("%w: %v", ErrBoundary, err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return zero, fmt.Errorf("%w: expected exactly one YAML document", ErrBoundary)
	}
	if len(document.Content) != 1 {
		return zero, fmt.Errorf("%w: definition must be a mapping", ErrBoundary)
	}
	root := document.Content[0]
	if err := boundaryMapping(root, map[string]string{"version": "!!int", "components": "!!seq"}); err != nil {
		return zero, err
	}
	var d Definition
	for i := 0; i < len(root.Content); i += 2 {
		value := root.Content[i+1]
		switch root.Content[i].Value {
		case "version":
			if err := value.Decode(&d.Version); err != nil {
				return zero, fmt.Errorf("%w: invalid version: %v", ErrBoundary, err)
			}
		case "components":
			for _, entry := range value.Content {
				if err := boundaryMapping(entry, map[string]string{"name": "!!str", "match": "!!seq", "provides": "!!seq", "requires": "!!seq"}); err != nil {
					return zero, err
				}
				b := Boundary{Line: entry.Line}
				for j := 0; j < len(entry.Content); j += 2 {
					field, item := entry.Content[j].Value, entry.Content[j+1]
					switch field {
					case "name":
						b.Name = item.Value
					case "match":
						for _, row := range item.Content {
							if err := boundaryMapping(row, map[string]string{"language": "!!str", "package_prefix": "!!str", "source_root": "!!str", "go_module": "!!str"}); err != nil {
								return zero, err
							}
							s := Selector{Line: row.Line}
							for k := 0; k < len(row.Content); k += 2 {
								key, v := row.Content[k].Value, row.Content[k+1].Value
								if strings.TrimSpace(v) == "" {
									return zero, fmt.Errorf("%w: empty selector %s at line %d", ErrBoundary, key, row.Content[k].Line)
								}
								switch key {
								case "language":
									s.Language = v
								case "package_prefix":
									s.PackagePrefix = v
								case "source_root":
									s.SourceRoot = v
								case "go_module":
									s.GoModule = v
								}
							}
							b.Match = append(b.Match, s)
						}
					case "provides", "requires":
						var names []string
						for _, name := range item.Content {
							if name.Kind != yaml.ScalarNode || name.Tag != "!!str" {
								return zero, fmt.Errorf("%w: %s entries at line %d must be strings", ErrBoundary, field, name.Line)
							}
							names = append(names, name.Value)
						}
						if field == "provides" {
							b.Provides = names
						} else {
							b.Requires = names
						}
					}
				}
				d.Components = append(d.Components, b)
			}
		}
	}
	if err := d.Validate(); err != nil {
		return zero, err
	}
	d.Source, err = filepath.Abs(path)
	if err != nil {
		return zero, fmt.Errorf("%w: %v", ErrBoundary, err)
	}
	for i := range d.Components {
		for j := range d.Components[i].Match {
			s := &d.Components[i].Match[j]
			if s.SourceRoot == "" {
				continue
			}
			root := s.SourceRoot
			if !filepath.IsAbs(root) {
				// Join cleans link/.. before the filesystem can resolve it.
				root = filepath.Dir(d.Source) + string(os.PathSeparator) + root
			}
			s.SourceRoot, err = fsidentity.Resolve(root)
			if err != nil {
				return zero, fmt.Errorf("%w: source_root at line %d: %v", ErrBoundary, s.Line, err)
			}
		}
	}
	hash := sha256.Sum256(content)
	d.Hash = hex.EncodeToString(hash[:])
	return d, nil
}

func boundaryMapping(node *yaml.Node, fields map[string]string) error {
	if node.Kind != yaml.MappingNode || node.Tag != "!!map" {
		return fmt.Errorf("%w: expected mapping at line %d", ErrBoundary, node.Line)
	}
	seen := make(map[string]bool, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		want, exists := fields[key.Value]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || !exists {
			return fmt.Errorf("%w: unknown key %q at line %d", ErrBoundary, key.Value, key.Line)
		}
		if seen[key.Value] {
			return fmt.Errorf("%w: duplicate key %q at line %d", ErrBoundary, key.Value, key.Line)
		}
		seen[key.Value] = true
		kind := yaml.ScalarNode
		if want == "!!seq" {
			kind = yaml.SequenceNode
		}
		if value.Kind != kind || value.Tag != want {
			return fmt.Errorf("%w: %s at line %d must have type %s", ErrBoundary, key.Value, value.Line, want)
		}
	}
	return nil
}

func (d Definition) Validate() error {
	if d.Version != 1 {
		return fmt.Errorf("%w: definition requires version: 1", ErrBoundary)
	}
	if len(d.Components) == 0 {
		return fmt.Errorf("%w: definition requires components", ErrBoundary)
	}
	names := make(map[string]bool, len(d.Components))
	for _, b := range d.Components {
		if strings.TrimSpace(b.Name) == "" || names[b.Name] {
			return fmt.Errorf("%w: empty or duplicate component name %q", ErrBoundary, b.Name)
		}
		names[b.Name] = true
		if len(b.Match) == 0 {
			return fmt.Errorf("%w: component %q requires match rows", ErrBoundary, b.Name)
		}
		for _, s := range b.Match {
			if s.Language != "go" && s.Language != "java" {
				return fmt.Errorf("%w: unsupported selector language %q", ErrBoundary, s.Language)
			}
			if s.PackagePrefix == "" && s.SourceRoot == "" && s.GoModule == "" {
				return fmt.Errorf("%w: selector requires a constraint", ErrBoundary)
			}
			if s.GoModule != "" && s.Language != "go" {
				return fmt.Errorf("%w: go_module is Go-only", ErrBoundary)
			}
			// Filesystem roots are literal identities: brackets, parentheses
			// and Windows separators are path characters, not regex syntax.
			if strings.ContainsRune(s.SourceRoot, 0) {
				return fmt.Errorf("%w: source_root contains NUL", ErrBoundary)
			}
			if s.SourceRoot != "" && strings.TrimSpace(s.SourceRoot) == "" {
				return fmt.Errorf("%w: source_root must not be blank", ErrBoundary)
			}
			for _, value := range []string{s.PackagePrefix, s.GoModule} {
				if value != "" && (strings.TrimSpace(value) != value || strings.ContainsAny(value, "*?[]{}\\^$|()")) {
					return fmt.Errorf("%w: invalid selector constraint %q (literal constraints only)", ErrBoundary, value)
				}
			}
			if s.PackagePrefix != "" {
				separator := "/"
				if s.Language == "java" {
					separator = "."
				}
				for _, part := range strings.Split(s.PackagePrefix, separator) {
					if part == "" || part == "." || part == ".." || strings.ContainsAny(part, " \t\r\n") {
						return fmt.Errorf("%w: invalid package_prefix %q", ErrBoundary, s.PackagePrefix)
					}
				}
			}
			if s.GoModule != "" {
				for _, part := range strings.Split(s.GoModule, "/") {
					if part == "" || part == "." || part == ".." || strings.ContainsAny(part, " \t\r\n") {
						return fmt.Errorf("%w: invalid go_module %q", ErrBoundary, s.GoModule)
					}
				}
			}
		}
		for _, contracts := range [][]string{b.Provides, b.Requires} {
			seen := make(map[string]bool, len(contracts))
			for _, name := range contracts {
				if strings.TrimSpace(name) == "" || seen[name] {
					return fmt.Errorf("%w: empty or duplicate contract name %q in component %q", ErrBoundary, name, b.Name)
				}
				seen[name] = true
			}
		}
	}
	return nil
}
