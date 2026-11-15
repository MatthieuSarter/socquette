package auth

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MatthieuSarter/socquette/internal/config"
)

func newTestAuth(t *testing.T, anonymous bool) *Authenticator {
	t.Helper()
	alice, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	c := config.Defaults()
	c.HTTP.Anonymous = anonymous
	c.SOCKS.Anonymous = anonymous
	c.Users = []config.User{
		{Name: "alice", PasswordHash: alice, HTTP: true, SOCKS: true},
		{Name: "bob", PasswordHash: bob, HTTP: true, SOCKS: false},
	}
	return New(c)
}

func TestHashPasswordIsNotThePassword(t *testing.T) {
	h, err := HashPassword("secret")
	if err != nil || h == "" || h == "secret" {
		t.Fatalf("hash=%q err=%v", h, err)
	}
}

func TestCheck(t *testing.T) {
	a := newTestAuth(t, false)
	tests := []struct {
		name     string
		proto    Proto
		user, pw string
		want     bool
	}{
		{"right creds", SOCKS, "alice", "secret", true},
		{"wrong password", SOCKS, "alice", "nope", false},
		{"unknown user", HTTP, "carol", "secret", false},
		{"no socks right", SOCKS, "bob", "hunter2", false},
		{"http right", HTTP, "bob", "hunter2", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := a.Check(tt.proto, tt.user, tt.pw); got != tt.want {
				t.Errorf("Check() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckCachesSuccess(t *testing.T) {
	a := newTestAuth(t, false)
	calls := 0
	inner := a.verify
	a.verify = func(hash, pw []byte) error { calls++; return inner(hash, pw) }
	a.Check(HTTP, "alice", "secret")
	a.Check(HTTP, "alice", "secret")
	if calls != 1 {
		t.Errorf("verify called %d times, want 1", calls)
	}
}

func TestCheckFailureNotCached(t *testing.T) {
	a := newTestAuth(t, false)
	calls := 0
	inner := a.verify
	a.verify = func(hash, pw []byte) error { calls++; return inner(hash, pw) }
	a.Check(HTTP, "alice", "nope")
	if !a.Check(HTTP, "alice", "secret") || calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestAnonymousWrongCredsRejected(t *testing.T) {
	a := newTestAuth(t, true)
	if !a.Anonymous(SOCKS) {
		t.Fatal("anonymous not reported")
	}
	if a.Check(SOCKS, "alice", "nope") {
		t.Error("wrong credentials accepted on an anonymous proxy")
	}
}

func TestHasUsers(t *testing.T) {
	a := newTestAuth(t, false)
	if !a.HasUsers(HTTP) || !a.HasUsers(SOCKS) {
		t.Error("users not reported")
	}
	if New(config.Defaults()).HasUsers(SOCKS) {
		t.Error("no users expected")
	}
	c := config.Defaults()
	c.Users = []config.User{{Name: "x", PasswordHash: "h", HTTP: true}}
	if New(c).HasUsers(SOCKS) {
		t.Error("user without socks right counted")
	}
}

func TestCheckBoundsConcurrentVerifies(t *testing.T) {
	a := newTestAuth(t, false)
	a.sem = make(chan struct{}, 2)
	var mu sync.Mutex
	active, peak := 0, 0
	a.verify = func([]byte, []byte) error {
		mu.Lock()
		active++
		peak = max(peak, active)
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
		return errors.New("wrong")
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() { a.Check(SOCKS, "alice", "nope") })
	}
	wg.Wait()
	if peak > 2 {
		t.Errorf("%d bcrypt checks ran at once, want at most 2", peak)
	}
}
