// Package auth checks proxy credentials against the configured users.
package auth

import (
	"crypto/sha256"
	"fmt"
	"runtime"
	"sync"

	"golang.org/x/crypto/bcrypt"

	"github.com/MatthieuSarter/socquette/internal/config"
)

// Proto identifies the proxy a credential check is for.
type Proto int

// The two proxies.
const (
	HTTP Proto = iota
	SOCKS
)

// HashPassword returns the bcrypt hash stored in the configuration for a password.
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(h), nil
}

// verifySlots bounds the bcrypt checks running at once, across reloads: each costs tens of milliseconds of a core,
// and anyone who finds an Internet-exposed port can send failing attempts in parallel.
var verifySlots = make(chan struct{}, runtime.NumCPU())

// dummyHash is compared against for unknown users, so they take as long to reject as a wrong password.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("socquette"), bcrypt.DefaultCost)

// Authenticator is an immutable snapshot of the users of one configuration, plus a cache of successful checks.
// Build a new one on every configuration reload: that also drops the cache.
type Authenticator struct {
	users     map[string]config.User
	anonymous [2]bool
	cache     sync.Map // [sha256.Size]byte -> struct{}
	verify    func(hash, password []byte) error
	sem       chan struct{} // bounds concurrent bcrypt checks
}

// New builds the authenticator of a configuration.
func New(c config.Config) *Authenticator {
	a := &Authenticator{
		users:     make(map[string]config.User, len(c.Users)),
		anonymous: [2]bool{HTTP: c.HTTP.Anonymous, SOCKS: c.SOCKS.Anonymous},
		verify:    bcrypt.CompareHashAndPassword,
		sem:       verifySlots,
	}
	for _, u := range c.Users {
		a.users[u.Name] = u
	}
	return a
}

// Anonymous reports whether the proxy accepts clients that send no credentials.
func (a *Authenticator) Anonymous(p Proto) bool { return a.anonymous[p] }

// HasUsers reports whether at least one user has the right to use the proxy.
func (a *Authenticator) HasUsers(p Proto) bool {
	for _, u := range a.users {
		if allowed(u, p) {
			return true
		}
	}
	return false
}

func allowed(u config.User, p Proto) bool {
	if p == HTTP {
		return u.HTTP
	}
	return u.SOCKS
}

// Check reports whether name/password are valid credentials of a user who has the right to use the proxy.
func (a *Authenticator) Check(p Proto, name, password string) bool {
	u, ok := a.users[name]
	if !ok {
		_ = a.slowVerify(dummyHash, []byte(password))
		return false
	}
	if !allowed(u, p) {
		return false
	}
	// ponytail: unbounded cache, but it only grows with successful logins of configured users.
	key := sha256.Sum256([]byte(name + "\x00" + password))
	if _, hit := a.cache.Load(key); hit {
		return true
	}
	if a.slowVerify([]byte(u.PasswordHash), []byte(password)) != nil {
		return false
	}
	a.cache.Store(key, struct{}{})
	return true
}

// slowVerify runs a bcrypt check once a slot is free.
func (a *Authenticator) slowVerify(hash, password []byte) error {
	a.sem <- struct{}{}
	defer func() { <-a.sem }()
	return a.verify(hash, password)
}
