package crashreport

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ChildEnv marks the window process a supervisor started.
const ChildEnv = "WF_SUPERVISED"

// keepBytes is how much of the child's output is kept for the report.
const keepBytes = 256 << 10

// Supervised reports whether this process is the window a supervisor
// started.
func Supervised() bool { return os.Getenv(ChildEnv) != "" }

// Options configure Supervise.
type Options struct {
	// Exe and Args start the child: this program again.
	Exe  string
	Args []string
	// Console receives the child's output as it runs; write errors are
	// ignored (a Windows GUI program has no console).
	Console io.Writer
	// LogDir is named in the dialog, or "" when file logging is off.
	LogDir string
	// Show displays the dialog.
	Show func(title, message string)
	// CrashFile, when set, also receives every dialog's text, so a dialog
	// closed unread still leaves a record.
	CrashFile string
	// StartupTimeout is how long the window may take to start (to log
	// StartupMarker) before the player is told and the stuck child closed;
	// 0 waits for ever.
	StartupTimeout time.Duration
}

// Supervise runs the child until it exits, shows a dialog when it stopped
// because of an error, and returns its exit code.
func Supervise(opts Options) int {
	cmd := exec.Command(opts.Exe, opts.Args...)
	cmd.Env = append(os.Environ(), ChildEnv+"=1")
	out := &tail{console: opts.Console, started: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = out, out
	show := func(message string) {
		record(opts.CrashFile, message)
		opts.Show(Title, message)
	}
	if err := cmd.Start(); err != nil {
		show(Message("The window couldn't start: "+err.Error(), opts.LogDir))
		return 1
	}
	stop := forwardSignals(cmd.Process)
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	stuck := false
	if opts.StartupTimeout > 0 {
		select {
		case <-exited:
		case <-out.started:
		case <-time.After(opts.StartupTimeout):
			// The window never started, usually because the web view
			// didn't: say so, then close the child so it doesn't hold the
			// one-instance lock and turn away every later launch.
			show(NotOpenedMessage(opts.LogDir))
			select {
			case <-out.started:
			default:
				stuck = true
				_ = cmd.Process.Kill()
			}
		}
	}
	<-exited
	stop()
	code := cmd.ProcessState.ExitCode()
	if msg, ok := Report(out.String(), code, stuck || stoppedOnRequest(cmd.ProcessState), opts.LogDir); ok {
		show(msg)
	}
	if code < 0 {
		code = 1
	}
	return code
}

// record appends a dialog's text to the crash file, if there is one.
func record(path, message string) {
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s: %s\n", time.Now().Format(time.RFC3339), Title, strings.ReplaceAll(message, "\n", " "))
}

// tail passes output to the console, keeps its last keepBytes, and closes
// started once StartupMarker has been written.
type tail struct {
	mu      sync.Mutex
	console io.Writer
	buf     []byte
	started chan struct{}
	seen    bool
}

func (t *tail) Write(p []byte) (int, error) {
	if t.console != nil {
		_, _ = t.console.Write(p)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	from := max(len(t.buf)-len(StartupMarker), 0)
	t.buf = append(t.buf, p...)
	if !t.seen && bytes.Contains(t.buf[from:], []byte(StartupMarker)) {
		t.seen = true
		close(t.started)
	}
	if over := len(t.buf) - keepBytes; over > 0 {
		t.buf = append(t.buf[:0], t.buf[over:]...)
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
