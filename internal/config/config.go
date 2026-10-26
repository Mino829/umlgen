package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const DefaultFile = ".umlgen.yaml"

type Config struct {
	Language string   `yaml:"language"`
	Source   []string `yaml:"source"`
	Exclude  []string `yaml:"exclude"`
	Output   struct {
		File   string `yaml:"file"`
		Format string `yaml:"format"`
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
	Renderer struct {
		Type      string        `yaml:"type"`
		ServerURL string        `yaml:"server_url"`
		Timeout   time.Duration `yaml:"timeout"`
	} `yaml:"renderer"`
}

func Defaults() Config {
	var c Config
	c.Language = "auto"
	c.Output.File = "class-diagram.puml"
	c.Output.Format = "plantuml"
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

	if err := yaml.Unmarshal(data, &c); err != nil {
		return c, path, fmt.Errorf("invalid configuration in %s: %w", path, err)
	}

	c.Language = strings.ToLower(strings.TrimSpace(c.Language))
	c.Renderer.Type = strings.ToLower(strings.TrimSpace(c.Renderer.Type))
	if c.Language != "auto" && c.Language != "java" && c.Language != "go" {
		return c, path, fmt.Errorf("invalid configuration in %s: unsupported language %q", path, c.Language)
	}
	return c, path, nil
}

func ResolvePaths(c *Config, configPath string) {
	if configPath == "" {
		return
	}
	base := filepath.Dir(configPath)
	for i, source := range c.Source {
		if !filepath.IsAbs(source) {
			c.Source[i] = filepath.Join(base, source)
		}
	}
	if c.Output.File != "" && !filepath.IsAbs(c.Output.File) {
		c.Output.File = filepath.Join(base, c.Output.File)
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
  file: class-diagram.puml
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

renderer:
  type: auto
  server_url: ""
  timeout: 30s
`
