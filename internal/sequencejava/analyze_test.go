package sequencejava

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Mino829/umlgen/internal/sequence"
)

func requireJDK(t *testing.T) {
	t.Helper()
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("installed JDK compiler is unavailable")
	}
	out, err := exec.Command(javac, "-version").CombinedOutput()
	if err != nil {
		t.Skipf("installed JDK compiler unavailable: %s", out)
	}
	match := regexp.MustCompile(`javac (\d+)`).FindStringSubmatch(string(out))
	if len(match) != 2 {
		t.Skipf("cannot determine JDK compiler version: %s", out)
	}
	version, _ := strconv.Atoi(match[1])
	if version < 17 {
		t.Skip("JDK >=17 required")
	}
	if _, err = exec.LookPath("java"); err != nil {
		t.Skip("Java launcher unavailable")
	}
}
func analyzeText(t *testing.T, text string) sequence.Program {
	t.Helper()
	requireJDK(t)
	root := t.TempDir()
	// Deliberately do not write this file: only the captured bytes exist.
	program, err := Analyze(context.Background(), []sequence.Source{{Path: filepath.Join(root, "Sample.java"), Root: root, Content: []byte(text)}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return program
}
func callable(t *testing.T, p sequence.Program, selector string) sequence.Callable {
	t.Helper()
	for _, c := range p.Callables {
		if c.Selector == selector {
			return c
		}
	}
	t.Fatalf("missing callable %q: %+v", selector, p.Callables)
	return sequence.Callable{}
}
func calls(events []sequence.Event) []sequence.Event {
	var result []sequence.Event
	for _, e := range events {
		if e.Kind == sequence.Call {
			result = append(result, e)
		}
	}
	return result
}
func assertNoErrors(t *testing.T, p sequence.Program) {
	t.Helper()
	for _, d := range p.Diagnostics {
		if d.Error {
			t.Fatalf("unexpected compiler diagnostic: %+v", d)
		}
	}
}

func TestCompilerBindingsEvaluationAndDispatch(t *testing.T) {
	p := analyzeText(t, `package sample;
interface Face { void dynamic(); }
class Base { public void dynamic() {} }
class Child extends Base { public void dynamic() {} }
final class Closed { void fixedOwner() {} }
class Sample implements Face {
  Sample(int x) {}
  private void secret() {}
  static int pick(int x) { return x; }
  static String pick(String x) { return x; }
  final void fixed() {}
  public void dynamic() {}
  static int inner() { return 1; }
  void entry(Face face, Base base, Closed closed) {
    pick(inner()); pick("text"); secret(); fixed(); closed.fixedOwner();
    dynamic(); face.dynamic(); base.dynamic(); new Sample(1);
  }
}`)
	assertNoErrors(t, p)
	entry := callable(t, p, "sample.Sample.entry(sample.Face,sample.Base,sample.Closed)")
	got := calls(entry.Body)
	if len(got) != 10 {
		t.Fatalf("expected ten calls in source evaluation order: %+v", got)
	}
	want := []string{"sample.Sample.inner()", "sample.Sample.pick(int)", "sample.Sample.pick(java.lang.String)", "sample.Sample.secret()", "sample.Sample.fixed()", "sample.Closed.fixedOwner()"}
	for i, label := range want {
		if got[i].Label != label || got[i].Dispatch != "direct" || got[i].Target == "" {
			t.Errorf("call %d: %+v; want bound direct %s", i, got[i], label)
		}
	}
	if got[6].Dispatch != "virtual" || got[6].Target != "" || len(got[6].Candidates) != 1 {
		t.Errorf("virtual self dispatch: %+v", got[6])
	}
	if got[7].Dispatch != "interface" || got[7].Target != "" || len(got[7].Candidates) != 1 {
		t.Errorf("interface dispatch: %+v", got[7])
	}
	if got[8].Dispatch != "virtual" || got[8].Target != "" || len(got[8].Candidates) != 2 {
		t.Errorf("override alternatives: %+v", got[8])
	}
	if got[9].Dispatch != "direct" || !strings.Contains(got[9].Target, "sample.Sample.<init>(int)") {
		t.Errorf("constructor binding: %+v", got[9])
	}
	for _, c := range p.Callables {
		if c.Selector == "sample.Face.dynamic()" {
			t.Error("abstract declaration became an executable entry")
		}
	}
}

func TestScopesAndLiteralBodiesDoNotExecuteOnCreation(t *testing.T) {
	p := analyzeText(t, `package sample;
class Other { static void hit() {} }
class Sample {
  static void hit() {}
  static int initialize() { return 1; }
  int field = initialize();
  static { initialize(); }
  { initialize(); }
  static void entry() {
    class Local { void hit() { Other.hit(); } }
    Runnable value = () -> { Other.hit(); };
    java.util.function.Supplier<String> reference = Object::toString;
    hit(); Other.hit();
  }
}`)
	// The deliberately invalid unbound instance reference gives a diagnostic,
	// but must not prevent real local binding of the remaining calls.
	entry := callable(t, p, "sample.Sample.entry()")
	got := calls(entry.Body)
	if len(got) != 2 || got[0].Label != "sample.Sample.hit()" || got[1].Label != "sample.Other.hit()" {
		t.Fatalf("literal/declaration body ran during creation or shadowing failed: %+v", got)
	}
	var lambda, field, staticInit, instanceInit bool
	for _, c := range p.Callables {
		lambda = lambda || c.Name == "<lambda>"
		field = field || c.Name == "<field:field>"
		staticInit = staticInit || c.Name == "<clinit>"
		instanceInit = instanceInit || c.Name == "<initializer>"
	}
	if !lambda || !field || !staticInit || !instanceInit {
		t.Fatalf("missing separate initialization/lambda regions: %+v", p.Callables)
	}
}

func TestControlFlowAndExceptionalBarriers(t *testing.T) {
	p := analyzeText(t, `package sample;
class Sample {
 static boolean test() { return true; }
 static void hit() {}
 static void entry(boolean value) {
   boolean x = value && test();
   int y = value ? (test() ? 1 : 2) : 3;
   for (hit(); test(); hit()) { if(value) break; hit(); }
   while (test()) { hit(); }
   if (value) { hit(); return; } else { hit(); }
   try { hit(); } catch (RuntimeException ex) { hit(); } finally { hit(); }
   hit();
 }
 static void throwing() { throw new RuntimeException(); }
}`)
	assertNoErrors(t, p)
	entry := callable(t, p, "sample.Sample.entry(boolean)")
	var opt, alt, loop, barrier bool
	for _, e := range entry.Body {
		opt = opt || e.Kind == sequence.Opt
		alt = alt || (e.Kind == sequence.Alt && e.Exhaustive)
		loop = loop || e.Kind == sequence.Loop
		if e.Kind == sequence.Unsupported && len(e.Branches) == 3 {
			barrier = true
		}
	}
	if !opt || !alt || !loop || !barrier {
		t.Fatalf("control regions lost: %+v", entry.Body)
	}
	last := entry.Body[len(entry.Body)-1]
	if last.Kind != sequence.Unsupported {
		t.Fatalf("calls were flattened beyond exceptional barrier: %+v", entry.Body)
	}
	throwing := callable(t, p, "sample.Sample.throwing()")
	if len(throwing.Body) == 0 || throwing.Body[len(throwing.Body)-1].Kind != sequence.Return {
		t.Fatalf("throw must end source path: %+v", throwing.Body)
	}
}

func TestUnicodeSpansDiagnosticsAndPartialBindings(t *testing.T) {
	text := "package sample; class Sample { static void ping() {} static void entry() { String value=\"😀é\"; ping(); Missing.nope(); } }"
	p := analyzeText(t, text)
	entry := callable(t, p, "sample.Sample.entry()")
	got := calls(entry.Body)
	if len(got) != 2 || got[0].Dispatch != "direct" || got[1].Dispatch != "unresolved" {
		t.Fatalf("lost partial compiler bindings: %+v", got)
	}
	start := strings.Index(text, "ping();")
	if got[0].Span.Start.Offset != start || got[0].Span.End.Offset != start+len("ping()") || got[0].Span.Start.Column != start+1 {
		t.Fatalf("not a UTF-8 byte span: %+v; byte offset %d", got[0].Span, start)
	}
	var diagnostic bool
	for _, d := range p.Diagnostics {
		if d.Error && d.File == entry.File && d.Span.Start.Offset > 0 {
			diagnostic = true
		}
	}
	if !diagnostic {
		t.Fatalf("compiler diagnostics missing source spans: %+v", p.Diagnostics)
	}
}

func TestCanceledContextAndInputValidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Analyze(ctx, nil, Options{})
	if err == nil {
		t.Fatal("canceled analysis succeeded")
	}
	if _, lookErr := exec.LookPath("java"); lookErr == nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("lost context cancellation: %v", err)
	}
	if _, err := Analyze(context.Background(), nil, Options{Release: 8}); err == nil {
		t.Fatal("unsupported release accepted")
	}
}

func TestTargetCodeAndAmbientProcessorsAreNeverExecuted(t *testing.T) {
	requireJDK(t)
	root := t.TempDir()
	marker := filepath.Join(root, "EXECUTED")
	processor := `import java.util.*; import javax.annotation.processing.*; import javax.lang.model.*; import javax.lang.model.element.*;
@SupportedAnnotationTypes("*") public class Evil extends AbstractProcessor {
 public SourceVersion getSupportedSourceVersion() { return SourceVersion.latestSupported(); }
 public boolean process(Set<? extends TypeElement> a, RoundEnvironment r) { try { java.nio.file.Files.writeString(java.nio.file.Path.of(System.getenv("UMLGEN_PROCESSOR_MARKER")),"processor executed"); } catch(Exception e) { throw new RuntimeException(e); } return false; }
}`
	javaFile := filepath.Join(root, "Evil.java")
	if err := os.WriteFile(javaFile, []byte(processor), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "javac", "-proc:none", "-d", root, javaFile).CombinedOutput(); err != nil {
		t.Fatalf("compile safety fixture: %v: %s", err, out)
	}
	class, err := os.ReadFile(filepath.Join(root, "Evil.class"))
	if err != nil {
		t.Fatal(err)
	}
	var jar bytes.Buffer
	writer := zip.NewWriter(&jar)
	for name, data := range map[string][]byte{"Evil.class": class, "META-INF/services/javax.annotation.processing.Processor": []byte("Evil\n")} {
		item, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = item.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	jarPath := filepath.Join(root, "evil.jar")
	if err = os.WriteFile(jarPath, jar.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLASSPATH", jarPath)
	t.Setenv("UMLGEN_PROCESSOR_MARKER", marker)
	// Invalid injection flags make sanitization observable without launching code.
	t.Setenv("JAVA_TOOL_OPTIONS", "-XX:UmlgenMustNeverAcceptThisOption")
	t.Setenv("JDK_JAVA_OPTIONS", "--umlgen-must-never-accept-this-option")
	t.Setenv("_JAVA_OPTIONS", "-XX:UmlgenMustNeverAcceptThisOption")
	target := fmt.Sprintf(`class Sample { static { try { java.nio.file.Files.writeString(java.nio.file.Path.of(%q),"target executed"); } catch(Exception e) {} } static void entry() {} }`, marker)
	program, err := Analyze(context.Background(), []sequence.Source{{Path: filepath.Join(root, "Sample.java"), Root: root, Content: []byte(target)}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertNoErrors(t, program)
	graph, err := AnalyzeActivity(context.Background(), []sequence.Source{{Path: filepath.Join(root, "Sample.java"), Root: root, Content: []byte(target)}}, "entry", 20, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 2 || graph.Function.Body != nil {
		t.Fatalf("activity executed initializer or built sequence events: %+v", graph)
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("target code or ambient annotation processor executed: %v", err)
	}
}

func TestCompilerProtocolPreservesUnicodeUnderLegacyStdoutEncoding(t *testing.T) {
	requireJDK(t)
	root := t.TempDir()
	empty := filepath.Join(root, "empty")
	if err := os.Mkdir(empty, 0700); err != nil {
		t.Fatal(err)
	}
	bridge := filepath.Join(root, "CompilerBridge.java")
	if err := os.WriteFile(bridge, helper, 0600); err != nil {
		t.Fatal(err)
	}
	var input bytes.Buffer
	input.WriteString("UMLSEQJ1")
	_ = binary.Write(&input, binary.BigEndian, uint32(17))
	_ = binary.Write(&input, binary.BigEndian, uint32(1))
	for _, value := range []string{filepath.Join(root, "Sample.java"), root, "class Sample { static void 対象() {} static void 入口() { 対象(); } }"} {
		data := []byte(value)
		_ = binary.Write(&input, binary.BigEndian, uint32(len(data)))
		input.Write(data)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// JDK 17 can inherit a non-UTF8 Windows charset; newer JDKs can reproduce
	// it through stdout.encoding. The bridge protocol must not depend on either.
	cmd := exec.CommandContext(ctx, "java", "-Dfile.encoding=windows-1252", "-Dstdout.encoding=windows-1252", "--class-path", empty, "--source", "17", bridge, empty)
	cmd.Dir, cmd.Stdin = root, &input
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("trusted bridge failed: %v", err)
	}
	var program sequence.Program
	if err := json.Unmarshal(output, &program); err != nil {
		t.Fatal(err)
	}
	entry, err := sequence.Select(program, "入口")
	if err != nil {
		t.Fatalf("non-UTF8 JVM stdout corrupted entry identity: %v", err)
	}
	target, err := sequence.Select(program, "対象")
	if err != nil {
		t.Fatal(err)
	}
	events := calls(entry.Body)
	if len(events) != 1 || events[0].Target != target.ID {
		t.Fatalf("non-UTF8 JVM stdout corrupted call binding: %#v", events)
	}
}
