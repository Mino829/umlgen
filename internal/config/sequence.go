package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func validateSequenceDiagram(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode {
		return validateSequenceDiagram(node.Alias)
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("diagrams.sequence must be a mapping")
	}
	for n := 0; n < len(node.Content); n += 2 {
		key, value := node.Content[n], node.Content[n+1]
		if key.Tag != "!!str" {
			return fmt.Errorf("unknown diagrams.sequence key %q", key.Value)
		}
		want := "!!int"
		switch key.Value {
		case "entry", "analysis_timeout":
			want = "!!str"
		case "max_call_depth", "max_participants", "max_messages", "java_release":
		default:
			return fmt.Errorf("unknown diagrams.sequence key %q", key.Value)
		}
		for value.Kind == yaml.AliasNode {
			value = value.Alias
		}
		if value.Kind != yaml.ScalarNode || value.Tag != want {
			return fmt.Errorf("diagrams.sequence.%s must have scalar type %s", key.Value, want)
		}
	}
	return nil
}
