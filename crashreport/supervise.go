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
	// Relaunch starts the updated program when the child exits with
	// RestartCode; nil uses Relaunch(Exe).
	Relaunch func() error
	// RetryWithin, when set, starts a child that crashed within that time
	// once more, after BeforeRetry, before the crash is reported. main sets
	// it for the first start after an update (ADR 0009).
	RetryWithin time.Duration
	BeforeRetry func()
}

// Supervise runs the child until it exits, shows a dialog when it stopped
// because of an error, and returns its exit code.
func Supervise(opts Options) int {
	show := func(message string) {
		record(opts.CrashFile, message)
		opts.Show(Title, message)
	}
	run, err := runChild(opts, show)
	if err != nil {
		show(Message("The window couldn't start: "+err.Error(), opts.LogDir))
		return 1
	}
	// Right after an update the old window's web view can still hold its
	// data folder for a moment (Windows), so a window that fails early is
	// started once more before it counts as a crash.
	if opts.RetryWithin > 0 && run.failed(opts) && run.ran < opts.RetryWithin {
		if opts.BeforeRetry != nil {
			opts.BeforeRetry()
		}
		if run, err = runChild(opts, show); err != nil {
			show(Message("The window couldn't start: "+err.Error(), opts.LogDir))
			return 1
		}
	}
	if run.code == RestartCode && !run.stuck {
		// The window installed an update: start the new version, which
		// waits for this supervisor to exit. Not a crash.
		relaunch := opts.Relaunch
		if relaunch == nil {
			relaunch = func() error { return Relaunch(opts.Exe) }
		}
		if err := relaunch(); err != nil {
			show(UpdatedNotStartedMessage(err))
			return 1
		}
		return 0
	}
	if msg, ok := Report(run.output, run.code, run.quiet, opts.LogDir); ok {
		show(msg)
	}
	if run.code < 0 {
		return 1
	}
	return run.code
}

// childRun is how one run of the child ended.
type childRun struct {
	code   int
	output string
	// quiet marks a stop the player asked for, or a stuck window already
	// reported; stuck a window that never started.
	quiet, stuck bool
	// ran is how long the child ran.
	ran time.Duration
}

// failed reports whether the run ended in a crash that would get a dialog.
func (r childRun) failed(opts Options) bool {
	if r.code == RestartCode || r.stuck {
		return false
	}
	_, ok := Report(r.output, r.code, r.quiet, opts.LogDir)
	return ok
}

// runChild starts the child and waits for it to exit.
func runChild(opts Options, show func(string)) (childRun, error) {
	cmd := exec.Command(opts.Exe, opts.Args...)
	cmd.Env = append(os.Environ(), ChildEnv+"=1")
	out := &tail{console: opts.Console, started: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = out, out
	began := time.Now()
	if err := cmd.Start(); err != nil {
		return childRun{}, err
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
	return childRun{
		code:   cmd.ProcessState.ExitCode(),
		output: out.String(),
		quiet:  stuck || stoppedOnRequest(cmd.ProcessState),
		stuck:  stuck,
		ran:    time.Since(began),
	}, nil
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
	defer func() { _ = f.Close() }()
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
