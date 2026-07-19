//go:build !windows

package discord

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
)

// dialPipe opens the first available Discord IPC unix socket.
func dialPipe() (io.ReadWriteCloser, error) {
	for i := 0; i < 10; i++ {
		c, err := net.Dial("unix", filepath.Join(runtimeDir(), fmt.Sprintf("discord-ipc-%d", i)))
		if err == nil {
			return c, nil
		}
	}
	return nil, errors.New("discord not running")
}

func runtimeDir() string {
	for _, k := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "/tmp"
}
