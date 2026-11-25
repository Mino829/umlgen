package cli

import (
	"bytes"
	"math"
	"os"
	"strings"
	"syscall"
)

func readWatchInput(path string, size int64) ([]byte, error) {
	file, err := openWatchInput(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var contents bytes.Buffer
	if size < math.MaxInt-bytes.MinRead {
		contents.Grow(int(size) + bytes.MinRead)
	}
	_, err = contents.ReadFrom(file)
	return contents.Bytes(), err
}

// path is a clean absolute path from watch planning. Keep extended and device
// paths intact, and opt clean drive or UNC paths into Win32 long-path handling.
func openWatchInput(path string) (*os.File, error) {
	name := path
	if len(name) >= 248 {
		switch {
		case strings.HasPrefix(name, `\\?\`), strings.HasPrefix(name, `\\.\`), strings.HasPrefix(name, `\??\`):
		case strings.HasPrefix(name, `\\`):
			name = `\\?\UNC\` + name[2:]
		default:
			name = `\\?\` + name
		}
	}
	wide, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	// Snapshot reads must not deny deletion or rename while the editor saves.
	handle, err := syscall.CreateFile(wide, syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(handle), path), nil
}
