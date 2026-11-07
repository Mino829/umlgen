package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func validateComponentDiagram(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode {
		return validateComponentDiagram(node.Alias)
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("diagrams.component must be a mapping")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		switch key.Value {
		case "boundary_config", "dependency_source", "external":
		default:
			return fmt.Errorf("unknown diagrams.component key %q", key.Value)
		}
		if key.Tag != "!!str" {
			return fmt.Errorf("unknown diagrams.component key %q", key.Value)
		}
		for value.Kind == yaml.AliasNode {
			value = value.Alias
		}
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
			return fmt.Errorf("diagrams.component.%s must be a string", key.Value)
		}
	}
	return nil
}
