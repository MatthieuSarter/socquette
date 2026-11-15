package control

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/MatthieuSarter/socquette/internal/config"
)

// TestStopBackgroundSparesOtherProcess: a PID file left behind by a dead server may name a process that reused the
// PID; stopping must not kill it.
func TestStopBackgroundSparesOtherProcess(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if runtime.GOOS == "windows" {
		cmd = exec.Command("ping", "-n", "30", "127.0.0.1")
	}
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start a helper process:", err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() { _ = cmd.Process.Kill(); <-done })

	paths := config.ResolvePaths(t.TempDir(), "")
	if err := os.WriteFile(paths.PID, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
	if Status(paths).Background {
		t.Error("an unrelated process is reported as the background server")
	}
	if err := StopBackground(paths); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		t.Fatal("an unrelated process was killed")
	case <-time.After(500 * time.Millisecond):
	}
	if _, err := os.Stat(paths.PID); !os.IsNotExist(err) {
		t.Errorf("stale PID file kept: %v", err)
	}
}
