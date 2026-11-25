//go:build !windows

package cli

import "os"

func readWatchInput(path string, _ int64) ([]byte, error) {
	return os.ReadFile(path)
}
