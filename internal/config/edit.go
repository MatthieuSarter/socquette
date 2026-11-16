package config

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"unicode"
)

// Errors of the user and network edits, mapped to translated messages by the UI.
var (
	ErrNameEmpty        = errors.New("empty name")
	ErrNameTaken        = errors.New("name already used")
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

// parseAddresses splits a comma- or space-separated list of IPs and CIDR subnets and checks each of them.
func parseAddresses(list string) ([]string, error) {
	addrs := strings.FieldsFunc(list, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	if len(addrs) == 0 {
		return nil, ErrNoAddresses
	}
	for _, a := range addrs {
		if _, err := ParseNetwork(a); err != nil {
			return nil, err
		}
	}
	return addrs, nil
}

// AddNetwork returns a copy of c with a new trusted network; addresses is a comma- or space-separated list.
func AddNetwork(c Config, name, addresses string, http, socks bool) (Config, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return c, ErrNameEmpty
	}
	if slices.ContainsFunc(c.Networks, func(n Network) bool { return n.Name == name }) {
		return c, ErrNameTaken
	}
	addrs, err := parseAddresses(addresses)
	if err != nil {
		return c, err
	}
	c.Networks = append(slices.Clone(c.Networks), Network{Name: name, Addresses: addrs, HTTP: http, SOCKS: socks})
	return c, nil
}

// SetNetworkAddresses returns a copy of c where the network's addresses are replaced.
func SetNetworkAddresses(c Config, name, addresses string) (Config, error) {
	i := slices.IndexFunc(c.Networks, func(n Network) bool { return n.Name == name })
	if i < 0 {
		return c, fmt.Errorf("unknown network %q", name)
	}
	addrs, err := parseAddresses(addresses)
	if err != nil {
		return c, err
	}
	c.Networks = slices.Clone(c.Networks)
	c.Networks[i].Addresses = addrs
	return c, nil
}

// RemoveNetwork returns a copy of c without the named network.
func RemoveNetwork(c Config, name string) Config {
	c.Networks = slices.DeleteFunc(slices.Clone(c.Networks), func(n Network) bool { return n.Name == name })
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
