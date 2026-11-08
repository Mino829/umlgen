package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"path/filepath"
)

// DeploymentDiagramConfig selects explicitly declared topology inputs.
type DeploymentDiagramConfig struct {
	Manifest       string `yaml:"manifest"`
	ManifestFormat string `yaml:"manifest_format"`
	BoundaryConfig string `yaml:"boundary_config"`
	MappingConfig  string `yaml:"mapping_config"`
}

func validateDeploymentDiagram(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode || node.Tag != "!!map" {
		return fmt.Errorf("diagrams.deployment must be a mapping")
	}
	seen := make(map[string]bool)
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || seen[key.Value] {
			return fmt.Errorf("invalid diagrams.deployment key")
		}
		seen[key.Value] = true
		switch key.Value {
		case "manifest", "manifest_format", "boundary_config", "mapping_config":
		default:
			return fmt.Errorf("unknown diagrams.deployment key")
		}
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
			return fmt.Errorf("diagrams.deployment.%s must be a string", key.Value)
		}
	}
	return nil
}

func resolveDeploymentPaths(c *DeploymentDiagramConfig, base string) {
	for _, path := range []*string{&c.Manifest, &c.BoundaryConfig, &c.MappingConfig} {
		if *path != "" && !filepath.IsAbs(*path) {
			*path = filepath.Join(base, *path)
		}
	}
}
