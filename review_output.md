# umlgen Code Review

**Project:** `github.com/Mino829/umlgen`  
**Scope:** All Go source files under `cmd/`, `internal/`, and tests.  
**Date:** 2026-10-14  
**Verification:** `go vet ./...` clean, `go test ./...` passes.

---

## 1. Summary

`umlgen` is a well-structured CLI that parses Java (Tree-sitter) and Go (`go/ast`) source into a language-neutral model and generates PlantUML class diagrams. The codebase is cleanly layered (`cli` → `scanner`/`language` → `parser` → `relations` → `plantuml`/`renderer`) and has solid integration test coverage, including golden fixtures and Git-diff workflows. `go vet` and the full test suite pass.

Most issues are localized correctness or maintainability problems rather than architectural flaws. The highest-impact bugs are in Go implementation inference (pointer/value receiver semantics), the hand-rolled YAML parser, and package/path exclusion logic.

---

## 2. Strengths

- **Clear architecture.** Separate packages for parsing, relations, generation, rendering, caching, and diffing; the `model` package provides a stable intermediate representation.
- **Good test coverage.** Golden-diagram tests (`TestCompatibilityFixtureGoldenDiagram`), end-to-end CLI tests, cache invalidation tests, and focused relation tests give confidence in the core flows.
- **Secure caching.** `internal/javacache/cache.go` uses `0700` cache dirs, `0600` entry files, SHA-256 content keys, atomic rename writes, and a marker file before `RemoveAll`.
- **Safe external rendering.** `internal/renderer/renderer.go` uses `exec.CommandContext` with separate arguments, avoiding shell injection.
- **Cross-language design.** The `language.Parser` interface lets Java and Go parsers plug in uniformly, and `Finalize` is delegated per language.
- **Tooling clean.** `go vet ./...` reports no issues and `go test ./...` passes.

---

## 3. Issues

### 3.1 High

#### H1: Go implementation inference ignores pointer-receiver semantics
- **File:** `internal/golang/parser.go:337-368`
- **Details:** `methodSet` collects all methods attached to a type, including methods whose receiver is `*T`. `inferImplementations` then treats that set as the method set of the value type `T`. In Go, a value `T` does **not** satisfy an interface that requires a method with a pointer receiver; only `*T` does. This produces false `Implementation` edges.
- **Example:**
  ```go
  type T struct{}
  func (t *T) M() {}
  type I interface { M() }
  ```
  The tool reports `T implements I`, which is incorrect.
- **Fix:** Track receiver pointer-ness in `model.Method` and exclude pointer-receiver methods from the method set of value types during `implements` checks.

#### H2: Custom config parser corrupts unquoted values containing colons
- **File:** `internal/config/config.go:82-93`, `internal/config/config.go:132-181`
- **Details:** `Load` splits each line with `strings.Cut(raw, ":")` on the **first** colon. Unquoted values that themselves contain colons lose everything before the second colon. For example:
  ```yaml
  renderer:
    server_url: http://example.com:8080
  ```
  yields `server_url` = `//example.com:8080`. The same problem affects `source`, `exclude`, `output.file`, etc. Only the quoted form (`"http://example.com:8080"`) works.
- **Fix:** Either require/document quoting for all colon-containing values, or switch to a real YAML parser (e.g. `gopkg.in/yaml.v3`) so the custom parser does not silently mangle configuration.

#### H3: CLI package exclusion only works for dotted Java package names
- **File:** `internal/cli/cli.go:704-712`
- **Details:** `excludePackage` skips any exclude pattern unless `strings.Contains(ex, ".")`:
  ```go
  if strings.Contains(ex, ".") && (pkg == ex || strings.HasPrefix(pkg, ex+".") || strings.HasPrefix(pkg, ex+"/")) {
      return true
  }
  ```
  Go packages (e.g. `cmd`, `app`) and path-style Java exclusions like `src/test` are never excluded at the package-filter stage. Path exclusions are partly handled by `scanner.SourceFiles`, but config-driven package filtering is effectively broken for non-dotted names.
- **Fix:** Treat the exclude list consistently as either package prefixes or path suffixes; avoid requiring a dot. Document the semantics in the help text.

#### H4: Cache invalidates on unrelated renderer/output settings
- **File:** `internal/cli/cli.go:387-396`, `internal/javacache/cache.go:43-69`, `internal/javacache/cache.go:131-138`
- **Details:** The CLI constructs `cacheSettings := cfg` and passes the whole `Config` as the cache settings blob. The cache signature includes `json.Marshal(settings)`, so changing `output.file`, `output.format`, `renderer.type`, `renderer.server_url`, or `renderer.timeout` changes the cache key and causes perfectly valid parse results to be re-parsed. These options do not affect AST extraction.
- **Fix:** Include only values that affect parsing (currently none, or future parse options) in the cache settings; exclude output/renderer fields.

---

### 3.2 Medium

#### M1: Global flag values can be mistaken for commands
- **File:** `internal/cli/cli.go:50-67`
- **Details:** `Run` scans raw arguments for command names before parsing flags. A global flag value that matches a command name breaks parsing. Example:
  ```
  umlgen --config class class src
  ```
  The loop sees the value `class` as the command, leaving `--config` without its value and failing with a flag-parsing error.
- **Fix:** Parse global flags with `flag.ContinueOnError` first (consuming values), or build the command list while skipping known-value global flags.

#### M2: Non-struct/interface Go types are rendered as synthetic classes
- **File:** `internal/golang/parser.go:90-101`
- **Details:** `parseType` defaults to creating a `model.Class` with a synthetic field `value` for type aliases, function types, channel types, etc.:
  ```go
  default:
      t.Fields = append(t.Fields, model.Field{
          Name: "value", Type: expressionText(fset, spec.Type), Visibility: model.Private,
      })
  ```
  This misrepresents `type Handler = func(...)`, `type ID = string`, and similar declarations.
- **Fix:** Skip type aliases (`TypeSpec.Assign` is non-nil) and non-struct/interface named types, or represent them with a more accurate `TypeKind`.

#### M3: Generic type parameters can be resolved as real types
- **File:** `internal/relations/relations.go:209-228`
- **Details:** `References` tokenizes generic signatures such as `List<T>` into `List` and `T`. If the codebase happens to contain a real type named `T`, a dependency edge to it is generated. Type parameters and wildcards are not distinguished from concrete types.
- **Fix:** Either strip generic arguments before tokenizing, or maintain a set of type parameter names per type and exclude them from relation resolution.

#### M4: PlantUML generator emits empty package blocks
- **File:** `internal/plantuml/generator.go:54-64`
- **Details:** When a type has an empty `Package` (e.g., Java default package), `Generate` writes:
  ```plantuml
  package "" {
  ```
  PlantUML accepts this in some versions, but it is fragile and visually wrong.
- **Fix:** Skip the `package` wrapper when `t.Package == ""`.

#### M5: Scanner silently ignores `filepath.Abs` failures
- **File:** `internal/scanner/scanner.go:45-50`, `internal/scanner/scanner.go:60-67`
- **Details:** Both file and directory collection use:
  ```go
  abs, _ := filepath.Abs(path)
  ```
  If `Abs` fails, an empty string is inserted into the result slice and the `seen` map, potentially dropping valid files or producing duplicates.
- **Fix:** Return the error from `filepath.Abs` instead of discarding it.

#### M6: Cache opening unconditionally chmods an existing directory
- **File:** `internal/javacache/cache.go:54-66`
- **Details:** `OpenFor` calls `os.MkdirAll(dir, 0o700)` followed by `os.Chmod(dir, 0o700)`. If the cache root already existed with different permissions (or contained other data), its permissions are forcibly changed.
- **Fix:** Only `Chmod` when the directory is newly created, or rely on `MkdirAll`'s mode argument.

#### M7: `runClassMode` is very long and mixes responsibilities
- **File:** `internal/cli/cli.go:233-616`
- **Details:** The function spans ~380 lines and handles flag parsing, config loading, scanning, caching, parsing, diff integration, focus filtering, diagram generation, file I/O, rendering, and output. This makes the CLI package hard to test in isolation and easy to regress.
- **Fix:** Decompose into smaller, testable helpers such as `loadConfiguration`, `selectParser`, `parseSources`, and `writeOutput`.

#### M8: Hand-rolled YAML parser is limited and undocumented
- **File:** `internal/config/config.go:68-128`
- **Details:** `Load` implements a tiny subset of YAML. It cannot handle inline maps, multi-line values, escaped quotes, or keys/values containing `#` inside quotes in all forms. Because the file extension is `.yaml`, users will expect standard YAML behavior.
- **Fix:** Replace the custom parser with a standard YAML library, or document exactly which YAML subset is supported and validate unsupported constructs explicitly.

---

### 3.3 Low

#### L1: Title sanitization only removes newlines, allowing PlantUML injection
- **File:** `internal/plantuml/generator.go:289-291`, `internal/cli/cli.go:619-624`
- **Details:** `escapeTitle` and `placeholderDiagram` replace `\n`/`\r` but do not escape PlantUML control characters (e.g., `@`, `}`). A malicious or accidental title can inject directives into the generated diagram.
- **Fix:** Strip or escape characters that PlantUML treats as syntax, or reject titles containing them.

#### L2: Tests mutate the process working directory
- **File:** `internal/cli/cli_test.go:73-83`, `internal/cli/cli_test.go:135-145`, `internal/gitdiff/gitdiff_test.go:38-48`
- **Details:** Several tests call `os.Chdir` and restore the original directory in a `defer`. These tests run sequentially in their own packages today, but they are not safe to parallelize and could interact with other tests or harnesses that rely on the working directory.
- **Fix:** Refactor `gitdiff.Analyze` and the diff CLI path to accept an explicit working directory so tests do not need to change the process cwd.

#### L3: `go.mod` declares Go 1.25
- **File:** `go.mod:3`
- **Details:** The module requires `go 1.25`, a version that does not exist in current toolchains. This prevents building with standard released Go versions and will break CI/users on stable Go.
- **Fix:** Set the directive to the minimum released version actually required (e.g., `go 1.23` or `go 1.24`).

#### L4: `normalizeClassArgs` does not support combined short flags
- **File:** `internal/cli/cli.go:627-656`
- **Details:** The custom normalizer only handles long flags and short flags written separately (`-o file`). It will mis-split combined forms such as `-vq` or `-oout`.
- **Fix:** Use the standard `flag` package's built-in parsing for the class subcommand rather than pre-processing arguments, or extend the normalizer to understand combined short flags.

#### L5: Scanner silently ignores exclude patterns that contain a dot but no slash
- **File:** `internal/scanner/scanner.go:72-82`
- **Details:** `excludedPath` skips patterns like `User.java` because of `strings.Contains(ex, ".") && !strings.Contains(ex, "/")`. This behavior is undocumented and surprising.
- **Fix:** Remove the dot/slash heuristic and match exclude patterns as literal path components or glob patterns, documenting the behavior.

#### L6: Renderer hardcodes a 30-second minimum timeout
- **File:** `internal/renderer/renderer.go:79-83`
- **Details:** `Render` replaces `cfg.Timeout <= 0` with `30 * time.Second`, so a user cannot explicitly disable the timeout.
- **Fix:** Honor a zero/negative timeout as "no timeout" or reject it explicitly with an error.

#### L7: Server renderer does not limit response size
- **File:** `internal/renderer/renderer.go:164-206`
- **Details:** `renderServer` copies the HTTP response body directly to disk with `io.Copy`. A misbehaving server could stream an unbounded response and exhaust disk space.
- **Fix:** Use `io.CopyN` or wrap `resp.Body` with `io.LimitReader` and fail if the limit is exceeded.

---

## 4. Recommendations

1. **Adopt a real YAML parser** for configuration (`gopkg.in/yaml.v3` or `github.com/goccy/go-yaml`) to eliminate H2 and M8 and to match user expectations for a `.yaml` file.
2. **Add receiver-pointer tracking** to the Go parser/model so implementation inference respects Go's actual method-set rules (H1).
3. **Refactor the CLI package** by splitting `runClassMode` into focused helpers and introducing integration tests that do not rely on changing `os.Getwd` (M7, L2).
4. **Clarify exclusion semantics** and make them consistent across scanning and package filtering (H3, L5).
5. **Tighten cache settings** to only include parse-relevant values (H4), and fix the unconditional `Chmod` (M6).
6. **Improve edge-case handling** in the scanner (M5), PlantUML generator (M4, L1), and renderer (L6, L7).
7. **Lower the `go.mod` directive** to a released Go version (L3).

---

*Review produced by automated/static analysis of the repository at `/Users/mino/umlgen`.*
