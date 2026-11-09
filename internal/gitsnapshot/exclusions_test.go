package gitsnapshot_test

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestDottedExclusionsApplyToBothPinnedSnapshots(t *testing.T) {
	root := repository(t)
	put(t, root, "src/User.java", "class User {}")
	put(t, root, "src/SuperUser.java", "class SuperUser {}")
	put(t, root, "src/.generated/Hidden.java", "class Hidden {}")
	before := commit(t, root, "before dotted excludes")
	put(t, root, "src/SuperUser.java", "class SuperUser { int changed; }")
	after := commit(t, root, "after dotted excludes")
	pair := load(t, before+".."+after, []string{"."}, []string{"User.java", ".generated"})
	wantPath := filepath.Join(root, "src", "SuperUser.java")
	if got := files(pair.Before); !reflect.DeepEqual(got, map[string]string{wantPath: "class SuperUser {}"}) {
		t.Fatalf("before snapshot includes excluded paths: %#v", got)
	}
	if got := files(pair.After); !reflect.DeepEqual(got, map[string]string{wantPath: "class SuperUser { int changed; }"}) {
		t.Fatalf("after snapshot includes excluded paths: %#v", got)
	}
	// Explicit files retain their existing precedence in immutable selection.
	explicit := load(t, before, []string{"src/User.java"}, []string{"User.java"})
	if got := files(explicit.Before); !reflect.DeepEqual(got, map[string]string{filepath.Join(root, "src", "User.java"): "class User {}"}) {
		t.Fatalf("explicit target override changed: %#v", got)
	}
}
