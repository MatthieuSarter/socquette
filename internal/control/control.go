// Package control installs, starts and stops the server: as an OS service (Windows service, systemd unit) through
// kardianos/service, or as a detached background process.
package control

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kardianos/service"

	"github.com/MatthieuSarter/socquette/internal/config"
	"github.com/MatthieuSarter/socquette/internal/server"
)

// serviceName is the OS service name.
const serviceName = "socquette"

// pollInterval is how often the server checks its configuration file.
const pollInterval = 2 * time.Second

// Action is a privileged service operation.
type Action string

// The service operations.
const (
	Install   Action = "install"
	Uninstall Action = "uninstall"
	Start     Action = "start"
	Stop      Action = "stop"
)

// ErrCancelled is returned when the user declines the elevation prompt.
var ErrCancelled = errors.New("elevation cancelled")

// ServiceConfig is the service definition: it runs `socquette --serve` with SOCQUETTE_CONF pointing at confDir, so
// the service reads the file the UI edits whatever its working directory.
func ServiceConfig(confDir string) *service.Config {
	return &service.Config{
		Name:        serviceName,
		DisplayName: "Socquette",
		Description: "Authenticated SOCKS5 and HTTP proxy.",
		Arguments:   []string{"--serve"},
		EnvVars:     map[string]string{"SOCQUETTE_CONF": confDir},
		Option:      service.KeyValue{"StartType": "automatic", "OnFailure": "restart"},
	}
}

type program struct {
	cancel context.CancelFunc
	done   chan struct{}
	run    func(ctx context.Context) error
}

func (p *program) Start(service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel, p.done = cancel, make(chan struct{})
	go func() {
		defer close(p.done)
		_ = p.run(ctx)
	}()
	return nil
}

func (p *program) Stop(service.Service) error {
	p.cancel()
	<-p.done
	return nil
}

// RunServe runs the server until the service manager (or Ctrl+C, when interactive) stops it.
func RunServe(paths config.Paths) error {
	var stderr io.Writer
	if service.Interactive() {
		stderr = os.Stderr
	}
	log, closer, err := server.NewLogger(paths.Log, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()
	p := &program{run: func(ctx context.Context) error {
		return server.Run(ctx, paths, log, pollInterval)
	}}
	s, err := service.New(p, ServiceConfig(paths.Dir))
	if err != nil {
		return fmt.Errorf("service: %w", err)
	}
	if err := s.Run(); err != nil {
		log.Error("server failed", "error", err)
		return err
	}
	return nil
}

// Do performs a service action in this process, which must have admin (root) rights. Install also starts the
// service.
func Do(a Action, paths config.Paths) error {
	dir, err := filepath.Abs(paths.Dir)
	if err != nil {
		return err
	}
	s, err := service.New(&program{}, ServiceConfig(dir))
	if err != nil {
		return fmt.Errorf("service: %w", err)
	}
	switch a {
	case Install:
		// A background server would fight the service over the ports.
		if err := StopBackground(paths); err != nil {
			return err
		}
	case Uninstall:
		_ = s.Stop()
	case Start, Stop:
	default:
		return fmt.Errorf("unknown service action %q", a)
	}
	if err := service.Control(s, string(a)); err != nil {
		return fmt.Errorf("%s service: %w", a, err)
	}
	if a == Install {
		// Start right away: one elevation prompt for "install and run".
		if err := s.Start(); err != nil {
			return fmt.Errorf("start service: %w", err)
		}
	}
	return nil
}

// Request performs a service action from the unprivileged UI, by running `socquette --service <a> --conf <dir>`
// with elevated rights (UAC prompt on Windows) and waiting for it.
func Request(a Action, paths config.Paths) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir, err := filepath.Abs(paths.Dir)
	if err != nil {
		return err
	}
	// The elevated process doesn't inherit this environment, so the config directory goes in the arguments.
	return runElevated(exe, []string{"--service", string(a), "--conf", dir})
}

// State is what the UI shows about the server.
type State struct {
	Installed  bool // the service exists
	Running    bool // the service is running
	Background bool // a background server process is running
}

// Status reports the service and background process state.
func Status(paths config.Paths) State {
	installed, running := serviceStatus()
	_, bg := backgroundPID(paths)
	return State{Installed: installed, Running: running, Background: bg}
}

// ParsePID parses the content of the PID file.
func ParsePID(data []byte) (int, error) {
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid PID file content %q", data)
	}
	return pid, nil
}

// backgroundPID returns the PID of the background server when it is running (and is really ours).
func backgroundPID(paths config.Paths) (int, bool) {
	data, err := os.ReadFile(paths.PID)
	if err != nil {
		return 0, false
	}
	pid, err := ParsePID(data)
	if err != nil {
		return 0, false
	}
	return pid, ownProcess(pid)
}

// StartBackground starts a detached `socquette --serve` process that outlives the UI, and records its PID.
func StartBackground(paths config.Paths) error {
	if _, running := backgroundPID(paths); running {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--serve") //nolint:gosec // Runs this very executable.
	cmd.Env = append(os.Environ(), "SOCQUETTE_CONF="+paths.Dir)
	cmd.Dir = paths.Dir
	cmd.SysProcAttr = detachedAttr()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return os.WriteFile(paths.PID, []byte(strconv.Itoa(pid)+"\n"), 0o600)
}

// StopBackground kills the background server, if any, and removes its PID file (stale or not).
func StopBackground(paths config.Paths) error {
	pid, running := backgroundPID(paths)
	if running {
		p, err := os.FindProcess(pid)
		if err == nil {
			err = p.Kill()
		}
		if err != nil {
			return fmt.Errorf("stop server: %w", err)
		}
	}
	if err := os.Remove(paths.PID); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
