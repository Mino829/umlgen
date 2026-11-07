package deployment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// decode never returns a YAML parser message: those can quote secret values.
func decode(path, format string) ([]*yaml.Node, Origin, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, Origin{}, fmt.Errorf("%w: invalid input path", ErrInput)
	}
	content, err := os.ReadFile(absolute)
	if err != nil {
		return nil, Origin{}, fmt.Errorf("read deployment input %q: %w", absolute, err)
	}
	hash := sha256.Sum256(content)
	source := Origin{File: absolute, Hash: hex.EncodeToString(hash[:]), Format: format}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var docs []*yaml.Node
	for {
		var doc yaml.Node
		err := decoder.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, source, fmt.Errorf("%w: malformed YAML; input values suppressed", ErrInput)
		}
		if len(doc.Content) == 0 || doc.Content[0].Tag == "!!null" {
			docs = append(docs, nil)
			continue
		}
		if err = validateYAML(&doc); err != nil {
			return nil, source, err
		}
		docs = append(docs, doc.Content[0])
	}
	if len(docs) == 0 {
		return nil, source, fmt.Errorf("%w: empty manifest", ErrInput)
	}
	return docs, source, nil
}

func validateYAML(n *yaml.Node) error {
	if n.Kind == yaml.AliasNode {
		return invalid(n, "YAML aliases are unsupported")
	}
	switch n.Tag {
	case "", "!!map", "!!seq", "!!str", "!!int", "!!bool", "!!null", "!!float", "!!timestamp":
	default:
		return invalid(n, "custom YAML tags are unsupported")
	}
	if n.Kind == yaml.MappingNode {
		seen := make(map[string]bool, len(n.Content)/2)
		for i := 0; i < len(n.Content); i += 2 {
			key := n.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || seen[key.Value] {
				return invalid(key, "non-string or duplicate YAML key")
			}
			seen[key.Value] = true
		}
	}
	for _, child := range n.Content {
		if err := validateYAML(child); err != nil {
			return err
		}
	}
	return nil
}

func invalid(n *yaml.Node, reason string) error {
	line := 0
	if n != nil {
		line = n.Line
	}
	return fmt.Errorf("%w: %s at line %d (input values suppressed)", ErrInput, reason, line)
}
func mapping(n *yaml.Node) (map[string]*yaml.Node, error) {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil, invalid(n, "expected mapping")
	}
	m := make(map[string]*yaml.Node, len(n.Content)/2)
	for i := 0; i < len(n.Content); i += 2 {
		m[n.Content[i].Value] = n.Content[i+1]
	}
	return m, nil
}
func text(n *yaml.Node) (string, error) {
	if n == nil || n.Kind != yaml.ScalarNode || n.Tag != "!!str" {
		return "", invalid(n, "expected string")
	}
	return n.Value, nil
}
func integer(n *yaml.Node) (int, error) {
	if n == nil || n.Kind != yaml.ScalarNode || n.Tag != "!!int" {
		return 0, invalid(n, "expected nonnegative integer")
	}
	v, err := strconv.Atoi(n.Value)
	if err != nil || v < 0 {
		return 0, invalid(n, "expected nonnegative integer")
	}
	return v, nil
}
func at(source Origin, n *yaml.Node) Origin {
	if n != nil {
		source.Line = n.Line
	}
	return source
}
func stringMap(n *yaml.Node) (map[string]string, error) {
	m, err := mapping(n)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(m))
	for key, value := range m {
		s, err := text(value)
		if err != nil {
			return nil, err
		}
		result[key] = s
	}
	return result, nil
}
func keys(m map[string]*yaml.Node) []string {
	result := make([]string, 0, len(m))
	for key := range m {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
func diagnostic(g *Graph, code, message string, source Origin, incomplete bool) {
	g.Diagnostics = append(g.Diagnostics, Diagnostic{Code: code, Message: message, Origin: source})
	g.Incomplete = g.Incomplete || incomplete
}

// addImage persists only supported literal image references, never a raw invalid
// URL, substitution expression or credential-like value. Unknown images remain
// scoped to their owner and container instead of binding equal variable names.
func addImage(g *Graph, seen map[ID]bool, image string, owner ID, source Origin, container string) {
	unknown := image == "" || strings.ContainsAny(image, "${} \t\r\n") || strings.Contains(image, "://")
	if i := strings.IndexByte(image, '@'); i >= 0 {
		digest := image[i+1:]
		if len(digest) != len("sha256:")+64 || !strings.HasPrefix(digest, "sha256:") {
			unknown = true
		} else {
			for _, c := range digest[len("sha256:"):] {
				if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
					unknown = true
					break
				}
			}
		}
	}
	id := ID{Kind: "image", Name: image}
	name := image
	if unknown {
		identity := container
		if identity == "" {
			identity = "service-image"
		}
		id = ID{Kind: "unknown-artifact", Namespace: owner.Kind + "/" + owner.Namespace + "/" + owner.Name, Name: identity}
		name = "unknown image (not expanded/built)"
		diagnostic(g, "unknown-image", "Image is absent or not a supported literal reference; no expansion/build was performed", source, true)
	}
	if !seen[id] {
		seen[id] = true
		g.Nodes = append(g.Nodes, Node{ID: id, Name: name, Origin: source})
	}
	g.Edges = append(g.Edges, Edge{From: owner, To: id, Kind: "uses-image", Detail: container, Origin: source})
}
