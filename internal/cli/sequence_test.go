package cli

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func requireSequenceJDK(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("Java sequence analysis requires an installed JDK >=17")
	}
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("Java sequence analysis requires an installed JDK >=17")
	}
	version, err := exec.Command(javac, "-version").CombinedOutput()
	if err != nil {
		t.Skipf("JDK version unavailable: %v", err)
	}
	match := regexp.MustCompile(`javac (\d+)`).FindStringSubmatch(string(version))
	if len(match) != 2 {
		t.Skipf("unrecognized JDK version %q", version)
	}
	major, _ := strconv.Atoi(match[1])
	if major < 17 {
		t.Skip("Java sequence analysis requires JDK >=17")
	}
}

func sequenceFixture(t *testing.T, language, root string) string {
	t.Helper()
	if language == "go" {
		packageCLIWrite(t, filepath.Join(root, "go.mod"), "module example.test/sequence\n\ngo 1.25\n")
		path := filepath.Join(root, "flow.go")
		packageCLIWrite(t, path, `package flow
func Nested() {}
func First() int { Nested(); return 1 }
func Second(v int) {}
func Left() {}
func Right() {}
func Entry(choice bool) {
    Second(First())
    if choice { Left() } else { Right() }
    for n := 0; n < 2; n++ { Second(n) }
}
`)
		return path
	}
	path := filepath.Join(root, "Flow.java")
	packageCLIWrite(t, path, `package sample;
class Flow {
    static void Nested() {}
    static int First() { Nested(); return 1; }
    static void Second(int v) {}
    static void Left() {}
    static void Right() {}
    static void Entry(boolean choice) {
        Second(First());
        if (choice) { Left(); } else { Right(); }
        for (int n = 0; n < 2; n++) { Second(n); }
    }
}
`)
	return path
}

var sequenceMessagePattern = regexp.MustCompile(`(?m)^s\d+ .+ : .+$`)

func TestSequenceCLIControlOrderLimitsAndCapturedEvidence(t *testing.T) {
	for _, language := range []string{"go", "java"} {
		t.Run(language, func(t *testing.T) {
			if language == "java" {
				requireSequenceJDK(t)
			}
			root := t.TempDir()
			source := sequenceFixture(t, language, root)
			output := filepath.Join(t.TempDir(), "sequence.puml")
			args := []string{"sequence", root, "--language", language, "--entry", "Entry", "--quiet", "-o", output}
			plain := packageCLISuccess(t, args...)
			if plain.stdout != "" {
				t.Fatalf("quiet mode printed progress: %s", plain.stdout)
			}
			diagram := packageCLIRead(t, output)
			messages := strings.Join(sequenceMessagePattern.FindAllString(diagram, -1), "\n")
			first, nested, second := strings.Index(messages, "First"), strings.Index(messages, "Nested"), strings.Index(messages, "Second")
			if first < 0 || nested <= first || second <= nested {
				t.Fatalf("argument/callee order was flattened or reordered:\n%s", diagram)
			}
			for _, region := range []string{"alt ", "else", "loop "} {
				if !strings.Contains(diagram, region) {
					t.Errorf("lost control region %q:\n%s", region, diagram)
				}
			}
			explained := packageCLISuccess(t, append(args, "--explain")...)
			if packageCLIRead(t, output) != diagram || !strings.Contains(explained.stdout, source) || !strings.Contains(explained.stdout, "Second(First())") {
				t.Fatalf("explanation changed the diagram or lost captured source:\n%s", explained.stdout)
			}
			packageCLISuccess(t, append(args, "--max-call-depth=0")...)
			depth := strings.Join(sequenceMessagePattern.FindAllString(packageCLIRead(t, output), -1), "\n")
			if strings.Contains(depth, "Nested") || !strings.Contains(depth, "First") || !strings.Contains(depth, "Second") {
				t.Fatalf("depth zero did not retain entry calls while omitting callee bodies:\n%s", depth)
			}
			packageCLISuccess(t, append(args, "--max-messages=1")...)
			limited := packageCLIRead(t, output)
			if len(sequenceMessagePattern.FindAllString(limited, -1)) != 1 || !strings.Contains(limited, "omitted") {
				t.Fatalf("message cap silently dropped events or exceeded budget:\n%s", limited)
			}
		})
	}
}

func TestSequenceCLIConfigPrecedenceAndDistinctDefaultOutput(t *testing.T) {
	root := t.TempDir()
	packageCLIChdir(t, root)
	sourceRoot := filepath.Join(root, "project", "sources")
	sequenceFixture(t, "go", sourceRoot)
	cfg := filepath.Join(root, "project", "settings.yaml")
	packageCLIWrite(t, cfg, "language: go\nsource: [sources]\noutput:\n  file: artifacts/sequence.puml\n  format: plantuml\ndiagrams:\n  sequence:\n    entry: NotPresent\n    max_call_depth: 0\n    max_participants: 10\n    max_messages: 100\n")
	packageCLISuccess(t, "--config", cfg, "sequence", "--entry=Entry", "--max-call-depth=1", "--quiet")
	configured := packageCLIRead(t, filepath.Join(root, "project", "artifacts", "sequence.puml"))
	if !strings.Contains(strings.Join(sequenceMessagePattern.FindAllString(configured, -1), "\n"), "Nested") {
		t.Fatalf("CLI entry/depth did not override sequence YAML:\n%s", configured)
	}
	for _, path := range []string{"class-diagram.puml", "package-diagram.puml", "package-change-diagram.puml", "change-diagram.puml"} {
		packageCLIWrite(t, path, path+" must survive\n")
	}
	packageCLISuccess(t, "sequence", sourceRoot, "--language=go", "--entry=Entry", "--quiet")
	generated := packageCLIRead(t, "sequence-diagram.puml")
	if !strings.Contains(generated, "First") || !strings.Contains(generated, "Second") {
		t.Fatal("default sequence output lacks actual entry calls")
	}
	for _, path := range []string{"class-diagram.puml", "package-diagram.puml", "package-change-diagram.puml", "change-diagram.puml"} {
		if packageCLIRead(t, path) != path+" must survive\n" {
			t.Errorf("sequence overwrote %s", path)
		}
	}
}

func TestSequenceCLIPartialAndEntryParseFailures(t *testing.T) {
	root := t.TempDir()
	packageCLIWrite(t, filepath.Join(root, "go.mod"), "module example.test/errors\n\ngo 1.25\n")
	source := filepath.Join(root, "flow.go")
	packageCLIWrite(t, source, "package flow\nfunc Entry() { Missing() }\n")
	output := filepath.Join(t.TempDir(), "sequence.puml")
	args := []string{"sequence", root, "--language=go", "--entry=Entry", "--quiet", "--explain", "-o", output}
	partial := packageCLIRun(args...)
	if partial.code != exitParse || partial.err == nil {
		t.Fatalf("partial analysis: code=%d err=%v", partial.code, partial.err)
	}
	if !strings.Contains(packageCLIRead(t, output), "Missing") || !strings.Contains(partial.stdout, source) || !strings.Contains(partial.stderr, "Missing") {
		t.Fatalf("partial analysis hid unresolved call/source: stdout=%s stderr=%s", partial.stdout, partial.stderr)
	}
	packageCLIWrite(t, source, "package flow\nfunc Entry( {\n")
	const prior = "retain diagram when entry source fails\n"
	packageCLIWrite(t, output, prior)
	failed := packageCLIRun(args...)
	if failed.code != exitParse || failed.err == nil || packageCLIRead(t, output) != prior || !strings.Contains(failed.stdout, source) {
		t.Fatalf("entry parse failure corrupted prior output or lost failure: code=%d err=%v stdout=%s", failed.code, failed.err, failed.stdout)
	}
}

func TestSequenceCLIAmbiguousEntryAndInvalidOptionsPreserveOutput(t *testing.T) {
	root := t.TempDir()
	packageCLIWrite(t, filepath.Join(root, "go.mod"), "module example.test/ambiguous\n\ngo 1.25\n")
	packageCLIWrite(t, filepath.Join(root, "flow.go"), "package flow\ntype A struct{}\ntype B struct{}\nfunc(A) Run(){}\nfunc(B) Run(){}\n")
	output := filepath.Join(t.TempDir(), "prior.puml")
	const prior = "unchanged prior output\n"
	packageCLIWrite(t, output, prior)
	ambiguous := packageCLIRun("sequence", root, "--language=go", "--entry=Run", "-o", output)
	if ambiguous.code != exitArgs || ambiguous.err == nil || !strings.Contains(ambiguous.err.Error(), "A") || !strings.Contains(ambiguous.err.Error(), "B") || packageCLIRead(t, output) != prior {
		t.Fatalf("ambiguous entry did not expose alternatives safely: code=%d err=%v", ambiguous.code, ambiguous.err)
	}
	for _, flags := range [][]string{
		{"--no-cache"}, {"--focus=Run"}, {"--dependency-source=imports"}, {"--layout-direction=left-to-right"},
		{"--max-call-depth=-1"}, {"--max-participants=0"}, {"--max-messages=0"}, {"--analysis-timeout=0s"}, {"--java-release=16"},
	} {
		args := append([]string{"sequence", root, "--language=go", "--entry=Run", "-o", output}, flags...)
		result := packageCLIRun(args...)
		if result.code != exitArgs || result.err == nil || packageCLIRead(t, output) != prior {
			t.Errorf("invalid flags %q changed output: code=%d err=%v", flags, result.code, result.err)
		}
	}
	for _, yaml := range []string{
		"diagrams:\n  sequence:\n    max_message: 10\n", "diagrams:\n  sequence:\n    entry: 42\n", "diagrams:\n  sequence:\n    max_messages: 1.5\n", "diagrams:\n  sequence:\n    max_call_depth: null\n", "diagrams:\n  sequence:\n    analysis_timeout: 1\n",
	} {
		cfg := packageCLIConfig(t, t.TempDir(), yaml)
		result := packageCLIRun("sequence", root, "--config", cfg, "--entry=Run", "-o", output)
		if result.code != exitArgs || result.err == nil || packageCLIRead(t, output) != prior {
			t.Errorf("invalid sequence YAML changed output: %q code=%d err=%v", yaml, result.code, result.err)
		}
	}
}

func TestSequenceCLIJavaParserRecoveryPreservesPriorDiagram(t *testing.T) {
	requireSequenceJDK(t)
	root := t.TempDir()
	source := filepath.Join(root, "Flow.java")
	packageCLIWrite(t, source, "class Flow { static void Valid() {} static void Entry( {\n")
	output := filepath.Join(t.TempDir(), "sequence.puml")
	const prior = "retain diagram instead of showing a recovered empty entry\n"
	packageCLIWrite(t, output, prior)
	result := packageCLIRun("sequence", root, "--language=java", "--entry=Entry", "--quiet", "--explain", "-o", output)
	if result.code != exitParse || result.err == nil || packageCLIRead(t, output) != prior {
		t.Fatalf("javac recovery was accepted as an executable source entry: code=%d err=%v", result.code, result.err)
	}
	if !strings.Contains(result.stdout, source) || !strings.Contains(result.stderr, "compiler.err") {
		t.Fatalf("rejected recovered entry lost the original parse diagnostics: stdout=%s stderr=%s", result.stdout, result.stderr)
	}
}

func TestSequenceCLIGoParserRecoveryPreservesPriorDiagram(t *testing.T) {
	root := t.TempDir()
	packageCLIWrite(t, filepath.Join(root, "go.mod"), "module example.test/recovery\n\ngo 1.25\n")
	source := filepath.Join(root, "flow.go")
	output := filepath.Join(t.TempDir(), "sequence.puml")
	const prior = "retain diagram instead of using a parser-recovered body\n"
	args := []string{"sequence", root, "--language=go", "--entry=Entry", "--quiet", "--explain", "-o", output}
	for _, entry := range []string{
		"func Entry(value int,,) { Local() }\n",
		"func Entry() { Local( }\n",
		"func Entry() { Local()\n",
	} {
		packageCLIWrite(t, source, "package flow\nfunc Local() {}\nfunc Good() { Local() }\n"+entry)
		packageCLIWrite(t, output, prior)
		result := packageCLIRun(args...)
		if result.code != exitParse || result.err == nil || packageCLIRead(t, output) != prior {
			t.Fatalf("recovered Go entry replaced prior output for %q: code=%d err=%v", entry, result.code, result.err)
		}
		if !strings.Contains(result.stdout, source) || !strings.Contains(result.stderr, "parse-error") {
			t.Fatalf("rejected entry lost parse diagnostics: stdout=%s stderr=%s", result.stdout, result.stderr)
		}
		good := packageCLIRun("sequence", root, "--language=go", "--entry=Good", "--quiet", "-o", output)
		if good.code != exitParse || !strings.Contains(packageCLIRead(t, output), "Local") {
			t.Fatalf("parse recovery erased a valid sibling entry: code=%d err=%v", good.code, good.err)
		}
	}
}
