package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestActivityConfigurationRejectsUnknownOrMistypedControls(t *testing.T) {
	for _, yaml := range []string{
		"diagrams:\n  activity: []\n",
		"diagrams:\n  activity:\n    entry: ProcessOrder\n",
		"diagrams:\n  activity:\n    max_nodes: '500'\n",
		"diagrams:\n  activity:\n    function: 42\n",
		"diagrams:\n  activity:\n    analysis_timeout: 30\n",
		"diagrams:\n  activity:\n    java_release: true\n",
		"diagrams:\n  activity:\n    function: one\n    function: two\n",
	} {
		path := filepath.Join(t.TempDir(), ".umlgen.yaml")
		if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := Load(path, true); err == nil {
			t.Errorf("accepted malformed activity controls: %s", yaml)
		}
	}
}
