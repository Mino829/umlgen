package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const DefaultFile = ".umlgen.yaml"

type Config struct {
	Language string
	Source   []string
	Exclude  []string
	Output   struct {
		File   string
		Format string
	}
	Visibility struct {
		Public         bool
		Protected      bool
		Private        bool
		PackagePrivate bool
	}
	Members struct {
		Fields  bool
		Methods bool
	}
	Relations struct {
		Inheritance         bool
		Implementation      bool
		FieldDependency     bool
		ParameterDependency bool
		ReturnDependency    bool
	}
}

func Defaults() Config {
	var c Config
	c.Language = "java"
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
	return c
}

func Load(path string, explicit bool) (Config, string, error) {
	c := Defaults()
	if path == "" {
		path = DefaultFile
	}
	data, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			return c, "", nil
		}
		return c, "", fmt.Errorf("cannot read configuration %s: %w", path, err)
	}
	defer data.Close()

	section := ""
	lineNo := 0
	scanner := bufio.NewScanner(data)
	for scanner.Scan() {
		lineNo++
		raw := strings.TrimSpace(stripComment(scanner.Text()))
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "- ") {
			value := unquote(strings.TrimSpace(strings.TrimPrefix(raw, "- ")))
			switch section {
			case "source":
				c.Source = append(c.Source, value)
			case "exclude":
				c.Exclude = append(c.Exclude, value)
			default:
				return c, path, fmt.Errorf("invalid configuration in %s at line %d: unexpected list item", path, lineNo)
			}
			continue
		}
		key, value, ok := strings.Cut(raw, ":")
		if !ok {
			return c, path, fmt.Errorf("invalid configuration in %s at line %d", path, lineNo)
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if value == "" {
			section = key
			continue
		}
		full := key
		if strings.HasPrefix(scanner.Text(), " ") || strings.HasPrefix(scanner.Text(), "\t") {
			full = section + "." + key
		} else {
			section = ""
		}
		if err := assign(&c, full, unquote(value)); err != nil {
			return c, path, fmt.Errorf("invalid configuration in %s at line %d: %w", path, lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return c, path, err
	}
	if c.Language != "java" {
		return c, path, fmt.Errorf("invalid configuration in %s: unsupported language %q", path, c.Language)
	}
	return c, path, nil
}

func assign(c *Config, key, value string) error {
	boolean := func() (bool, error) { return strconv.ParseBool(value) }
	switch key {
	case "language":
		c.Language = value
	case "output.file":
		c.Output.File = value
	case "output.format":
		c.Output.Format = value
	case "visibility.public":
		v, e := boolean()
		c.Visibility.Public = v
		return e
	case "visibility.protected":
		v, e := boolean()
		c.Visibility.Protected = v
		return e
	case "visibility.private":
		v, e := boolean()
		c.Visibility.Private = v
		return e
	case "visibility.package_private":
		v, e := boolean()
		c.Visibility.PackagePrivate = v
		return e
	case "members.fields":
		v, e := boolean()
		c.Members.Fields = v
		return e
	case "members.methods":
		v, e := boolean()
		c.Members.Methods = v
		return e
	case "relations.inheritance":
		v, e := boolean()
		c.Relations.Inheritance = v
		return e
	case "relations.implementation":
		v, e := boolean()
		c.Relations.Implementation = v
		return e
	case "relations.field_dependency":
		v, e := boolean()
		c.Relations.FieldDependency = v
		return e
	case "relations.parameter_dependency":
		v, e := boolean()
		c.Relations.ParameterDependency = v
		return e
	case "relations.return_dependency":
		v, e := boolean()
		c.Relations.ReturnDependency = v
		return e
	default:
		// Forward-compatible: unknown keys are ignored in the MVP.
	}
	return nil
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

func stripComment(s string) string {
	quote := rune(0)
	for i, r := range s {
		if r == '\'' || r == '"' {
			if quote == 0 {
				quote = r
			} else if quote == r {
				quote = 0
			}
		}
		if r == '#' && quote == 0 {
			return s[:i]
		}
	}
	return s
}

func unquote(s string) string {
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}

const Template = `language: java

source:
  - src/main/java

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
`
