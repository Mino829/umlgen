package golang

import (
	"path/filepath"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/scanner"
)

func TestParseAndFinalizeCompatibilityFixture(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "go", "compatibility")
	files, err := scanner.SourceFiles([]string{root}, nil, ".go")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("files = %d, want 3", len(files))
	}

	var parsed []model.Type
	for _, file := range files {
		types, parseErr := ParseFile(file)
		if parseErr != nil {
			t.Fatalf("ParseFile(%s): %v", file, parseErr)
		}
		parsed = append(parsed, types...)
	}
	types := Finalize(parsed)
	byName := map[string]model.Type{}
	for _, parsedType := range types {
		byName[parsedType.QualifiedName()] = parsedType
	}

	user := byName["example.com/shop/domain.User"]
	if user.Kind != model.Struct || len(user.Fields) != 4 || len(user.Methods) != 1 {
		t.Fatalf("user = %#v", user)
	}
	if user.Fields[2].Type != "*Profile" || user.Fields[3].Type != "[]Tag" {
		t.Fatalf("user fields = %#v", user.Fields)
	}

	repository := byName["example.com/shop/service.Repository"]
	if repository.Kind != model.Interface || len(repository.Extends) != 1 || repository.Extends[0] != "Finder" {
		t.Fatalf("repository = %#v", repository)
	}
	memory := byName["example.com/shop/service.MemoryRepository"]
	if !contains(memory.Implements, "example.com/shop/service.Repository") {
		t.Fatalf("memory repository implementations = %#v", memory.Implements)
	}
	service := byName["example.com/shop/service.UserService"]
	if len(service.Extends) != 1 || service.Extends[0] != "BaseService" {
		t.Fatalf("service embedding = %#v", service.Extends)
	}
	if !contains(service.Implements, "example.com/shop/service.Repository") {
		t.Fatalf("service implementations = %#v", service.Implements)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestParseSourceReportsSyntaxError(t *testing.T) {
	_, err := ParseSource("broken.go", []byte("package broken\ntype User struct {"))
	if err == nil {
		t.Fatal("expected syntax error")
	}
}

func TestMethodsFromSeparateFilesAreMerged(t *testing.T) {
	declaration, err := ParseSource("user.go", []byte("package sample\ntype User struct{}"))
	if err != nil {
		t.Fatal(err)
	}
	methods, err := ParseSource("user_methods.go", []byte("package sample\nfunc (u *User) Name() string { return \"\" }"))
	if err != nil {
		t.Fatal(err)
	}
	types := Finalize(append(declaration, methods...))
	if len(types) != 1 || len(types[0].Methods) != 1 || types[0].Methods[0].Name != "Name" {
		t.Fatalf("types = %#v", types)
	}
}

func TestEmbeddedInterfaceMethodsAreRequired(t *testing.T) {
	types := Finalize([]model.Type{
		{
			Package: "sample", Name: "Finder", Kind: model.Interface,
			Methods: []model.Method{{Name: "Find", ReturnType: "string"}},
		},
		{
			Package: "sample", Name: "Repository", Kind: model.Interface,
			Extends: []string{"Finder"},
			Methods: []model.Method{{Name: "Save", ReturnType: "error"}},
		},
		{
			Package: "sample", Name: "SaveOnly", Kind: model.Struct,
			Methods: []model.Method{{Name: "Save", ReturnType: "error"}},
		},
	})
	for _, parsedType := range types {
		if parsedType.Name == "SaveOnly" && contains(parsedType.Implements, "sample.Repository") {
			t.Fatalf("SaveOnly must not implement Repository: %#v", parsedType.Implements)
		}
	}
}
