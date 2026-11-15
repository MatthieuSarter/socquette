package config

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
)

// Errors of the user edits, mapped to translated messages by the UI.
var (
	ErrNameEmpty        = errors.New("empty user name")
	ErrNameTaken        = errors.New("user name already used")
	ErrPasswordEmpty    = errors.New("empty password")
	ErrPasswordMismatch = errors.New("passwords don't match")
)

func checkPassword(pw1, pw2 string) error {
	if pw1 == "" {
		return ErrPasswordEmpty
	}
	if pw1 != pw2 {
		return ErrPasswordMismatch
	}
	return nil
}

// AddUser returns a copy of c with a new user whose password (typed twice) is stored hashed by hash.
func AddUser(c Config, name, pw1, pw2 string, http, socks bool, hash func(string) (string, error)) (Config, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return c, ErrNameEmpty
	}
	if slices.ContainsFunc(c.Users, func(u User) bool { return u.Name == name }) {
		return c, ErrNameTaken
	}
	if err := checkPassword(pw1, pw2); err != nil {
		return c, err
	}
	h, err := hash(pw1)
	if err != nil {
		return c, err
	}
	c.Users = append(slices.Clone(c.Users), User{Name: name, PasswordHash: h, HTTP: http, SOCKS: socks})
	return c, nil
}

// SetPassword returns a copy of c where the user's password is replaced.
func SetPassword(c Config, name, pw1, pw2 string, hash func(string) (string, error)) (Config, error) {
	i := slices.IndexFunc(c.Users, func(u User) bool { return u.Name == name })
	if i < 0 {
		return c, fmt.Errorf("unknown user %q", name)
	}
	if err := checkPassword(pw1, pw2); err != nil {
		return c, err
	}
	h, err := hash(pw1)
	if err != nil {
		return c, err
	}
	c.Users = slices.Clone(c.Users)
	c.Users[i].PasswordHash = h
	return c, nil
}

// RemoveUser returns a copy of c without the named user.
func RemoveUser(c Config, name string) Config {
	c.Users = slices.DeleteFunc(slices.Clone(c.Users), func(u User) bool { return u.Name == name })
	return c
}

// ListenSuggestions lists the listen addresses the UI offers: all interfaces, loopback, then the machine's own IPs.
func ListenSuggestions(ifaceAddrs []net.Addr) []string {
	out := []string{"0.0.0.0", "::", "127.0.0.1"}
	for _, a := range ifaceAddrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip != nil && !slices.Contains(out, ip.String()) {
			out = append(out, ip.String())
		}
	}
	return out
}
