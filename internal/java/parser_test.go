package java

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/relations"
	"github.com/Mino829/umlgen/internal/scanner"
)

func TestParseFile(t *testing.T) {
	source := `package com.example.user;

public class UserService extends BaseService implements UserUseCase, Auditable {
    private final UserRepository repository;
    protected List<User> users;

    public UserService(UserRepository repository) {
        this.repository = repository;
    }

    public User findById(long id) {
        return repository.findById(id);
    }

    private void reset() {}
}`
	path := filepath.Join(t.TempDir(), "UserService.java")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	unit, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	types := unit.Types
	if len(types) != 1 {
		t.Fatalf("got %d types", len(types))
	}
	got := types[0]
	if got.Name != "UserService" || got.Package != "com.example.user" || got.Visibility != model.Public {
		t.Fatalf("unexpected type: %#v", got)
	}
	if len(got.Fields) != 2 || got.Fields[0].Type != "UserRepository" {
		t.Fatalf("unexpected fields: %#v", got.Fields)
	}
	if len(got.Methods) != 3 || !got.Methods[0].Constructor {
		t.Fatalf("unexpected methods: %#v", got.Methods)
	}
	if got.Methods[1].ReturnType != "User" || len(got.Methods[1].Parameters) != 1 {
		t.Fatalf("unexpected findById: %#v", got.Methods[1])
	}
	if len(got.Extends) != 1 || len(got.Implements) != 2 {
		t.Fatalf("unexpected supertypes: %#v %#v", got.Extends, got.Implements)
	}
}

func TestParseInterface(t *testing.T) {
	source := `package sample;
public interface Repository extends Parent {
    User findById(long id);
}`
	path := filepath.Join(t.TempDir(), "Repository.java")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	unit, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	types := unit.Types
	if len(types) != 1 || types[0].Kind != model.Interface {
		t.Fatalf("unexpected types: %#v", types)
	}
	if len(types[0].Extends) != 1 || len(types[0].Methods) != 1 {
		t.Fatalf("unexpected interface: %#v", types[0])
	}
	if types[0].Methods[0].Visibility != model.Public {
		t.Fatalf("interface method should be implicitly public: %#v", types[0].Methods[0])
	}
}

func TestParseRecordAndEnum(t *testing.T) {
	source := `package sample;
public record UserId(long value) implements Comparable<UserId> {
    public int compareTo(UserId other) { return 0; }
}
enum Status { ACTIVE, INACTIVE }`
	path := filepath.Join(t.TempDir(), "Types.java")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	unit, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	types := unit.Types
	if len(types) != 2 {
		t.Fatalf("got %d types: %#v", len(types), types)
	}
	if types[0].Kind != model.Record || len(types[0].Fields) != 1 || types[0].Fields[0].Type != "long" {
		t.Fatalf("unexpected record: %#v", types[0])
	}
	if len(types[0].Implements) != 1 || types[0].Implements[0].Name != "Comparable<UserId>" {
		t.Fatalf("unexpected record interfaces: %#v", types[0].Implements)
	}
	if types[1].Kind != model.Enum || strings.Join(types[1].EnumValues, ",") != "ACTIVE,INACTIVE" {
		t.Fatalf("unexpected enum: %#v", types[1])
	}
}

func TestParseCommonModernDeclarations(t *testing.T) {
	source := `package sample;

public @interface Configuration {
    String value() default "";
    Class<?> target();
}

public abstract sealed class Base<T extends Entity & Comparable<T>> permits Child {
    protected static final String KIND = "base";
    public abstract T load(String id);
}

non-sealed class Child extends Base<Entity> {}

enum Status {
    ACTIVE("active"), INACTIVE("inactive");

    private final String label;
    Status(String label) { this.label = label; }

    static class Metadata {}
}`

	unit, err := ParseSource("Common.java", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	types := unit.Types
	if len(types) != 5 {
		t.Fatalf("types = %d, want 5: %#v", len(types), types)
	}
	byName := map[string]model.Type{}
	for _, parsed := range types {
		byName[parsed.QualifiedName()] = parsed
	}

	annotation := byName["sample.Configuration"]
	if annotation.Kind != model.Annotation || len(annotation.Methods) != 2 ||
		annotation.Methods[0].Name != "value" || annotation.Methods[0].ReturnType != "String" ||
		annotation.Methods[1].ReturnType != "Class<?>" {
		t.Fatalf("annotation = %#v", annotation)
	}
	base := byName["sample.Base"]
	if !base.Abstract || !base.Sealed || base.TypeParameters != "<T extends Entity & Comparable<T>>" {
		t.Fatalf("base modifiers/type parameters = %#v", base)
	}
	if len(base.Fields) != 1 || !base.Fields[0].Static || len(base.Methods) != 1 || !base.Methods[0].Abstract {
		t.Fatalf("base members = %#v %#v", base.Fields, base.Methods)
	}
	if child := byName["sample.Child"]; !child.NonSealed {
		t.Fatalf("child = %#v", child)
	}
	status := byName["sample.Status"]
	if strings.Join(status.EnumValues, ",") != "ACTIVE,INACTIVE" || len(status.Fields) != 1 || len(status.Methods) != 1 {
		t.Fatalf("status = %#v", status)
	}
	if _, ok := byName["sample.Status.Metadata"]; !ok {
		t.Fatalf("nested enum type was not parsed: %#v", byName)
	}
}

func TestParseDeclarationLocations(t *testing.T) {
	const path = "relative/Declarations.java"
	source := `package sample;
class Outer
    extends
        Base<String,
             Integer>
    implements
        First<String, Integer>,
        /* between parents */
        Second {
    private Target first,
        second[];
    Outer(
        Target input) {}
    Target load(
        Target input,
        Other another) { return null; }
    Other load(
        Other input) { return null; }
    record Entry(
        Target value,
        Other other) implements
            First<Target, Other> {
        Entry {}
    }
    @interface Config {
        Target value();
    }
    interface Nested extends
        First<Target, Other>,
        Second {}
}`
	unit, err := ParseSource(path, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	types := unit.Types
	if len(types) != 4 {
		t.Fatalf("types = %d, want 4: %#v", len(types), types)
	}
	assertLocation := func(label, gotSource string, gotLine, wantLine int) {
		t.Helper()
		if gotSource != path || gotLine != wantLine {
			t.Errorf("%s location = %s:%d, want %s:%d", label, gotSource, gotLine, path, wantLine)
		}
	}
	assertParents := func(label string, got []model.TypeReference, names []string, lines []int) {
		t.Helper()
		if len(got) != len(names) {
			t.Fatalf("%s parents = %#v, want %v", label, got, names)
		}
		for i, parent := range got {
			if parent.Name != names[i] || parent.Inferred {
				t.Errorf("%s parent %d = %#v, want explicit %q", label, i, parent, names[i])
			}
			assertLocation(label, parent.Source, parent.Line, lines[i])
		}
	}
	outer := types[0]
	assertLocation("outer", outer.Source, outer.Line, 2)
	assertParents("extends", outer.Extends, []string{"Base<String,\n             Integer>"}, []int{4})
	assertParents("implements", outer.Implements, []string{"First<String, Integer>", "Second"}, []int{7, 9})
	if len(outer.Fields) != 2 || outer.Fields[0].Name != "first" || outer.Fields[0].Type != "Target" ||
		outer.Fields[1].Name != "second" || outer.Fields[1].Type != "Target[]" {
		t.Fatalf("multi-declarator fields = %#v", outer.Fields)
	}
	for _, field := range outer.Fields {
		assertLocation(field.Name, field.Source, field.Line, 10)
	}
	if len(outer.Methods) != 3 || !outer.Methods[0].Constructor ||
		outer.Methods[1].Name != "load" || outer.Methods[1].ReturnType != "Target" ||
		outer.Methods[2].Name != "load" || outer.Methods[2].ReturnType != "Other" {
		t.Fatalf("methods = %#v", outer.Methods)
	}
	for i, wantLine := range []int{12, 14, 17} {
		method := outer.Methods[i]
		assertLocation(method.Name, method.Source, method.Line, wantLine)
		wantParameters := [][]int{{13}, {15, 16}, {18}}[i]
		if len(method.Parameters) != len(wantParameters) {
			t.Fatalf("method %d parameters = %#v", i, method.Parameters)
		}
		for j, parameter := range method.Parameters {
			if parameter.Line != wantParameters[j] {
				t.Errorf("method %d parameter %d line = %d, want %d", i, j, parameter.Line, wantParameters[j])
			}
		}
	}
	entry := types[1]
	assertLocation("record", entry.Source, entry.Line, 19)
	if entry.QualifiedName() != "sample.Outer.Entry" || len(entry.Fields) != 2 ||
		entry.Fields[0].Name != "value" || entry.Fields[0].Type != "Target" ||
		entry.Fields[1].Name != "other" || entry.Fields[1].Type != "Other" {
		t.Fatalf("record = %#v", entry)
	}
	for i, field := range entry.Fields {
		assertLocation(field.Name, field.Source, field.Line, 20+i)
	}
	assertParents("record implements", entry.Implements, []string{"First<Target, Other>"}, []int{22})
	if len(entry.Methods) != 1 || !entry.Methods[0].Constructor {
		t.Fatalf("compact constructor = %#v", entry.Methods)
	}
	assertLocation("compact constructor", entry.Methods[0].Source, entry.Methods[0].Line, 23)
	annotation := types[2]
	assertLocation("annotation", annotation.Source, annotation.Line, 25)
	if len(annotation.Methods) != 1 || annotation.Methods[0].ReturnType != "Target" {
		t.Fatalf("annotation methods = %#v", annotation.Methods)
	}
	assertLocation("annotation method", annotation.Methods[0].Source, annotation.Methods[0].Line, 26)
	nested := types[3]
	assertLocation("nested interface", nested.Source, nested.Line, 28)
	assertParents("interface extends", nested.Extends, []string{"First<Target, Other>", "Second"}, []int{29, 30})
}

func TestParseImportsAndNestedTypes(t *testing.T) {
	source := `package app;
import two.User;
import one.*;
public class Outer {
    private User user;
    static class Inner {
        User value;
    }
}`
	path := filepath.Join(t.TempDir(), "Outer.java")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	unit, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	types := unit.Types
	if len(types) != 2 {
		t.Fatalf("types = %#v", types)
	}
	if len(types[0].Imports) != 2 || types[0].Imports[0].Name != "two.User" ||
		!types[0].Imports[1].Wildcard {
		t.Fatalf("imports = %#v", types[0].Imports)
	}
	if types[1].QualifiedName() != "app.Outer.Inner" || types[1].DisplayName() != "Outer.Inner" {
		t.Fatalf("nested type = %#v", types[1])
	}
}

func TestParseCompatibilityFixture(t *testing.T) {
	root := compatibilityFixturePath(t, "project")
	files, err := scanner.JavaFiles([]string{root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 8 {
		t.Fatalf("files = %d, want 8", len(files))
	}

	byName := map[string]model.Type{}
	for _, file := range files {
		unit, parseErr := ParseFile(file)
		if parseErr != nil {
			t.Fatalf("ParseFile(%s): %v", file, parseErr)
		}
		for _, parsed := range unit.Types {
			byName[parsed.QualifiedName()] = parsed
		}
	}
	if len(byName) != 9 {
		t.Fatalf("types = %d, want 9: %#v", len(byName), byName)
	}

	if command := byName["com.acme.app.Command"]; command.Kind != model.Interface ||
		command.Visibility != model.Public {
		t.Fatalf("sealed interface = %#v", command)
	}
	if annotation := byName["com.acme.shared.DomainType"]; annotation.Kind != model.Annotation ||
		len(annotation.Methods) != 1 || annotation.Methods[0].Name != "value" {
		t.Fatalf("annotation = %#v", annotation)
	}
	identifiable := byName["com.acme.shared.Identifiable"]
	if len(identifiable.Methods) != 1 || identifiable.Methods[0].ReturnType != "T" {
		t.Fatalf("generic interface = %#v", identifiable)
	}
	salesUser := byName["com.acme.sales.User"]
	if salesUser.Visibility != model.Public || len(salesUser.Implements) != 1 ||
		salesUser.Implements[0].Name != "Identifiable<String>" {
		t.Fatalf("annotated class = %#v", salesUser)
	}
	lineItem := byName["com.acme.sales.Order.LineItem"]
	if lineItem.Kind != model.Record || len(lineItem.Fields) != 2 ||
		lineItem.Fields[0].Name != "product" || lineItem.Fields[0].Type != "User" {
		t.Fatalf("nested record = %#v", lineItem)
	}
	service := byName["com.acme.app.UserService"]
	if len(service.Imports) != 4 || !service.Imports[0].Wildcard ||
		service.Imports[0].Name != "com.acme.sales" || service.Imports[1].Name != "com.acme.support.User" {
		t.Fatalf("imports = %#v", service.Imports)
	}
	if len(service.Fields) != 4 || service.Fields[2].Type != "List<Order.LineItem>" ||
		service.Fields[3].Type != "ExternalAuditClient" {
		t.Fatalf("service fields = %#v", service.Fields)
	}
}

func TestParseCompatibilityFixtureReportsSyntaxLine(t *testing.T) {
	_, err := ParseFile(compatibilityFixturePath(t, "broken", "Broken.java"))
	if err == nil || !strings.Contains(err.Error(), "Java syntax error at line 4") {
		t.Fatalf("err = %v", err)
	}
}

func compatibilityFixturePath(t *testing.T, parts ...string) string {
	t.Helper()
	all := append([]string{"..", "..", "testdata", "java", "compatibility"}, parts...)
	path, err := filepath.Abs(filepath.Join(all...))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParsePackageOnlySourceUnit(t *testing.T) {
	const source = "package sample;"
	unit, err := ParseSource("package-info.java", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	wantSpan := model.SourceSpan{
		Start: model.SourcePosition{Offset: 0, Line: 1, Column: 1},
		End:   model.SourcePosition{Offset: 15, Line: 1, Column: 16},
	}
	if unit.Span != wantSpan || unit.Package != (model.PackageDeclaration{Name: "sample", Span: wantSpan}) {
		t.Fatalf("source/package spans = %#v / %#v, want %#v", unit.Span, unit.Package, wantSpan)
	}
	if len(unit.Types) != 0 || len(unit.Imports) != 0 || len(unit.Diagnostics) != 0 {
		t.Fatalf("package-only declarations = %#v", unit)
	}
}

func TestParseUnnamedPackageSourceUnit(t *testing.T) {
	unit, err := ParseSource("Plain.java", []byte("class Plain {}"))
	if err != nil {
		t.Fatal(err)
	}
	if unit.Package != (model.PackageDeclaration{}) || len(unit.Types) != 1 ||
		unit.Types[0].Package != "" || unit.Types[0].QualifiedName() != "Plain" {
		t.Fatalf("unnamed package = %#v", unit)
	}
	wantEnd := model.SourcePosition{Offset: 14, Line: 1, Column: 15}
	if unit.Span.End != wantEnd || len(unit.Diagnostics) != 0 {
		t.Fatalf("unnamed source span/diagnostics = %#v / %#v", unit.Span, unit.Diagnostics)
	}
}

func TestParseSourceUnitStructuralImportsAndPhysicalSpans(t *testing.T) {
	const source = "// π\r\n" +
		"package café /*x*/ . app;\r\n" +
		"import one /*x*/ . Two;\r\n" +
		"import /*x*/ static\tone . Outer /*x*/ . member;\r\n" +
		"import two /*x*/ . *;\r\n" +
		"import static three . Outer /*x*/ . *;\r\n" +
		"import four . Outer /*x*/ . Inner;\r\n" +
		"import static five . Outer /*x*/ . Nested;\r\n" +
		"class Outer { class Inner {} }\r\n" +
		"class Other {}"
	unit, err := ParseSource("Types.java", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	wantSpan := model.SourceSpan{
		Start: model.SourcePosition{Offset: 0, Line: 1, Column: 1},
		End:   model.SourcePosition{Offset: 298, Line: 10, Column: 15},
	}
	wantPackage := model.PackageDeclaration{
		Name: "café.app",
		Span: model.SourceSpan{
			Start: model.SourcePosition{Offset: 7, Line: 2, Column: 1},
			End:   model.SourcePosition{Offset: 33, Line: 2, Column: 27},
		},
	}
	if unit.Span != wantSpan || unit.Package != wantPackage {
		t.Fatalf("source/package spans = %#v / %#v, want %#v / %#v", unit.Span, unit.Package, wantSpan, wantPackage)
	}
	wants := []struct {
		name             string
		static, wildcard bool
		start, end       int
		line, endColumn  int
	}{
		{"one.Two", false, false, 35, 58, 3, 24},
		{"one.Outer.member", true, false, 60, 107, 4, 48},
		{"two", false, true, 109, 130, 5, 22},
		{"three.Outer", true, true, 132, 170, 6, 39},
		{"four.Outer.Inner", false, false, 172, 206, 7, 35},
		{"five.Outer.Nested", true, false, 208, 250, 8, 43},
	}
	if len(unit.Imports) != len(wants) {
		t.Fatalf("imports = %#v, want %d", unit.Imports, len(wants))
	}
	for i, want := range wants {
		expected := model.Import{
			Name: want.name, Static: want.static, Wildcard: want.wildcard,
			Span: model.SourceSpan{
				Start: model.SourcePosition{Offset: want.start, Line: want.line, Column: 1},
				End:   model.SourcePosition{Offset: want.end, Line: want.line, Column: want.endColumn},
			},
		}
		if unit.Imports[i] != expected {
			t.Errorf("import %d = %#v, want %#v", i, unit.Imports[i], expected)
		}
	}
	wantNames := []string{"café.app.Outer", "café.app.Outer.Inner", "café.app.Other"}
	if len(unit.Types) != len(wantNames) {
		t.Fatalf("types = %#v", unit.Types)
	}
	for i, parsed := range unit.Types {
		if parsed.QualifiedName() != wantNames[i] || parsed.Source != "Types.java" {
			t.Errorf("type %d = %#v", i, parsed)
		}
		if !reflect.DeepEqual(parsed.Imports, unit.Imports) {
			t.Errorf("type %d lost its file's imports", i)
		}
	}
	if unit.Types[0].Line != 9 || unit.Types[1].Line != 9 || unit.Types[2].Line != 10 {
		t.Errorf("physical type lines = %d, %d, %d", unit.Types[0].Line, unit.Types[1].Line, unit.Types[2].Line)
	}
	if len(unit.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", unit.Diagnostics)
	}
}

func TestParseModuleDeclarationDiagnostic(t *testing.T) {
	const source = "/* π */\r\nopen module sample.app {\r\n    requires java.base;\r\n    exports sample.api;\r\n}"
	unit, err := ParseSource("module-info.java", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	want := model.SourceDiagnostic{
		Code: "unsupported-module-declaration",
		Span: model.SourceSpan{
			Start: model.SourcePosition{Offset: 10, Line: 2, Column: 1},
			End:   model.SourcePosition{Offset: 87, Line: 5, Column: 2},
		},
	}
	if len(unit.Diagnostics) != 1 || unit.Diagnostics[0] != want {
		t.Fatalf("module diagnostics = %#v, want %#v", unit.Diagnostics, want)
	}
	if unit.Package != (model.PackageDeclaration{}) || len(unit.Imports) != 0 || len(unit.Types) != 0 {
		t.Fatalf("module directives invented package/type dependencies: %#v", unit)
	}
	if unit.Span.End != want.Span.End {
		t.Fatalf("module source end = %#v, want %#v", unit.Span.End, want.Span.End)
	}
}

func TestParseSyntaxDiagnosticHasPhysicalMissingTokenSpan(t *testing.T) {
	const source = "class Good {}\r\nclass Café {"
	unit, err := ParseSource("Broken.java", []byte(source))
	if err == nil {
		t.Fatalf("err = %v", err)
	}
	if unit == nil {
		t.Fatal("syntax failure did not return a source unit")
	}
	wantPosition := model.SourcePosition{Offset: 28, Line: 2, Column: 14}
	want := model.SourceDiagnostic{
		Code: "syntax-error", Span: model.SourceSpan{Start: wantPosition, End: wantPosition},
	}
	if len(unit.Diagnostics) != 1 || unit.Diagnostics[0] != want {
		t.Fatalf("syntax diagnostics = %#v, want %#v", unit.Diagnostics, want)
	}
	if len(unit.Types) != 0 {
		t.Fatalf("syntax failure returned partial types: %#v", unit.Types)
	}
	if unit.Span.End != wantPosition {
		t.Fatalf("syntax source end = %#v", unit.Span.End)
	}
}

func TestGenericDeclarationMethodAndEnclosingScopes(t *testing.T) {
	unit, err := ParseSource("Scopes.java", []byte(`package sample;
class T {}
class U {}
class Bound {}
class Pair<A, B> {}
interface Contract<T> { T take(T input); }
class Box<T extends Bound & Comparable<T>> {
 T value;
 Pair<T, Pair<T, U>> nested;
 sample.T qualified;
 <U extends Bound> U convert(U input) { return input; }
 <U> Box(U input) {}
 U ordinary(U input) { return input; }
 class Inner<V extends T> {
  T enclosing;
  V own;
  U concrete;
 }
 static class Static { T concrete; }
 interface Nested { T concrete(); }
}
class Outside {
 T value;
 <T> T hidden(T input) { return input; }
 T concrete(T input) { return input; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]model.Type{}
	for _, typ := range unit.Types {
		byName[typ.QualifiedName()] = typ
	}
	if got := byName["sample.Box"].TypeParameterNames; !reflect.DeepEqual(got, []string{"T"}) {
		t.Fatalf("bounds were confused with parameters: %#v", got)
	}
	if got := byName["sample.Box.Inner"].TypeParameterNames; !reflect.DeepEqual(got, []string{"T", "V"}) {
		t.Fatalf("nested parameter scope = %#v", got)
	}
	for _, name := range []string{"sample.Box.Static", "sample.Box.Nested", "sample.Outside"} {
		if len(byName[name].TypeParameterNames) != 0 {
			t.Fatalf("outer scope leaked into %s: %#v", name, byName[name])
		}
	}
	analysis := relations.Analyze(relations.Input{Types: unit.Types, Known: unit.Types}, true)
	if len(analysis.Diagnostics) != 0 {
		t.Fatalf("generic placeholders produced diagnostics: %#v", analysis.Diagnostics)
	}
	got := map[string]bool{}
	for _, relation := range analysis.Relations {
		got[relation.From+"|"+relation.Label+"|"+relation.To] = true
	}
	want := map[string]bool{
		"sample.Box|nested|sample.Pair":           true,
		"sample.Box|nested|sample.U":              true,
		"sample.Box|qualified|sample.T":           true,
		"sample.Box|ordinary(input)|sample.U":     true,
		"sample.Box|ordinary|sample.U":            true,
		"sample.Box.Inner|concrete|sample.U":      true,
		"sample.Box.Static|concrete|sample.T":     true,
		"sample.Box.Nested|concrete|sample.T":     true,
		"sample.Outside|value|sample.T":           true,
		"sample.Outside|concrete(input)|sample.T": true,
		"sample.Outside|concrete|sample.T":        true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("generic relations = %#v, want %#v", got, want)
	}
}
