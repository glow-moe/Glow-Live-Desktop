//go:build windows

package discord

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
)

// dialPipe opens the first available Discord IPC named pipe using CreateFile
// (the reliable way on Windows; os.OpenFile on a pipe is flaky).
func dialPipe() (io.ReadWriteCloser, error) {
	for i := 0; i < 10; i++ {
		path := fmt.Sprintf(`\\.\pipe\discord-ipc-%d`, i)
		p, err := syscall.UTF16PtrFromString(path)
		if err != nil {
			continue
		}
		h, err := syscall.CreateFile(
			p,
			syscall.GENERIC_READ|syscall.GENERIC_WRITE,
			0, nil,
			syscall.OPEN_EXISTING,
			0, 0,
		)
		if err == nil {
			return os.NewFile(uintptr(h), path), nil
		}
	}
	return nil, errors.New("discord not running")
}
