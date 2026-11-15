//go:build !windows

package control

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/kardianos/service"
)

// runElevated runs exe with args directly: on Unix the UI must already run as root to manage the service.
func runElevated(exe string, args []string) error {
	if os.Geteuid() != 0 {
		return errors.New("managing the service needs root: run socquette as root")
	}
	if out, err := exec.Command(exe, args...).CombinedOutput(); err != nil { //nolint:gosec // This very executable.
		return fmt.Errorf("%s failed: %w: %s", args[1], err, out)
	}
	return nil
}

func serviceStatus() (installed, running bool) {
	s, err := service.New(&program{}, ServiceConfig(""))
	if err != nil {
		return false, false
	}
	st, err := s.Status()
	if err != nil {
		return false, false
	}
	return true, st == service.StatusRunning
}

func detachedAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }

// ownProcess reports whether pid is a running instance of this very executable, so a PID reused by another program
// after the server died is never taken for (and killed as) the server. Without /proc it can't tell, and says no.
func ownProcess(pid int) bool {
	target, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return false
	}
	exe, err := os.Executable()
	return err == nil && target == exe
}
