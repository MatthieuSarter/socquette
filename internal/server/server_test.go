package server

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MatthieuSarter/socquette/internal/auth"
	"github.com/MatthieuSarter/socquette/internal/config"
)

const (
	loopback        = "127.0.0.1"
	loopbackAnyPort = "127.0.0.1:0"
)

var testHash, _ = auth.HashPassword("secret")

func TestWanted(t *testing.T) {
	c := config.Defaults()
	if w := Wanted(c); w.HTTP != "" || w.SOCKS != "" {
		t.Errorf("disabled = %+v", w)
	}
	c.HTTP.Enabled, c.SOCKS.Enabled, c.SOCKS.Listen = true, true, loopback
	if w := Wanted(c); w.HTTP != "0.0.0.0:8080" || w.SOCKS != "127.0.0.1:1080" {
		t.Errorf("enabled = %+v", w)
	}
}

type lockedWriter struct {
	mu sync.Mutex
	sb strings.Builder
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sb.Write(p)
}

func (l *lockedWriter) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sb.String()
}

type fixture struct {
	paths config.Paths
	logs  *lockedWriter
	r     *runner
	n     int // saves so far, to give each one a distinct modification time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	paths := config.ResolvePaths(t.TempDir(), "")
	logs := &lockedWriter{}
	r := newRunner(paths, slog.New(slog.NewTextHandler(logs, nil)))
	t.Cleanup(r.close)
	return &fixture{paths: paths, logs: logs, r: r}
}

// write saves a configuration and makes sure its stamp differs from the previous one.
func (f *fixture) write(t *testing.T, c config.Config) {
	t.Helper()
	if err := config.Save(f.paths.Config, c); err != nil {
		t.Fatal(err)
	}
	f.n++
	future := time.Now().Add(time.Duration(f.n) * time.Second)
	_ = os.Chtimes(f.paths.Config, future, future)
}

// freePort returns a port free in both TCP and UDP (Windows reserves some UDP ranges, e.g. for Hyper-V); where UDP
// sockets can't be opened at all, a port free in TCP.
func freePort(t *testing.T) int {
	t.Helper()
	for range 20 {
		ln, err := net.Listen("tcp", loopbackAnyPort)
		if err != nil {
			t.Fatal(err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		pc, err := net.ListenPacket("udp", ln.Addr().String())
		_ = ln.Close()
		if udpUnavailable(err) {
			return port
		}
		if err == nil {
			_ = pc.Close()
			return port
		}
	}
	t.Fatal("no port free in both TCP and UDP")
	return 0
}

// udpUnavailable reports whether err means this platform can't open UDP sockets at all: Wine lacks the
// SIO_UDP_CONNRESET ioctl that Go's Windows runtime sets on every UDP socket.
func udpUnavailable(err error) bool {
	var se *os.SyscallError
	return errors.As(err, &se) && se.Syscall == "wsaioctl"
}

// requireUDP skips the test where UDP sockets can't be opened.
func requireUDP(t *testing.T) {
	t.Helper()
	pc, err := net.ListenPacket("udp", loopbackAnyPort)
	if udpUnavailable(err) {
		t.Skip("UDP sockets unavailable (Wine):", err)
	}
	if err == nil {
		_ = pc.Close()
	}
}

func httpConfig(port int) config.Config {
	c := config.Defaults()
	c.HTTP.Enabled, c.HTTP.Listen, c.HTTP.Port = true, loopback, port
	return c
}

func listening(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// proxyGet fetches target through the HTTP proxy at addr with alice's credentials and returns the status code.
func proxyGet(t *testing.T, addr, target string) int {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	req, _ := http.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("alice:secret")))
	_ = req.WriteProxy(conn)
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func TestTickStartsEnabledProxies(t *testing.T) {
	requireUDP(t)
	f := newFixture(t)
	c := httpConfig(freePort(t))
	c.SOCKS.Enabled, c.SOCKS.Listen, c.SOCKS.Port = true, loopback, freePort(t)
	f.write(t, c)
	f.r.tick()
	if !listening(c.HTTP.Addr()) || !listening(c.SOCKS.Addr()) {
		t.Fatalf("not listening; logs:\n%s", f.logs)
	}
	udp, err := net.ListenPacket("udp", c.SOCKS.Addr())
	if err == nil {
		_ = udp.Close()
		t.Error("SOCKS UDP port not bound")
	}
}

func TestReloadUsers(t *testing.T) {
	f := newFixture(t)
	origin := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { /* 200, empty body */ }))
	defer origin.Close()
	c := httpConfig(freePort(t))
	f.write(t, c)
	f.r.tick()
	if got := proxyGet(t, c.HTTP.Addr(), origin.URL); got != http.StatusProxyAuthRequired {
		t.Fatalf("before = %d", got)
	}
	c.Users = []config.User{{Name: "alice", PasswordHash: testHash, HTTP: true}}
	f.write(t, c)
	f.r.tick()
	if got := proxyGet(t, c.HTTP.Addr(), origin.URL); got != http.StatusOK {
		t.Errorf("after = %d", got)
	}
}

func TestReloadPortChange(t *testing.T) {
	f := newFixture(t)
	c := httpConfig(freePort(t))
	f.write(t, c)
	f.r.tick()
	old := c.HTTP.Addr()
	c.HTTP.Port = freePort(t)
	f.write(t, c)
	f.r.tick()
	if !listening(c.HTTP.Addr()) || listening(old) {
		t.Errorf("new=%v old=%v", listening(c.HTTP.Addr()), listening(old))
	}
}

func TestReloadSamePortNewAddress(t *testing.T) {
	f := newFixture(t)
	c := httpConfig(freePort(t))
	c.HTTP.Listen = "0.0.0.0"
	f.write(t, c)
	f.r.tick()
	c.HTTP.Listen = loopback
	f.write(t, c)
	f.r.tick()
	if got := f.r.bound.HTTP; got != c.HTTP.Addr() {
		t.Errorf("bound = %q, want %q; logs:\n%s", got, c.HTTP.Addr(), f.logs)
	}
	if !listening(c.HTTP.Addr()) {
		t.Error("not listening")
	}
}

func TestBindFailureKeepsOldListener(t *testing.T) {
	f := newFixture(t)
	c := httpConfig(freePort(t))
	f.write(t, c)
	f.r.tick()
	busy, _ := net.Listen("tcp", loopbackAnyPort)
	defer func() { _ = busy.Close() }()
	old := c.HTTP.Addr()
	c.HTTP.Port = busy.Addr().(*net.TCPAddr).Port
	f.write(t, c)
	f.r.tick()
	f.r.tick()
	if f.r.bound.HTTP != old || !listening(old) {
		t.Errorf("bound = %q, want %q", f.r.bound.HTTP, old)
	}
	if n := strings.Count(f.logs.String(), "cannot listen"); n != 1 {
		t.Errorf("bind error logged %d times, want 1:\n%s", n, f.logs)
	}
	_ = busy.Close()
	f.r.tick()
	if f.r.bound.HTTP != c.HTTP.Addr() {
		t.Errorf("not retried: bound = %q", f.r.bound.HTTP)
	}
}

func TestReloadInvalidKeepsState(t *testing.T) {
	f := newFixture(t)
	c := httpConfig(freePort(t))
	f.write(t, c)
	f.r.tick()
	if err := os.WriteFile(f.paths.Config, []byte("[http\ngarbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		f.r.tick()
	}
	if !listening(c.HTTP.Addr()) {
		t.Error("listener lost after an invalid file")
	}
	if n := strings.Count(f.logs.String(), "invalid configuration"); n != 1 {
		t.Errorf("error logged %d times, want 1:\n%s", n, f.logs)
	}
}

func TestDisableClosesListener(t *testing.T) {
	f := newFixture(t)
	c := httpConfig(freePort(t))
	f.write(t, c)
	f.r.tick()
	c.HTTP.Enabled = false
	f.write(t, c)
	f.r.tick()
	if listening(c.HTTP.Addr()) {
		t.Error("still listening")
	}
}

func TestMissingFileThenCreated(t *testing.T) {
	f := newFixture(t)
	f.r.tick()
	if f.r.bound.HTTP != "" || f.r.bound.SOCKS != "" {
		t.Fatalf("bound = %+v", f.r.bound)
	}
	c := httpConfig(freePort(t))
	f.write(t, c)
	f.r.tick()
	if !listening(c.HTTP.Addr()) {
		t.Error("not listening after the file appeared")
	}
}

func TestRunCancelClosesAll(t *testing.T) {
	dir := t.TempDir()
	paths := config.ResolvePaths(dir, "")
	c := httpConfig(freePort(t))
	if err := config.Save(paths.Config, c); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- Run(ctx, paths, slog.New(slog.DiscardHandler), 20*time.Millisecond) }()
	deadline := time.Now().Add(2 * time.Second)
	for !listening(c.HTTP.Addr()) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if listening(c.HTTP.Addr()) {
		t.Error("still listening after cancel")
	}
}

func TestNewLogger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "socquette.log")
	var stderr strings.Builder
	log, closer, err := NewLogger(path, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	log.Info("hello", "k", "v")
	_ = closer.Close()
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "k=v") || !strings.Contains(stderr.String(), "k=v") {
		t.Errorf("file=%q stderr=%q", data, stderr.String())
	}
	if _, closer, err := NewLogger(path, nil); err != nil {
		t.Error(err)
	} else {
		_ = closer.Close()
	}
}

func TestSOCKSUDPPortBusy(t *testing.T) {
	requireUDP(t)
	f := newFixture(t)
	port := freePort(t)
	busy, err := net.ListenPacket("udp", net.JoinHostPort(loopback, strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = busy.Close() }()
	c := config.Defaults()
	c.SOCKS.Enabled, c.SOCKS.Listen, c.SOCKS.Port = true, loopback, port
	f.write(t, c)
	f.r.tick()
	if f.r.bound.SOCKS != "" || listening(c.SOCKS.Addr()) {
		t.Errorf("SOCKS half-bound: bound=%q", f.r.bound.SOCKS)
	}
}
