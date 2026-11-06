package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func validateStateDiagram(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode {
		return validateStateDiagram(node.Alias)
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("diagrams.state must be a mapping")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		want := "!!str"
		switch key.Value {
		case "state_config", "type", "field", "analysis_timeout":
		case "max_nodes", "java_release":
			want = "!!int"
		default:
			return fmt.Errorf("unknown diagrams.state key %q", key.Value)
		}
		if key.Tag != "!!str" {
			return fmt.Errorf("unknown diagrams.state key %q", key.Value)
		}
		for value.Kind == yaml.AliasNode {
			value = value.Alias
		}
		if value.Kind != yaml.ScalarNode || value.Tag != want {
			return fmt.Errorf("diagrams.state.%s must have scalar type %s", key.Value, want)
		}
	}
	return nil
}
