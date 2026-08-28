package update

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/minio/selfupdate"
)

const downloadBase = "https://github.com/glow-moe/Glow-Live-Desktop/releases/download"

// assetName is the versionless binary published for this OS on every release.
func assetName() string {
	if runtime.GOOS == "windows" {
		return "glow-live-windows-x64.exe"
	}
	return "glow-live-linux-x64"
}

// expectedSHA fetches the release's SHA256SUMS and returns the hex hash for our
// asset, so a download can be verified before it replaces the running binary.
func expectedSHA(tag string) (string, error) {
	url := fmt.Sprintf("%s/%s/SHA256SUMS", downloadBase, tag)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksums: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", err
	}
	name := assetName()
	for _, line := range strings.Split(string(b), "\n") {
		// "<hex>  <filename>"
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == name {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", errors.New("no checksum listed for " + name)
}

// Apply downloads the release binary for `tag`, verifies its SHA256 against the
// release's SHA256SUMS, and swaps the running executable in place. minio's
// selfupdate handles the Windows "can't overwrite a running exe" dance (rename
// aside, write new, roll back on failure). The Windows binary is Authenticode
// signed, so Windows revalidates it on the next launch too.
func Apply(tag string) error {
	if !strings.HasPrefix(tag, "v") {
		return errors.New("bad release tag")
	}
	shaHex, err := expectedSHA(tag)
	if err != nil {
		return err
	}
	sum, err := hex.DecodeString(shaHex)
	if err != nil || len(sum) != sha256.Size {
		return errors.New("bad checksum")
	}
	url := fmt.Sprintf("%s/%s/%s", downloadBase, tag, assetName())
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: %s", resp.Status)
	}
	return selfupdate.Apply(resp.Body, selfupdate.Options{Checksum: sum})
}

// Relaunch starts the freshly-swapped binary and exits this process. The new
// copy gets --relaunch so it waits for this one to release the single-instance
// lock (see cmd/gui/main.go). Does not return on success.
func Relaunch() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if err := exec.Command(exe, "--relaunch").Start(); err != nil {
		return
	}
	os.Exit(0)
}
