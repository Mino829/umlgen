package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func validateActivityDiagram(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode {
		return validateActivityDiagram(node.Alias)
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("diagrams.activity must be a mapping")
	}
	for n := 0; n < len(node.Content); n += 2 {
		key, value := node.Content[n], node.Content[n+1]
		want := "!!str"
		switch key.Value {
		case "function", "analysis_timeout":
		case "max_nodes", "java_release":
			want = "!!int"
		default:
			return fmt.Errorf("unknown diagrams.activity key %q", key.Value)
		}
		if key.Tag != "!!str" {
			return fmt.Errorf("unknown diagrams.activity key %q", key.Value)
		}
		for value.Kind == yaml.AliasNode {
			value = value.Alias
		}
		if value.Kind != yaml.ScalarNode || value.Tag != want {
			return fmt.Errorf("diagrams.activity.%s must have scalar type %s", key.Value, want)
		}
	}
	return nil
}
