// Package logtail follows a game's growing log file (Roblox, Fortnite): the
// first look reads only its last few MB (these logs grow to tens of MB), after
// that only the whole lines added since, so a tick every second or so costs a
// few KB, not a re-read.
package logtail

import (
	"bytes"
	"io"
	"os"
)

// Follower remembers how far one log has been read.
type Follower struct {
	path   string
	offset int64
}

const (
	firstLook = 4 << 20
	maxStep   = 8 << 20
)

// Read hands feed the whole lines written to path since the last call. fresh
// is true when it started over (another file, or the same one cut short: a new
// session), so the caller drops what it knew. A half-written last line waits
// for the next call.
func (f *Follower) Read(path string, feed func(lines io.Reader, fresh bool)) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	st, err := fh.Stat()
	if err != nil {
		return err
	}
	size := st.Size()
	fresh := false
	if path != f.path || size < f.offset {
		*f = Follower{path: path, offset: max(0, size-firstLook)}
		fresh = true
	}
	if size-f.offset > maxStep {
		f.offset = size - maxStep
	}
	if size <= f.offset {
		if fresh {
			feed(bytes.NewReader(nil), true)
		}
		return nil
	}
	buf := make([]byte, size-f.offset)
	n, err := fh.ReadAt(buf, f.offset)
	if err != nil && err != io.EOF {
		return err
	}
	buf = buf[:n]
	end := bytes.LastIndexByte(buf, '\n')
	if end < 0 {
		if fresh {
			feed(bytes.NewReader(nil), true)
		}
		return nil
	}
	feed(bytes.NewReader(buf[:end+1]), fresh)
	f.offset += int64(end + 1)
	return nil
}
