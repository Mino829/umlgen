package workflowfilters_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// Exercise the shipped script, rather than a second implementation of its parser.
type workflowHarness struct {
	node   string
	script string
}

type workflowResult struct {
	err     error
	output  string
	summary string
	log     string
}

func loadWorkflow(t *testing.T) workflowHarness {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("Node is required to execute the workflow's inline JavaScript: %v", err)
	}
	node, err = filepath.Abs(node)
	if err != nil {
		t.Fatal(err)
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate workflow test source")
	}
	workflowPath := filepath.Join(filepath.Dir(source), "..", "..", ".github", "workflows", "umlgen-review-filters.yml")
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				ID   string `yaml:"id"`
				With struct {
					Script string `yaml:"script"`
				} `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	var script string
	for _, step := range workflow.Jobs["validate"].Steps {
		if step.ID == "parse" {
			if script != "" {
				t.Fatal("multiple parse steps in validation workflow")
			}
			script = step.With.Script
		}
	}
	if script == "" {
		t.Fatal("validation workflow has no inline parse script")
	}
	return workflowHarness{node: node, script: script}
}

func (h workflowHarness) run(t *testing.T, event any, depth, direction, dir string) workflowResult {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{
		"GITHUB_EVENT_PATH":   filepath.Join(dir, "event.json"),
		"GITHUB_OUTPUT":       filepath.Join(dir, "output"),
		"GITHUB_STEP_SUMMARY": filepath.Join(dir, "summary"),
	}
	for key, path := range paths {
		var data []byte
		if key == "GITHUB_EVENT_PATH" {
			data = eventJSON
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	scriptPath := filepath.Join(dir, "parse.cjs")
	if err := os.WriteFile(scriptPath, []byte(h.script), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, h.node, scriptPath)
	cmd.Dir = dir
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		_, replaced := paths[key]
		if !replaced && key != "INPUT_DEPTH" && key != "INPUT_DIRECTION" && key != "NODE_OPTIONS" {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	for key, value := range paths {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.Env = append(cmd.Env, "INPUT_DEPTH="+depth, "INPUT_DIRECTION="+direction)
	log, runErr := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("workflow script did not finish: %v", ctx.Err())
	}
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	return workflowResult{err: runErr, output: read(paths["GITHUB_OUTPUT"]), summary: read(paths["GITHUB_STEP_SUMMARY"]), log: string(log)}
}

func prEvent(body string) any {
	return map[string]any{"pull_request": map[string]any{"body": body}}
}

func fenced(flags string) string {
	return "Review context kept out of outputs.\n\n```umlgen\n" + flags + "\n```\n"
}

func requireSuccess(t *testing.T, result workflowResult, body, source string, depth int, direction string, include, relations *string) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("workflow rejected valid input: %v\n%s\n%s", result.err, result.log, result.summary)
	}
	outputs := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSuffix(result.output, "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || (key != "args-json" && key != "metadata-json") {
			t.Fatalf("unexpected workflow output line %q", line)
		}
		if _, duplicate := outputs[key]; duplicate {
			t.Fatalf("duplicate workflow output %q", key)
		}
		outputs[key] = value
	}
	if len(outputs) != 2 {
		t.Fatalf("expected both JSON outputs, got %q", result.output)
	}
	var args []string
	if err := json.Unmarshal([]byte(outputs["args-json"]), &args); err != nil {
		t.Fatalf("invalid args JSON: %v", err)
	}
	wantArgs := map[string]string{"--depth": strconv.Itoa(depth), "--direction": direction}
	if include != nil {
		wantArgs["--include"] = *include
	}
	if relations != nil {
		wantArgs["--relations"] = *relations
	}
	if len(args) != 2*len(wantArgs) {
		t.Fatalf("argv = %q, want exactly flag/value pairs %v", args, wantArgs)
	}
	gotArgs := make(map[string]string)
	for i := 0; i < len(args); i += 2 {
		if _, duplicate := gotArgs[args[i]]; duplicate {
			t.Fatalf("duplicate argv flag: %q", args)
		}
		gotArgs[args[i]] = args[i+1]
	}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Errorf("argv = %q, want semantic argv %v", args, wantArgs)
	}
	digest := sha256.Sum256([]byte(body))
	wantMetadata := map[string]any{
		"source":      source,
		"body_sha256": hex.EncodeToString(digest[:]),
		"filters": map[string]any{
			"depth": float64(depth), "direction": direction,
			"include": nil, "relations": nil,
		},
		"applies_to": "change-diagram",
	}
	if include != nil {
		wantMetadata["filters"].(map[string]any)["include"] = *include
	}
	if relations != nil {
		wantMetadata["filters"].(map[string]any)["relations"] = *relations
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(outputs["metadata-json"]), &metadata); err != nil {
		t.Fatalf("invalid metadata JSON: %v", err)
	}
	if !reflect.DeepEqual(metadata, wantMetadata) {
		t.Errorf("metadata = %#v, want %#v", metadata, wantMetadata)
	}
	if body != "" {
		for name, data := range map[string]string{"outputs": result.output, "summary": result.summary, "log": result.log} {
			if strings.Contains(data, body) {
				t.Errorf("%s discloses the full PR body", name)
			}
		}
	}
}

func requireFailure(t *testing.T, result workflowResult) {
	t.Helper()
	if result.err == nil {
		t.Fatalf("workflow accepted invalid input; outputs: %s", result.output)
	}
	if strings.Contains(result.output, "args-json=") || strings.Contains(result.output, "metadata-json=") {
		t.Errorf("failed validation emitted success outputs: %q", result.output)
	}
	if strings.TrimSpace(result.summary) == "" {
		t.Error("failed validation did not write a failure summary")
	}
}

func text(value string) *string { return &value }

func TestWorkflowFilterDefaults(t *testing.T) {
	h := loadWorkflow(t)
	for _, tc := range []struct {
		name      string
		event     any
		body      string
		depth     int
		direction string
	}{
		{name: "empty body", event: prEvent(""), depth: 1, direction: "both"},
		{name: "no pull request", event: map[string]any{}, depth: 1, direction: "both"},
		{name: "null body", event: map[string]any{"pull_request": map[string]any{"body": nil}}, depth: 2, direction: "in"},
		{name: "custom depth above PR maximum", body: "Review-only prose with Japanese 日本語 and CRLF.\r\n", depth: 27, direction: "out"},
		{name: "inline is not a fence", body: "The `umlgen --depth 0` example is not configuration.\n", depth: 0, direction: "both"},
		{name: "extra info is a foreign fence", body: "```umlgen example\n--depth 0\n```\n", depth: 15, direction: "in"},
		{name: "four spaces are not a fence", body: "    ```umlgen\n    --depth 0\n    ```\n", depth: 3, direction: "out"},
		{name: "long foreign fence", body: "````markdown\n```umlgen\n--depth 0\n```\n````\n", depth: 19, direction: "both"},
		{name: "foreign fence of other character", body: "```text\n~~~umlgen\n--depth 0\n~~~\n```\n", depth: 4, direction: "in"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := tc.event
			if event == nil {
				event = prEvent(tc.body)
			}
			requireSuccess(t, h.run(t, event, strconv.Itoa(tc.depth), tc.direction, ""), tc.body, "workflow-defaults", tc.depth, tc.direction, nil, nil)
		})
	}
}

func TestWorkflowFilterAcceptedSyntax(t *testing.T) {
	h := loadWorkflow(t)
	for _, tc := range []struct {
		name      string
		body      string
		depth     int
		direction string
		include   *string
		relations *string
	}{
		{name: "quoted Unicode prefix", body: fenced("--depth 0 --direction in --include '日本.資料.é' --relations field,return"), depth: 0, direction: "in", include: text("日本.資料.é"), relations: text("field,return")},
		{name: "equals and all relation kinds", body: fenced("--direction=out --relations=inheritance,implementation,field,parameter,return --include=\"com.alpha\" --depth=10"), depth: 10, direction: "out", include: text("com.alpha"), relations: text("inheritance,implementation,field,parameter,return")},
		{name: "escaped prefix and flag equals", body: fenced("--include com\\.alpha --depth\\=2 --direction b\\oth"), depth: 2, direction: "both", include: text("com.alpha")},
		{name: "backslash continuation and dollar identifier", body: fenced("--include app.\\$Model \\\n--depth 4"), depth: 4, direction: "out", include: text("app.$Model")},
		{name: "quotes on flag and value", body: fenced("'--depth' '3' \"--direction\" \"in\""), depth: 3, direction: "in"},
		{name: "all relations", body: fenced("--relations all"), depth: 27, direction: "out", relations: text("all")},
		{name: "retain trusted defaults", body: fenced("--include com.review"), depth: 27, direction: "out", include: text("com.review")},
		{name: "tilde fence indentation and longer close", body: "  ~~~umlgen\n--depth 5\n   ~~~~~\n", depth: 5, direction: "out"},
		{name: "long fence with longer close", body: "````umlgen\n--depth 6\n``````\n", depth: 6, direction: "out"},
		{name: "CRLF and tabs", body: "```umlgen\r\n\t--depth\t7\r\n--direction=in\r\n```\r\n", depth: 7, direction: "in"},
		{name: "foreign example before actual block", body: "````markdown\n```umlgen\n--output /untrusted\n```\n````\n" + fenced("--depth 8"), depth: 8, direction: "out"},
		{name: "foreign character before actual block", body: "~~~text\n```umlgen\n--output /untrusted\n```\n~~~\n" + fenced("--depth 9"), depth: 9, direction: "out"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireSuccess(t, h.run(t, prEvent(tc.body), "27", "out", ""), tc.body, "pr-body", tc.depth, tc.direction, tc.include, tc.relations)
		})
	}
}

func TestWorkflowFilterRejectedSyntax(t *testing.T) {
	h := loadWorkflow(t)
	cases := []struct {
		name string
		body string
	}{
		{name: "duplicate depth", body: fenced("--depth 1 --depth=2")},
		{name: "duplicate direction", body: fenced("--direction in --direction out")},
		{name: "duplicate include", body: fenced("--include app --include=other")},
		{name: "duplicate relations", body: fenced("--relations all --relations field")},
		{name: "two blocks", body: fenced("--depth 1") + fenced("--depth 2")},
		{name: "empty block", body: fenced("")},
		{name: "whitespace block", body: fenced(" \t\n ")},
		{name: "unclosed block", body: "```umlgen\n--depth 1\n"},
		{name: "wrong close character", body: "```umlgen\n--depth 1\n~~~\n"},
		{name: "shorter close", body: "````umlgen\n--depth 1\n```\n"},
		{name: "close with text", body: "```umlgen\n--depth 1\n``` trailing\n"},
		{name: "positional target", body: fenced("--depth 1 ./src")},
		{name: "lone separator", body: fenced("--")},
		{name: "missing value", body: fenced("--depth")},
		{name: "flag instead of value", body: fenced("--depth --direction in")},
		{name: "empty value", body: fenced("--include=\"\"")},
		{name: "multiple include prefixes unsupported by CLI", body: fenced("--include app,other")},
		{name: "quoted multiple include prefixes", body: fenced("--include 'app, other'")},
		{name: "unterminated single quote", body: fenced("--include 'app")},
		{name: "unterminated double quote", body: fenced("--include \"app")},
		{name: "NUL control", body: fenced("--include app\x00name")},
		{name: "vertical tab control", body: fenced("--depth\v1")},
		{name: "form feed control", body: fenced("--depth\f1")},
	}
	for _, flag := range []string{"--help", "--config=other.yml", "--output=result", "--server=https://example.invalid", "--renderer=local", "--language=go", "--format=svg", "--depth-limit=1", "-h", "--unknown=value"} {
		cases = append(cases, struct{ name, body string }{name: "forbidden " + flag, body: fenced(flag)})
	}
	for _, value := range []string{"-1", "+1", "1.0", "1e0", "01", "00", "11", "999999999999999999999999", "NaN", "Infinity", "１"} {
		cases = append(cases, struct{ name, body string }{name: "depth " + value, body: fenced("--depth=" + value)})
	}
	for _, value := range []string{"IN", "outside", "all", "\" in \""} {
		cases = append(cases, struct{ name, body string }{name: "direction " + value, body: fenced("--direction=" + value)})
	}
	for _, value := range []string{"app.*", "app/path", "app\\\\name", "app..name", ".app", "app.", "1app", "app-name", "app,,other", ",app", "app,", "\"app,   \"", "app.😀", "app.\u0301name"} {
		cases = append(cases, struct{ name, body string }{name: "include " + value, body: fenced("--include=" + value)})
	}
	for _, value := range []string{"unknown", "field,field", "all,field", "field,all", "all,all", "field,,return", ",field", "field,", "FIELD", "\"\""} {
		cases = append(cases, struct{ name, body string }{name: "relations " + value, body: fenced("--relations=" + value)})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireFailure(t, h.run(t, prEvent(tc.body), "1", "both", ""))
		})
	}
}

func TestWorkflowFilterSizeBoundaries(t *testing.T) {
	h := loadWorkflow(t)
	t.Run("include exactly 1024 UTF8 bytes", func(t *testing.T) {
		include := "app." + strings.Repeat("界", 340)
		body := fenced("--include=" + include)
		requireSuccess(t, h.run(t, prEvent(body), "1", "both", ""), body, "pr-body", 1, "both", &include, nil)
	})
	t.Run("include over 1024 UTF8 bytes", func(t *testing.T) {
		body := fenced("--include=app." + strings.Repeat("界", 340) + "a")
		requireFailure(t, h.run(t, prEvent(body), "1", "both", ""))
	})
	t.Run("body exactly 256KiB", func(t *testing.T) {
		body := strings.Repeat("界", (256*1024)/3) + "a"
		requireSuccess(t, h.run(t, prEvent(body), "1", "both", ""), body, "workflow-defaults", 1, "both", nil, nil)
	})
	t.Run("body over 256KiB", func(t *testing.T) {
		body := strings.Repeat("界", (256*1024)/3) + "ab"
		requireFailure(t, h.run(t, prEvent(body), "1", "both", ""))
	})
	t.Run("block below 8KiB", func(t *testing.T) {
		body := fenced("--depth 1" + strings.Repeat(" ", 8*1024-32))
		requireSuccess(t, h.run(t, prEvent(body), "1", "both", ""), body, "pr-body", 1, "both", nil, nil)
	})
	t.Run("block over 8KiB", func(t *testing.T) {
		body := fenced("--depth 1" + strings.Repeat(" ", 8*1024))
		requireFailure(t, h.run(t, prEvent(body), "1", "both", ""))
	})
	t.Run("over 32 tokens", func(t *testing.T) {
		body := fenced("--include app " + strings.Repeat("extra ", 31))
		requireFailure(t, h.run(t, prEvent(body), "1", "both", ""))
	})
}

func TestWorkflowFilterTrustedDefaultsMustBeValid(t *testing.T) {
	h := loadWorkflow(t)
	for _, tc := range []struct{ depth, direction string }{
		{"-1", "both"}, {"1.5", "both"}, {"NaN", "both"}, {"1", "invalid"}, {"1", "IN"},
	} {
		t.Run(tc.depth+"/"+tc.direction, func(t *testing.T) {
			requireFailure(t, h.run(t, prEvent("No configuration block."), tc.depth, tc.direction, ""))
		})
	}
}

func TestWorkflowFilterNeverExecutesPayloads(t *testing.T) {
	h := loadWorkflow(t)
	for _, kind := range []string{"command substitution", "backticks", "semicolon", "newline", "output injection", "HTML summary injection"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "must-not-exist")
			command := "touch " + strconv.Quote(filepath.ToSlash(marker))
			var flags string
			switch kind {
			case "command substitution":
				flags = "--include 'app.$(" + command + ")'"
			case "backticks":
				flags = "--include 'app.`" + command + "`'"
			case "semicolon":
				flags = "--include \"app; " + command + "\""
			case "newline":
				flags = "--include app\n" + command
			case "output injection":
				flags = "--include \"app\nargs-json=[\\\"--output\\\",\\\"" + filepath.ToSlash(marker) + "\\\"]\""
			case "HTML summary injection":
				flags = "--include '<script>" + command + "</script>'"
			}
			result := h.run(t, prEvent(fenced(flags)), "1", "both", dir)
			requireFailure(t, result)
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("payload marker must remain nonexistent; stat error: %v", err)
			}
			if strings.Contains(result.summary, "<script>") || strings.Contains(result.summary, marker) {
				t.Error("failure summary renders untrusted payload content")
			}
		})
	}
	t.Run("literal dollar identifier is not expansion", func(t *testing.T) {
		include := "app.$HOME"
		body := fenced(fmt.Sprintf("--include '%s'", include))
		requireSuccess(t, h.run(t, prEvent(body), "1", "both", ""), body, "pr-body", 1, "both", &include, nil)
	})
}
