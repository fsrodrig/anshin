package qa

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Cmd is one external command the helper runs. Every one is recorded in the run's commands.log, so a report can say
// exactly how its build and route were made.
type Cmd struct {
	Name string
	Args []string
	Dir  string
	Env  []string
}

func (c Cmd) String() string {
	parts := append([]string{c.Name}, c.Args...)
	for i, p := range parts {
		if p == "" || strings.ContainsAny(p, " \t'\"$;&|<>*?") {
			parts[i] = "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
		}
	}
	return strings.Join(parts, " ")
}

// Runner runs external commands. Tests substitute a scripted one, which is how the helper's adb, Gradle and signing
// sequences are tested without a device.
type Runner interface {
	Run(ctx context.Context, c Cmd) (stdout, stderr []byte, err error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, c Cmd) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	return out.Bytes(), errb.Bytes(), err
}

// loggedRunner appends each command and its exit to a log file before returning.
type loggedRunner struct {
	next Runner
	path string
	mu   sync.Mutex
}

func (l *loggedRunner) Run(ctx context.Context, c Cmd) ([]byte, []byte, error) {
	start := time.Now()
	out, errb, err := l.next.Run(ctx, c)
	status := "exit 0"
	if err != nil {
		status = "error: " + err.Error()
	}
	line := fmt.Sprintf("%s\t%s\t%s\t%dms\n", start.UTC().Format(time.RFC3339), c.String(), status, time.Since(start).Milliseconds())
	l.mu.Lock()
	defer l.mu.Unlock()
	if f, ferr := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); ferr == nil {
		f.WriteString(line)
		f.Close()
	}
	return out, errb, err
}

// cmdError keeps a failed command's stderr, which is usually the only useful part of the failure.
func cmdError(c Cmd, stderr []byte, err error) error {
	msg := strings.TrimSpace(string(stderr))
	if len(msg) > 600 {
		msg = "…" + msg[len(msg)-600:]
	}
	if msg == "" {
		return fmt.Errorf("%s: %w", c.Name+" "+strings.Join(c.Args, " "), err)
	}
	return fmt.Errorf("%s: %w: %s", c.Name+" "+strings.Join(c.Args, " "), err, msg)
}
