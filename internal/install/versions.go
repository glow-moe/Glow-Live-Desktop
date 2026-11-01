package install

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/glow-moe/glow-collector/internal/update"
)

// Each release lives in its own folder next to the launcher, the way Discord
// keeps app-1.0.x folders: an update is a new folder, never a file written
// over a running one, so the previous build is still there when the new one
// gets removed by an antivirus or dies on start.
//
//	glow L!VE\
//	  glow L!VE.exe        the launcher (same binary, see Launch)
//	  app-26.11\glow L!VE.exe
//	  app-26.12\glow L!VE.exe, healthy   (ran a minute: it works here)
//	  app-26.13\glow L!VE.exe, bad       (died before that: skipped)

const (
	exeName     = "glow L!VE.exe"
	appPrefix   = "app-"
	healthyMark = "healthy"
	badMark     = "bad"
)

// versionDir is the folder of release `ver` ("v26.11" -> app-26.11) under dir.
func versionDir(dir, ver string) string {
	return filepath.Join(dir, appPrefix+strings.TrimPrefix(ver, "v"))
}

func versionExe(dir, ver string) string { return filepath.Join(versionDir(dir, ver), exeName) }

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isBad(dir, ver string) bool { return exists(filepath.Join(versionDir(dir, ver), badMark)) }

func isHealthy(dir, ver string) bool {
	return exists(filepath.Join(versionDir(dir, ver), healthyMark))
}

func mark(dir, ver, name string) {
	_ = os.WriteFile(filepath.Join(versionDir(dir, ver), name), nil, 0o644)
}

// versions lists the release folders under dir that still hold their exe,
// newest first ("v26.12", "v26.11").
func versions(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), appPrefix) {
			continue
		}
		ver := "v" + strings.TrimPrefix(e.Name(), appPrefix)
		if update.IsRelease(ver) && exists(versionExe(dir, ver)) {
			out = append(out, ver)
		}
	}
	sort.Slice(out, func(i, j int) bool { return update.Compare(out[i], out[j]) > 0 })
	return out
}

// pick is the build to start: the newest one not marked bad.
func pick(dir string) string {
	for _, v := range versions(dir) {
		if !isBad(dir, v) {
			return v
		}
	}
	return ""
}

// prune keeps the running build and the newest other healthy one (the one to
// fall back to) and removes the rest. A folder still in use just stays.
func prune(dir, running string) {
	entries, _ := os.ReadDir(dir)
	keepPrev := ""
	for _, v := range versions(dir) {
		if v != running && isHealthy(dir, v) && !isBad(dir, v) {
			keepPrev = v
			break
		}
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), appPrefix) {
			continue
		}
		ver := "v" + strings.TrimPrefix(e.Name(), appPrefix)
		if ver == running || ver == keepPrev {
			continue
		}
		// A bad build's folder stays as long as it is the newest, so the
		// updater knows not to fetch it again.
		if isBad(dir, ver) && update.Compare(ver, running) > 0 {
			continue
		}
		_ = os.RemoveAll(filepath.Join(dir, e.Name()))
	}
}

// copyExe writes src over dst through a temp file, so a half-written copy
// never replaces a working one.
func copyExe(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return replace(tmp, dst)
}

// replace moves a finished file onto dst. Windows can't overwrite an exe that
// is running (the launcher, waiting on the app) but can rename it, so the old
// one steps aside as dst.old and is removed on a later start.
func replace(tmp, dst string) error {
	if err := os.Rename(tmp, dst); err == nil {
		return nil
	}
	_ = os.Remove(dst + ".old")
	if err := os.Rename(dst, dst+".old"); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Rename(dst+".old", dst)
		os.Remove(tmp)
		return err
	}
	return nil
}
