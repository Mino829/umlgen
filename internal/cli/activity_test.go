package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func activityFixture(t *testing.T, language, root string) {
	t.Helper()
	if language == "go" {
		packageCLIWrite(t, filepath.Join(root, "go.mod"), "module example.test/activity\n\ngo 1.25\n")
		packageCLIWrite(t, filepath.Join(root, "flow.go"), `package flow
func Check() bool { return true }
func ProcessOrder(ready bool) int {
    if ready && Check() { return 7 }
    for n := 0; n < 3; n++ { if n == 1 { continue }; if !ready { break } }
    return 9
}
func Other() {}
`)
		return
	}
	packageCLIWrite(t, filepath.Join(root, "Flow.java"), `class Flow {
    static boolean Check() { return true; }
    static int ProcessOrder(boolean ready) {
        if (ready && Check()) { return 7; }
        for (int n=0; n<3; n++) { if (n==1) continue; if (!ready) break; }
        return 9;
    }
    static void Other() {}
}`)
}

func TestActivityCLISelectionLimitsAndEvidence(t *testing.T) {
	for _, language := range []string{"go", "java"} {
		t.Run(language, func(t *testing.T) {
			if language == "java" {
				requireSequenceJDK(t)
			}
			root := t.TempDir()
			activityFixture(t, language, root)
			output := filepath.Join(t.TempDir(), "activity.puml")
			args := []string{"activity", root, "--language", language, "--function", "ProcessOrder", "--quiet", "--explain", "-o", output}
			result := packageCLISuccess(t, args...)
			for _, evidence := range []string{"ProcessOrder", "ready", "Check()", "continue", "break", "return 7", "return 9", "half-open"} {
				if !strings.Contains(result.stdout, evidence) {
					t.Errorf("lost CFG/source evidence %q:\n%s", evidence, result.stdout)
				}
			}
			before := packageCLIRead(t, output)
			for _, flags := range [][]string{{"--max-nodes", "2"}, {"--function", "Missing"}, {"--max-nodes", "0"}, {"--no-cache"}, {"--max-call-depth", "2"}} {
				r := packageCLIRun(append(args, flags...)...)
				if r.code != exitArgs || r.err == nil || packageCLIRead(t, output) != before {
					t.Fatalf("%v: expected argument error and preserved output: %+v", flags, r)
				}
			}
			// A changed function selection must not reuse a previous graph/cache.
			r := packageCLISuccess(t, append(args, "--function", "Other")...)
			if !strings.Contains(r.stdout, "Other()") || packageCLIRead(t, output) == before {
				t.Fatal("selected function change reused the previous graph")
			}
		})
	}
}

func TestActivityCLIConfigPrecedenceAndPartialFailure(t *testing.T) {
	root := t.TempDir()
	activityFixture(t, "go", root)
	configPath := filepath.Join(root, ".umlgen.yaml")
	packageCLIWrite(t, configPath, "language: go\nsource: [.]\noutput:\n  file: generated/flow.puml\ndiagrams:\n  activity:\n    function: ProcessOrder\n    max_nodes: 2\n")
	args := []string{"activity", "--config", configPath, "--quiet"}
	r := packageCLIRun(args...)
	if r.code != exitArgs || r.err == nil {
		t.Fatalf("configured cap ignored: %+v", r)
	}
	packageCLISuccess(t, append(args, "--max-nodes", "100")...)
	output := filepath.Join(root, "generated", "flow.puml")
	before := packageCLIRead(t, output)
	packageCLIWrite(t, filepath.Join(root, "broken.go"), "package flow\nfunc Broken( {\n")
	r = packageCLIRun(append(args, "--max-nodes", "100", "--explain")...)
	if r.code != exitParse || r.err == nil || !strings.Contains(r.stderr, "parse") || !strings.Contains(r.stdout, "ProcessOrder") || !strings.Contains(packageCLIRead(t, output), "@enduml") {
		t.Fatalf("partial graph was lost or reported complete: %+v", r)
	}
	// An unavailable damaged selection must preserve the last usable output.
	partial := packageCLIRead(t, output)
	r = packageCLIRun(append(args, "--max-nodes", "100", "--function", "Broken")...)
	if r.code != exitParse || r.err == nil || packageCLIRead(t, output) != partial {
		t.Fatalf("damaged function overwrote output: %+v; original=%s", r, before)
	}
}
