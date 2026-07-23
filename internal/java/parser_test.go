package java

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
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
	if types[1].Kind != model.Enum {
		t.Fatalf("unexpected enum: %#v", types[1])
	}
}
