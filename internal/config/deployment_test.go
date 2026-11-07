package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergedDeploymentSettingsCannotBypassSchema(t *testing.T) {
	for _, setting := range []string{"manifest: 123", "unknown: PRIVATE_MERGE_VALUE", "manifest: [PRIVATE_MERGE_VALUE]"} {
		path := filepath.Join(t.TempDir(), "settings.yaml")
		input := "defaults: &base\n  diagrams:\n    deployment:\n      " + setting + "\n<<: *base\n"
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		_, _, err := Load(path, true)
		if err == nil {
			t.Fatalf("merged invalid setting bypassed deployment schema: %s", setting)
		}
		if strings.Contains(err.Error(), "PRIVATE_MERGE_VALUE") {
			t.Fatal("merged deployment validation disclosed a private value")
		}
	}
}
