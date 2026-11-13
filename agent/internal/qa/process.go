package qa

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ProcessControl starts and stops the host QA Agent. A process is identified by its PID and its kernel start time
// together, so a stop never signals an unrelated process that reused the PID.
type ProcessControl interface {
	Start(c Cmd, stdoutPath, stderrPath string) (pid int, startTime string, err error)
	Alive(pid int, startTime string) bool
	Stop(pid int, startTime string) error
}

type OSProcesses struct{}

func (OSProcesses) Start(c Cmd, stdoutPath, stderrPath string) (int, string, error) {
	out, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, "", err
	}
	defer out.Close()
	errf, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, "", err
	}
	defer errf.Close()
	cmd := exec.Command(c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.Stdout, cmd.Stderr = out, errf
	// Its own process group, so the helper exiting doesn't take the QA Agent with it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return 0, "", err
	}
	pid := cmd.Process.Pid
	st, err := procStartTime(pid)
	cmd.Process.Release()
	return pid, st, err
}

func (OSProcesses) Alive(pid int, startTime string) bool {
	st, err := procStartTime(pid)
	return err == nil && st == startTime && !procZombie(pid)
}

func (p OSProcesses) Stop(pid int, startTime string) error {
	if !p.Alive(pid, startTime) {
		return nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	for range 50 {
		if !p.Alive(pid, startTime) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

// procStartTime is field 22 of /proc/<pid>/stat, read after the parenthesized command name, which may hold spaces.
func procStartTime(pid int) (string, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return "", err
	}
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return "", fmt.Errorf("unreadable /proc/%d/stat", pid)
	}
	f := strings.Fields(s[i+1:])
	if len(f) < 20 {
		return "", fmt.Errorf("short /proc/%d/stat", pid)
	}
	return f[19], nil
}

func procZombie(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true
	}
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	return i >= 0 && strings.HasPrefix(strings.TrimSpace(s[i+1:]), "Z")
}
