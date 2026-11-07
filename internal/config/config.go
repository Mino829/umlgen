package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mino829/umlgen/internal/activity"
	"github.com/Mino829/umlgen/internal/sequence"

	"gopkg.in/yaml.v3"
)

const DefaultFile = ".umlgen.yaml"

type DiagramsConfig struct {
	Package    PackageDiagramConfig    `yaml:"package"`
	Sequence   SequenceDiagramConfig   `yaml:"sequence"`
	Activity   ActivityDiagramConfig   `yaml:"activity"`
	State      StateDiagramConfig      `yaml:"state"`
	Component  ComponentDiagramConfig  `yaml:"component"`
	Deployment DeploymentDiagramConfig `yaml:"deployment"`
}

type PackageDiagramConfig struct {
	DependencySource string `yaml:"dependency_source"`
	External         string `yaml:"external"`
	Cycles           string `yaml:"cycles"`
}

type SequenceDiagramConfig struct {
	Entry           string        `yaml:"entry"`
	MaxCallDepth    int           `yaml:"max_call_depth"`
	MaxParticipants int           `yaml:"max_participants"`
	MaxMessages     int           `yaml:"max_messages"`
	JavaRelease     int           `yaml:"java_release"`
	AnalysisTimeout time.Duration `yaml:"analysis_timeout"`
}

type ActivityDiagramConfig struct {
	Function        string        `yaml:"function"`
	MaxNodes        int           `yaml:"max_nodes"`
	JavaRelease     int           `yaml:"java_release"`
	AnalysisTimeout time.Duration `yaml:"analysis_timeout"`
}

type StateDiagramConfig struct {
	StateConfig     string        `yaml:"state_config"`
	Type            string        `yaml:"type"`
	Field           string        `yaml:"field"`
	MaxNodes        int           `yaml:"max_nodes"`
	JavaRelease     int           `yaml:"java_release"`
	AnalysisTimeout time.Duration `yaml:"analysis_timeout"`
}

type ComponentDiagramConfig struct {
	BoundaryConfig   string `yaml:"boundary_config"`
	DependencySource string `yaml:"dependency_source"`
	External         string `yaml:"external"`
}

type Config struct {
	Language string   `yaml:"language"`
	Source   []string `yaml:"source"`
	Exclude  []string `yaml:"exclude"`
	Output   struct {
		File    string `yaml:"file"`
		FileSet bool   `yaml:"-"`
		Format  string `yaml:"format"`
	} `yaml:"output"`
	Visibility struct {
		Public         bool `yaml:"public"`
		Protected      bool `yaml:"protected"`
		Private        bool `yaml:"private"`
		PackagePrivate bool `yaml:"package_private"`
	} `yaml:"visibility"`
	Members struct {
		Fields  bool `yaml:"fields"`
		Methods bool `yaml:"methods"`
	} `yaml:"members"`
	Relations struct {
		Inheritance         bool `yaml:"inheritance"`
		Implementation      bool `yaml:"implementation"`
		FieldDependency     bool `yaml:"field_dependency"`
		ParameterDependency bool `yaml:"parameter_dependency"`
		ReturnDependency    bool `yaml:"return_dependency"`
	} `yaml:"relations"`
	Layout struct {
		PackageGrouping string            `yaml:"package_grouping"`
		Direction       string            `yaml:"direction"`
		SkinParams      map[string]string `yaml:"skinparams"`
	} `yaml:"layout"`
	Renderer struct {
		Type      string        `yaml:"type"`
		ServerURL string        `yaml:"server_url"`
		Timeout   time.Duration `yaml:"timeout"`
	} `yaml:"renderer"`
	Diagrams DiagramsConfig `yaml:"diagrams"`
}

func Defaults() Config {
	var c Config
	limits := sequence.DefaultLimits()
	c.Language = "auto"
	c.Output.File = "class-diagram.puml"
	c.Output.Format = "plantuml"
	c.Diagrams.Package.DependencySource = "declared-types"
	c.Diagrams.Package.External = "hide"
	c.Diagrams.Package.Cycles = "mark"
	c.Diagrams.Sequence = SequenceDiagramConfig{
		MaxCallDepth: limits.MaxCallDepth, MaxParticipants: limits.MaxParticipants, MaxMessages: limits.MaxMessages,
		JavaRelease: 17, AnalysisTimeout: 30 * time.Second,
	}
	c.Diagrams.Activity = ActivityDiagramConfig{
		MaxNodes: activity.DefaultMaxNodes, JavaRelease: 17, AnalysisTimeout: 30 * time.Second,
	}
	c.Diagrams.State = StateDiagramConfig{
		MaxNodes: activity.DefaultMaxNodes, JavaRelease: 17, AnalysisTimeout: 30 * time.Second,
	}
	c.Diagrams.Component = ComponentDiagramConfig{DependencySource: "both", External: "hide"}
	c.Diagrams.Deployment.ManifestFormat = "auto"
	c.Visibility.Public = true
	c.Visibility.Protected = true
	c.Visibility.Private = true
	c.Visibility.PackagePrivate = true
	c.Members.Fields = true
	c.Members.Methods = true
	c.Relations.Inheritance = true
	c.Relations.Implementation = true
	c.Relations.FieldDependency = true
	c.Relations.ParameterDependency = true
	c.Relations.ReturnDependency = true
	c.Layout.PackageGrouping = "flat"
	c.Layout.Direction = "top-to-bottom"
	c.Renderer.Type = "auto"
	c.Renderer.Timeout = 30 * time.Second
	return c
}

func Load(path string, explicit bool) (Config, string, error) {
	c := Defaults()
	if path == "" {
		path = DefaultFile
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			return c, "", nil
		}
		return c, "", fmt.Errorf("cannot read configuration %s: %w", path, err)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return c, path, fmt.Errorf("invalid configuration in %s: malformed YAML (values suppressed)", path)
	}
	var settings struct {
		Diagrams yaml.Node `yaml:"diagrams"`
		Output   struct {
			File yaml.Node `yaml:"file"`
		} `yaml:"output"`
	}
	if err := document.Decode(&settings); err != nil {
		return c, path, fmt.Errorf("invalid configuration in %s: %w", path, err)
	}
	if settings.Diagrams.Kind != 0 {
		if err := validateDiagrams(&settings.Diagrams); err != nil {
			return c, path, fmt.Errorf("invalid configuration in %s: %w", path, err)
		}
	}
	if err := document.Decode(&c); err != nil {
		return c, path, fmt.Errorf("invalid configuration in %s: %w", path, err)
	}
	file := &settings.Output.File
	for file.Kind == yaml.AliasNode {
		file = file.Alias
	}
	c.Output.FileSet = file.Kind != 0 && file.Tag != "!!null"

	c.Language = strings.ToLower(strings.TrimSpace(c.Language))
	c.Renderer.Type = strings.ToLower(strings.TrimSpace(c.Renderer.Type))
	c.Diagrams.Package.DependencySource = strings.ToLower(strings.TrimSpace(c.Diagrams.Package.DependencySource))
	c.Diagrams.Package.External = strings.ToLower(strings.TrimSpace(c.Diagrams.Package.External))
	c.Diagrams.Package.Cycles = strings.ToLower(strings.TrimSpace(c.Diagrams.Package.Cycles))
	if c.Language != "auto" && c.Language != "java" && c.Language != "go" {
		return c, path, fmt.Errorf("invalid configuration in %s: unsupported language %q", path, c.Language)
	}
	return c, path, nil
}

func validateDiagrams(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode {
		return validateDiagrams(node.Alias)
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("diagrams must be a mapping")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Tag != "!!str" {
			return fmt.Errorf("unknown diagrams key %q", key.Value)
		}
		switch key.Value {
		case "package":
			if err := validatePackageDiagram(value); err != nil {
				return err
			}
		case "sequence":
			if err := validateSequenceDiagram(value); err != nil {
				return err
			}
		case "activity":
			if err := validateActivityDiagram(value); err != nil {
				return err
			}
		case "state":
			if err := validateStateDiagram(value); err != nil {
				return err
			}
		case "component":
			if err := validateComponentDiagram(value); err != nil {
				return err
			}
		case "deployment":
			if err := validateDeploymentDiagram(value); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown diagrams key %q", key.Value)
		}
	}
	return nil
}

func validatePackageDiagram(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode {
		return validatePackageDiagram(node.Alias)
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("diagrams.package must be a mapping")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		switch key.Value {
		case "dependency_source", "external", "cycles":
			if key.Tag != "!!str" {
				return fmt.Errorf("unknown diagrams.package key %q", key.Value)
			}
		default:
			return fmt.Errorf("unknown diagrams.package key %q", key.Value)
		}
		for value.Kind == yaml.AliasNode {
			value = value.Alias
		}
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
			return fmt.Errorf("diagrams.package.%s must be a string", key.Value)
		}
	}
	return nil
}

func ResolvePaths(c *Config, configPath string) {
	if configPath == "" {
		return
	}
	base := filepath.Dir(configPath)
	resolveDeploymentPaths(&c.Diagrams.Deployment, base)
	for i, source := range c.Source {
		if !filepath.IsAbs(source) {
			c.Source[i] = filepath.Join(base, source)
		}
	}
	if c.Output.File != "" && !filepath.IsAbs(c.Output.File) {
		c.Output.File = filepath.Join(base, c.Output.File)
	}
	if path := c.Diagrams.State.StateConfig; path != "" && !filepath.IsAbs(path) {
		c.Diagrams.State.StateConfig = filepath.Join(base, path)
	}
	if path := c.Diagrams.Component.BoundaryConfig; path != "" && !filepath.IsAbs(path) {
		c.Diagrams.Component.BoundaryConfig = filepath.Join(base, path)
	}
}

const Template = `language: auto

source:
  - .

exclude:
  - src/test
  - target
  - build
  - generated

output:
  # Omit file for command-specific class/package/package-change/sequence/activity/state/component/deployment defaults.
  # file: diagrams/model.puml
  format: plantuml

visibility:
  public: true
  protected: true
  private: true
  package_private: true

members:
  fields: true
  methods: true

relations:
  inheritance: true
  implementation: true
  field_dependency: true
  parameter_dependency: true
  return_dependency: true

layout:
  package_grouping: flat
  direction: top-to-bottom
  skinparams: {}

renderer:
  type: auto
  server_url: ""
  timeout: 30s

diagrams:
  package:
    dependency_source: declared-types
    external: hide
    cycles: mark
  sequence:
    entry: ""
    max_call_depth: 3
    max_participants: 30
    max_messages: 200
    java_release: 17
    analysis_timeout: 30s
  activity:
    function: ""
    max_nodes: 500
    java_release: 17
    analysis_timeout: 30s
  state:
    state_config: ""
    type: ""
    field: ""
    max_nodes: 500
    java_release: 17
    analysis_timeout: 30s
  component:
    boundary_config: ""
    dependency_source: both
    external: hide
  deployment:
    manifest: ""
    manifest_format: auto
    boundary_config: ""
    mapping_config: ""
`
