# umlgen

`umlgen` is a local-first CLI that analyzes Java declarations and generates an
editable PlantUML class diagram. Version 0.1.0 is the MVP described in the
project requirements.

## Features

- Recursively discovers Java files
- Extracts packages, classes, interfaces, fields, methods, constructors,
  inheritance, implementation, and type dependencies
- Filters by package or excluded path/package
- Controls private members, fields, methods, and relationships
- Reads `.umlgen.yaml`
- Always generates `.puml`; optionally invokes a local PlantUML installation
  to generate SVG
- Continues when an individual Java file cannot be parsed

Source code is processed locally and is never uploaded.

## Requirements

- Go 1.24 or later (build only)
- PlantUML executable or `PLANTUML_JAR` plus Java (SVG output only)

## Build and test

```bash
make build
make test
```

The resulting executable is `./umlgen`. To install it on your `PATH`:

```bash
go install ./cmd/umlgen
```

## Usage

```bash
umlgen class ./src/main/java
umlgen class ./src/main/java -o docs/domain.puml
umlgen class ./src --include com.example.user
umlgen class ./src --exclude test --exclude generated
umlgen class ./src --hide-private --hide-methods
umlgen class ./src --format svg
```

Options may appear before or after the target. Run `umlgen class --help` for
the complete list.

Generate a starter configuration:

```bash
umlgen init
umlgen class
```

When `--format svg` is used, the `.puml` file is kept. If PlantUML is not
installed, generation ends with exit code 5 after preserving that file.

## Configuration

`.umlgen.yaml`:

```yaml
language: java
source:
  - src/main/java
exclude:
  - src/test
  - target
output:
  file: docs/class-diagram.puml
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
```

Precedence is command-line options, an explicitly selected configuration,
`.umlgen.yaml`, then built-in defaults. Relative `source` and output paths are
resolved from the configuration file's directory.

## Example

```bash
./umlgen class ./examples/java --output ./examples/class-diagram.puml
```

## Exit codes

| Code | Meaning |
| ---: | --- |
| 0 | Success |
| 1 | General/target error |
| 2 | CLI argument or configuration error |
| 3 | All source files failed to parse |
| 4 | Output error |
| 5 | SVG rendering error (`.puml` remains available) |

## MVP parser scope

The parser tokenizes Java source and builds a declaration-level intermediate
representation. It handles multiline declarations, comments, annotations,
generics, arrays, and method bodies without using regular expressions as a
source parser. It does not perform Java symbol resolution or inspect method
bodies, so reflection, Lombok-generated members, anonymous classes, and
framework-specific dependency inference are intentionally outside the MVP.
