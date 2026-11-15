//go:build !windows

package control

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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

func firewallAvailable() error {
	if _, err := exec.LookPath("ufw"); err != nil {
		return ErrNoFirewall
	}
	return nil
}

// applyFirewall replaces the ufw rules commented "Socquette" with rules.
func applyFirewall(rules []FirewallRule) error {
	if err := firewallAvailable(); err != nil {
		return err
	}
	status, err := ufw("status", "numbered")
	if err != nil {
		return err
	}
	for _, n := range UfwRuleNumbers(status) {
		if _, err := ufw("--force", "delete", strconv.Itoa(n)); err != nil {
			return err
		}
	}
	for _, r := range rules {
		if _, err := ufw("allow", fmt.Sprintf("%d/%s", r.Port, r.Proto), "comment", firewallRuleName); err != nil {
			return err
		}
	}
	return nil
}

func ufw(args ...string) (string, error) {
	out, err := exec.Command("ufw", args...).CombinedOutput() //nolint:gosec // Fixed ufw subcommands, built here.
	if err != nil {
		return "", fmt.Errorf("ufw %v: %w: %s", args, err, out)
	}
	return string(out), nil
}
