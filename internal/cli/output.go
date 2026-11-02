package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mino829/umlgen/internal/renderer"
)

func diagramOutputPath(output string) string {
	ext := strings.ToLower(filepath.Ext(output))
	if ext == ".svg" || ext == ".png" {
		output = strings.TrimSuffix(output, filepath.Ext(output)) + ".puml"
	}
	if filepath.Ext(output) == "" {
		output += ".puml"
	}
	return output
}

func writeDiagram(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %s", filepath.Dir(path))
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write output file: %s", path)
	}
	return nil
}

func renderDiagram(format, path string, cfg renderer.Config, quiet bool, stdout, stderr io.Writer) (int, error) {
	if format != "svg" && format != "png" {
		return exitOK, nil
	}
	if cfg.Type == renderer.TypeServer && !quiet {
		fmt.Fprintln(stdout, "Note: the PlantUML server receives the generated PlantUML, which may include private identifiers. The default renderer keeps it local.")
	}
	image, err := renderer.Render(format, path, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "Warning: PlantUML file was generated, but %s rendering failed: %v\n", strings.ToUpper(format), err)
		return exitRender, nil
	}
	if !quiet {
		fmt.Fprintf(stdout, "Generated %s\n", image)
	}
	return exitOK, nil
}
