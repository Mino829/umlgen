package renderer

import (
	"bytes"
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Type selects how PlantUML diagrams are rendered.
type Type string

const (
	TypeAuto   Type = "auto"
	TypeLocal  Type = "local"
	TypeJar    Type = "jar"
	TypeServer Type = "server"
)

// Config controls rendering behaviour.
type Config struct {
	Type      Type
	ServerURL string
	Timeout   time.Duration
}

// Defaults returns a Config that auto-detects a local renderer.
func Defaults() Config {
	return Config{Type: TypeAuto, Timeout: 30 * time.Second}
}

// Validate checks the renderer configuration.
func (c Config) Validate() error {
	switch c.Type {
	case TypeAuto, TypeLocal, TypeJar, TypeServer:
	default:
		return fmt.Errorf("unsupported renderer: %q (supported: auto, local, jar, server)", c.Type)
	}
	if c.Type == TypeServer && c.ServerURL == "" {
		return errors.New("renderer server requires a server_url or PLANTUML_SERVER environment variable")
	}
	return nil
}

// Render converts the PlantUML file at pumlPath to the requested format and
// returns the path of the generated image. The original .puml file is kept.
// Supported formats are "svg" and "png".
func Render(format, pumlPath string, cfg Config) (string, error) {
	format = strings.ToLower(format)
	if format != "svg" && format != "png" {
		return "", fmt.Errorf("unsupported render format: %q (supported: svg, png)", format)
	}

	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if err := cfg.Validate(); err != nil {
		return "", err
	}

	outPath := outputPath(pumlPath, format)
	if outPath == pumlPath {
		return "", fmt.Errorf("output path %q would overwrite the input .puml file", outPath)
	}

	ctx := context.Background()
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	switch cfg.Type {
	case TypeServer:
		return renderServer(ctx, format, pumlPath, cfg.ServerURL)
	case TypeJar:
		return renderJar(ctx, format, pumlPath)
	case TypeLocal:
		return renderLocal(ctx, format, pumlPath)
	default: // auto
		return renderAuto(ctx, format, pumlPath, cfg)
	}
}

func renderAuto(ctx context.Context, format, pumlPath string, cfg Config) (string, error) {
	localPath, localErr := renderLocal(ctx, format, pumlPath)
	if localErr == nil {
		return localPath, nil
	}
	if jar := os.Getenv("PLANTUML_JAR"); jar != "" {
		jarPath, jarErr := renderJar(ctx, format, pumlPath)
		if jarErr == nil {
			return jarPath, nil
		}
		return "", fmt.Errorf("PlantUML renderer failed. local command: %v; PLANTUML_JAR: %v", localErr, jarErr)
	}
	return "", fmt.Errorf(`PlantUML renderer was not found.
local command error: %v
To render images, choose one of:
  1. Install the plantuml command and add it to PATH.
  2. Set PLANTUML_JAR to the path of plantuml.jar.
  3. Set renderer to "server" in .umlgen.yaml and provide server_url.
The .puml file has still been generated.`, localErr)
}

func renderLocal(ctx context.Context, format, pumlPath string) (string, error) {
	binary, err := exec.LookPath("plantuml")
	if err != nil {
		return "", err
	}
	return runCommand(ctx, exec.CommandContext(ctx, binary, typeFlag(format), pumlPath), format, pumlPath)
}

func renderJar(ctx context.Context, format, pumlPath string) (string, error) {
	jar := os.Getenv("PLANTUML_JAR")
	if jar == "" {
		return "", errors.New("PLANTUML_JAR is not set")
	}
	return runCommand(ctx, exec.CommandContext(ctx, "java", "-jar", jar, typeFlag(format), pumlPath), format, pumlPath)
}

func typeFlag(format string) string {
	if format == "png" {
		return "-tpng"
	}
	return "-tsvg"
}

func runCommand(ctx context.Context, cmd *exec.Cmd, format, pumlPath string) (string, error) {
	outPath := outputPath(pumlPath, format)
	// Remove a stale output file so that a previous successful render is not
	// mistaken for the result of this invocation.
	_ = os.Remove(outPath)

	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("rendering timed out: %w", ctx.Err())
		}
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	info, err := os.Stat(outPath)
	if err != nil {
		return "", fmt.Errorf("renderer did not produce output file %q", outPath)
	}
	if info.IsDir() {
		return "", fmt.Errorf("renderer output path %q is a directory", outPath)
	}
	return outPath, nil
}

func outputPath(pumlPath, format string) string {
	return strings.TrimSuffix(pumlPath, filepath.Ext(pumlPath)) + "." + format
}

func renderServer(ctx context.Context, format, pumlPath, serverURL string) (string, error) {
	source, err := os.ReadFile(pumlPath)
	if err != nil {
		return "", fmt.Errorf("failed to read puml file: %w", err)
	}

	encoded, err := encodePlantUML(source)
	if err != nil {
		return "", fmt.Errorf("failed to encode PlantUML diagram: %w", err)
	}

	serverURL = strings.TrimRight(serverURL, "/")
	url := fmt.Sprintf("%s/%s/%s", serverURL, format, encoded)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build PlantUML server request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("PlantUML server request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("PlantUML server returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	outPath := outputPath(pumlPath, format)
	outFile, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		return "", fmt.Errorf("failed to write output file: %w", err)
	}
	return outPath, nil
}

// encodePlantUML encodes source text using PlantUML's deflate + 6-bit encoding.
func encodePlantUML(source []byte) (string, error) {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		return "", err
	}
	if _, err := w.Write(source); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return encode6bitGroups(buf.Bytes()), nil
}

func encode6bitGroups(data []byte) string {
	var sb strings.Builder
	for i := 0; i < len(data); i += 3 {
		b1 := int(data[i])
		b2 := 0
		b3 := 0
		if i+1 < len(data) {
			b2 = int(data[i+1])
		}
		if i+2 < len(data) {
			b3 = int(data[i+2])
		}
		v := (b1 << 16) | (b2 << 8) | b3
		sb.WriteByte(encode6bit((v >> 18) & 0x3F))
		sb.WriteByte(encode6bit((v >> 12) & 0x3F))
		sb.WriteByte(encode6bit((v >> 6) & 0x3F))
		sb.WriteByte(encode6bit(v & 0x3F))
	}
	return sb.String()
}

func encode6bit(b int) byte {
	if b < 10 {
		return byte('0' + b)
	}
	b -= 10
	if b < 26 {
		return byte('A' + b)
	}
	b -= 26
	if b < 26 {
		return byte('a' + b)
	}
	if b == 0 {
		return '-'
	}
	return '_'
}
