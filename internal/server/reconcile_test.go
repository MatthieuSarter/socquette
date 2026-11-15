package server

import (
	"errors"
	"log/slog"
	"testing"
)

const (
	anyAddr        = "0.0.0.0:1080"
	unbindableAddr = "192.0.2.1:1080" // never binds in the fake
)

// fakeBinder simulates a port where a more specific address can't be bound while another address holds the port.
type fakeBinder struct {
	held    map[string]bool // addresses currently bound
	refuse  map[string]bool // addresses that never bind
	binds   []string
	release int
}

type fakeBinding struct {
	f    *fakeBinder
	addr string
}

func (b *fakeBinding) Close() error { delete(b.f.held, b.addr); b.f.release++; return nil }
func (b *fakeBinding) Shutdown()    { _ = b.Close() }

func (f *fakeBinder) bind(addr string) (binding, error) {
	f.binds = append(f.binds, addr)
	if f.refuse[addr] || len(f.held) > 0 {
		return nil, errors.New("address in use")
	}
	f.held[addr] = true
	return &fakeBinding{f: f, addr: addr}, nil
}

func fakeSlot(refuse ...string) (*runner, *slot, *fakeBinder) {
	f := &fakeBinder{held: map[string]bool{}, refuse: map[string]bool{}}
	for _, a := range refuse {
		f.refuse[a] = true
	}
	r := &runner{log: slog.New(slog.DiscardHandler)}
	return r, &slot{name: "test", bind: f.bind}, f
}

func TestReconcileSamePortFallback(t *testing.T) {
	r, s, _ := fakeSlot()
	r.reconcile(s, anyAddr)
	r.reconcile(s, "127.0.0.1:1080")
	if s.addr != "127.0.0.1:1080" {
		t.Errorf("addr = %q", s.addr)
	}
}

func TestReconcileRestoresOld(t *testing.T) {
	r, s, f := fakeSlot(unbindableAddr)
	r.reconcile(s, anyAddr)
	r.reconcile(s, unbindableAddr)
	if s.addr != anyAddr || !f.held[anyAddr] {
		t.Errorf("addr = %q, held = %v", s.addr, f.held)
	}
}

func TestReconcileNoFlap(t *testing.T) {
	r, s, f := fakeSlot(unbindableAddr)
	r.reconcile(s, anyAddr)
	r.reconcile(s, unbindableAddr)
	before := f.release
	r.reconcile(s, unbindableAddr)
	r.reconcile(s, unbindableAddr)
	if f.release != before || s.addr != anyAddr {
		t.Errorf("old listener released %d more times; addr = %q", f.release-before, s.addr)
	}
}

func TestReconcileOtherPortKeepsOld(t *testing.T) {
	r, s, f := fakeSlot()
	r.reconcile(s, anyAddr)
	r.reconcile(s, "0.0.0.0:2080") // the fake refuses while 1080 is held: closing 1080 must not be tried
	if s.addr != anyAddr || f.release != 0 {
		t.Errorf("addr = %q, releases = %d", s.addr, f.release)
	}
}
