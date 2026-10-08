package java

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
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
	types, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
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
	types, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
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
	types, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 2 {
		t.Fatalf("got %d types: %#v", len(types), types)
	}
	if types[0].Kind != model.Record || len(types[0].Fields) != 1 || types[0].Fields[0].Type != "long" {
		t.Fatalf("unexpected record: %#v", types[0])
	}
	if len(types[0].Implements) != 1 || types[0].Implements[0] != "Comparable<UserId>" {
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

	types, err := ParseSource("Common.java", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
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
	types, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
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
		types, parseErr := ParseFile(file)
		if parseErr != nil {
			t.Fatalf("ParseFile(%s): %v", file, parseErr)
		}
		for _, parsed := range types {
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
		salesUser.Implements[0] != "Identifiable<String>" {
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
