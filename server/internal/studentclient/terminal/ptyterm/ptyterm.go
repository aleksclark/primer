// Package ptyterm wraps creack/pty with a bounded scrollback for Primer student sessions.
//
// The TUI never holds PTY or process handles; the broker/engine owns Terminal
// instances and exposes only Write/Resize/ScreenContent over IPC.
package ptyterm

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

// DefaultScrollback is the maximum number of bytes retained for ScreenContent.
const DefaultScrollback = 64 * 1024

// DefaultSize is used when Start is called without an explicit size.
var DefaultSize = pty.Winsize{Rows: 24, Cols: 80}

// Options configure a PTY-backed shell.
type Options struct {
	// Cmd is the process to start under a PTY (required).
	Cmd *exec.Cmd
	// Rows/Cols are the initial window size (defaults 24x80).
	Rows, Cols uint16
	// Scrollback caps retained output bytes (default DefaultScrollback).
	Scrollback int
}

// Terminal is a live PTY session with bounded output capture.
type Terminal struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	ptmx   *os.File
	buf    []byte
	max    int
	rows   uint16
	cols   uint16
	closed bool
	done   chan struct{}
	err    error
}

// Start launches cmd under a PTY and begins reading output into a ring buffer.
func Start(opts Options) (*Terminal, error) {
	if opts.Cmd == nil {
		return nil, fmt.Errorf("ptyterm: Cmd is required")
	}
	rows := opts.Rows
	cols := opts.Cols
	if rows == 0 {
		rows = DefaultSize.Rows
	}
	if cols == 0 {
		cols = DefaultSize.Cols
	}
	max := opts.Scrollback
	if max <= 0 {
		max = DefaultScrollback
	}

	ws := &pty.Winsize{Rows: rows, Cols: cols}
	ptmx, err := pty.StartWithSize(opts.Cmd, ws)
	if err != nil {
		return nil, fmt.Errorf("ptyterm start: %w", err)
	}

	t := &Terminal{
		cmd:  opts.Cmd,
		ptmx: ptmx,
		max:  max,
		rows: rows,
		cols: cols,
		done: make(chan struct{}),
	}
	go t.readLoop()
	return t, nil
}

// StartShell is a convenience that runs shell (default "sh") as a login-ish interactive shell
// with Dir set to workDir.
func StartShell(workDir, shell string, rows, cols uint16) (*Terminal, error) {
	if shell == "" {
		shell = "sh"
	}
	// Prefer bash -i when available for a friendlier prompt; fall back to sh -i.
	var cmd *exec.Cmd
	switch shell {
	case "bash":
		cmd = exec.Command("bash", "--noprofile", "--norc", "-i")
	default:
		// POSIX sh often ignores -i; still start as interactive when possible.
		cmd = exec.Command(shell, "-i")
	}
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"PS1=$ ",
		"HOME="+workDir,
	)
	return Start(Options{Cmd: cmd, Rows: rows, Cols: cols})
}

func (t *Terminal) readLoop() {
	defer close(t.done)
	buf := make([]byte, 4096)
	for {
		n, err := t.ptmx.Read(buf)
		if n > 0 {
			t.mu.Lock()
			t.buf = append(t.buf, buf[:n]...)
			if len(t.buf) > t.max {
				// Drop oldest bytes.
				t.buf = append([]byte(nil), t.buf[len(t.buf)-t.max:]...)
			}
			t.mu.Unlock()
		}
		if err != nil {
			t.mu.Lock()
			if !t.closed {
				if err != io.EOF {
					t.err = err
				}
			}
			t.mu.Unlock()
			return
		}
	}
}

// Write sends raw bytes to the PTY (keystrokes).
func (t *Terminal) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.ptmx == nil {
		return 0, fmt.Errorf("ptyterm: closed")
	}
	return t.ptmx.Write(p)
}

// WriteString is Write for strings.
func (t *Terminal) WriteString(s string) (int, error) {
	return t.Write([]byte(s))
}

// Resize updates the PTY window size.
func (t *Terminal) Resize(rows, cols uint16) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.ptmx == nil {
		return fmt.Errorf("ptyterm: closed")
	}
	if rows == 0 {
		rows = 24
	}
	if cols == 0 {
		cols = 80
	}
	if err := pty.Setsize(t.ptmx, &pty.Winsize{Rows: rows, Cols: cols}); err != nil {
		return err
	}
	t.rows = rows
	t.cols = cols
	return nil
}

// Size returns the last applied window size.
func (t *Terminal) Size() (rows, cols uint16) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.rows, t.cols
}

// ScreenContent returns the bounded scrollback as a string suitable for TUI display.
// ANSI sequences are left intact so the TUI can pass them through or strip later.
func (t *Terminal) ScreenContent() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// ScreenPlain returns scrollback with common ANSI CSI sequences stripped for tests/matching.
func (t *Terminal) ScreenPlain() string {
	return stripANSI(t.ScreenContent())
}

// Alive reports whether the PTY is still open.
func (t *Terminal) Alive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return !t.closed
}

// Wait blocks until the read loop exits (process exit or Close).
func (t *Terminal) Wait() error {
	<-t.done
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}

// Close terminates the PTY and process.
func (t *Terminal) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	ptmx := t.ptmx
	cmd := t.cmd
	t.mu.Unlock()

	if ptmx != nil {
		_ = ptmx.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() {
			_, _ = cmd.Process.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}
	// Wait briefly for reader to finish.
	select {
	case <-t.done:
	case <-time.After(300 * time.Millisecond):
	}
	return nil
}

func stripANSI(s string) string {
	// Minimal CSI stripper: ESC [ ... letter
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			// ESC
			if i+1 < len(s) && s[i+1] == '[' {
				i += 2
				for i < len(s) {
					c := s[i]
					if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
						break
					}
					i++
				}
				continue
			}
			// OSC or other: skip until BEL or ST
			if i+1 < len(s) && s[i+1] == ']' {
				i += 2
				for i < len(s) {
					if s[i] == 0x07 {
						break
					}
					if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
						i++
						break
					}
					i++
				}
				continue
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
