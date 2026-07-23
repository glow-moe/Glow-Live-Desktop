// Package discord is a tiny, dependency-free Discord Rich Presence client. It
// speaks the local IPC protocol directly (named pipe on Windows, unix socket
// elsewhere): a length-prefixed frame protocol with a JSON handshake, then
// SET_ACTIVITY frames. large_image/small_image accept external https URLs which
// Discord media-proxies, so no assets are uploaded to the app.
package discord

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
)

// Activity is the Rich Presence payload.
type Activity struct {
	// Type sets the verb: 0 Playing (default), 2 Listening, 3 Watching, 5
	// Competing. The local IPC honours it, so anime can read "Watching …".
	Type       int         `json:"type,omitempty"`
	Details    string      `json:"details,omitempty"`
	State      string      `json:"state,omitempty"`
	Timestamps *Timestamps `json:"timestamps,omitempty"`
	Assets     *Assets     `json:"assets,omitempty"`
	Buttons    []Button    `json:"buttons,omitempty"`
}

type Timestamps struct {
	Start int64 `json:"start,omitempty"`
}

type Assets struct {
	LargeImage string `json:"large_image,omitempty"`
	LargeText  string `json:"large_text,omitempty"`
	SmallImage string `json:"small_image,omitempty"`
	SmallText  string `json:"small_text,omitempty"`
}

type Button struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Client is a connected Discord IPC session. Its writes are serialized so it's
// safe to call SetActivity/Clear from a goroutine.
type Client struct {
	conn  io.ReadWriteCloser
	mu    sync.Mutex
	nonce int
}

// msg is a decoded Discord IPC reply frame.
type msg struct {
	Cmd  string `json:"cmd"`
	Evt  string `json:"evt"`
	Data struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"data"`
}

// Connect opens the first available Discord IPC pipe and handshakes with the
// given application client id. Returns an error if Discord isn't running.
//
// We deliberately do NOT run a background reader: a read blocked on the pipe
// concurrent with a write DEADLOCKS the os.File pipe write on Windows (Discord
// then never receives the activity). Instead every write reads its own reply
// synchronously under the mutex - the request/response the IPC protocol expects.
func Connect(clientID string) (*Client, error) {
	conn, err := dialPipe()
	if err != nil {
		return nil, err
	}
	c := &Client{conn: conn}
	if err := c.writeFrame(0, map[string]any{"v": 1, "client_id": clientID}); err != nil {
		conn.Close()
		return nil, err
	}
	m, err := c.readFrame()
	if err != nil {
		conn.Close()
		return nil, err
	}
	if m.Evt == "ERROR" {
		conn.Close()
		return nil, fmt.Errorf("discord handshake rejected: %s (code %d)", m.Data.Message, m.Data.Code)
	}
	fmt.Println("Discord: handshake ready ✓")
	return c, nil
}

// SetActivity publishes a presence, then drains Discord's reply.
func (c *Client) SetActivity(a Activity) error {
	return c.command(map[string]any{"pid": os.Getpid(), "activity": a})
}

// Clear removes the presence (null activity).
func (c *Client) Clear() error {
	return c.command(map[string]any{"pid": os.Getpid(), "activity": nil})
}

// command writes a SET_ACTIVITY frame and reads its reply, serialized so
// concurrent callers never interleave a write with another's read.
func (c *Client) command(args map[string]any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nonce++
	if err := c.writeFrame(1, map[string]any{
		"cmd":   "SET_ACTIVITY",
		"nonce": strconv.Itoa(c.nonce),
		"args":  args,
	}); err != nil {
		return err
	}
	m, err := c.readFrame()
	if err != nil {
		return err
	}
	if m.Evt == "ERROR" {
		fmt.Printf("Discord rejected the presence: %s (code %d)\n", m.Data.Message, m.Data.Code)
	}
	return nil
}

func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) writeFrame(op uint32, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// Single write (header + body) - some pipe modes split separate writes into
	// separate messages, which would break Discord's framing.
	buf := make([]byte, 8+len(body))
	binary.LittleEndian.PutUint32(buf[0:4], op)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(len(body)))
	copy(buf[8:], body)
	_, err = c.conn.Write(buf)
	return err
}

func (c *Client) readFrame() (msg, error) {
	var hdr [8]byte
	if _, err := io.ReadFull(c.conn, hdr[:]); err != nil {
		return msg{}, err
	}
	n := binary.LittleEndian.Uint32(hdr[4:8])
	body := make([]byte, n)
	if _, err := io.ReadFull(c.conn, body); err != nil {
		return msg{}, err
	}
	var m msg
	_ = json.Unmarshal(body, &m)
	return m, nil
}
