// Package config holds the socquette.toml model, its validation and its file I/O.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// maxCredentialLen is the RFC 1929 limit on SOCKS5 user names and passwords.
const maxCredentialLen = 255

// Validation errors, wrapped by Validate so the UI can translate them.
var (
	ErrInvalidListen    = errors.New("not an IP address")
	ErrInvalidPort      = errors.New("not a port in 1-65535")
	ErrInvalidAdvertise = errors.New("neither an IP address nor a host name")
	ErrSameAddress      = errors.New("http and socks listen on the same address")
	ErrInvalidNetwork   = errors.New("neither an IP address nor a CIDR subnet")
	ErrNoAddresses      = errors.New("no address")
)

// HTTP is the [http] section: the HTTP proxy listener.
type HTTP struct {
	Enabled   bool   `toml:"enabled"`
	Listen    string `toml:"listen"`
	Port      int    `toml:"port"`
	Anonymous bool   `toml:"anonymous"`
}

// SOCKS is the [socks] section: the SOCKS5 listener (TCP, and UDP on the same port number).
type SOCKS struct {
	Enabled   bool   `toml:"enabled"`
	Listen    string `toml:"listen"`
	Port      int    `toml:"port"`
	Anonymous bool   `toml:"anonymous"`
	// AdvertiseAddress is the IP or host name announced in UDP ASSOCIATE replies; empty means 0.0.0.0.
	AdvertiseAddress string `toml:"advertise_address"`
}

// User is one [[users]] entry.
type User struct {
	Name         string `toml:"name"`
	PasswordHash string `toml:"password_hash"`
	HTTP         bool   `toml:"http"`
	SOCKS        bool   `toml:"socks"`
}

// Network is one [[networks]] entry: clients whose source address is in Addresses are accepted without credentials.
type Network struct {
	Name      string   `toml:"name"`
	Addresses []string `toml:"addresses"` // IPs or CIDR subnets, IPv4 or IPv6
	HTTP      bool     `toml:"http"`
	SOCKS     bool     `toml:"socks"`
}

// Config is the whole socquette.toml file.
type Config struct {
	HTTP     HTTP      `toml:"http"`
	SOCKS    SOCKS     `toml:"socks"`
	Users    []User    `toml:"users"`
	Networks []Network `toml:"networks,omitempty"`
}

// Defaults returns the configuration used when the file or some of its keys are missing.
func Defaults() Config {
	return Config{
		HTTP:  HTTP{Listen: "0.0.0.0", Port: 8080},
		SOCKS: SOCKS{Listen: "0.0.0.0", Port: 1080},
	}
}

// Addr returns the HTTP listen address as host:port.
func (h HTTP) Addr() string { return net.JoinHostPort(h.Listen, strconv.Itoa(h.Port)) }

// Addr returns the SOCKS listen address as host:port.
func (s SOCKS) Addr() string { return net.JoinHostPort(s.Listen, strconv.Itoa(s.Port)) }

// Parse decodes a TOML document on top of Defaults() and validates the result.
func Parse(data []byte) (Config, error) {
	c := Defaults()
	if err := toml.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse: %w", err)
	}
	if err := Validate(c); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Marshal encodes a configuration as TOML.
func Marshal(c Config) ([]byte, error) {
	data, err := toml.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	return data, nil
}

// ParseNetwork parses an IP or a CIDR subnet; a bare IP is a /32 (IPv4) or /128 (IPv6) subnet. IPv4-mapped IPv6
// addresses are taken as IPv4, as client addresses are.
func ParseNetwork(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "/") {
		ip, err := netip.ParseAddr(s)
		if err != nil || ip.Zone() != "" {
			return netip.Prefix{}, fmt.Errorf("%q: %w", s, ErrInvalidNetwork)
		}
		ip = ip.Unmap()
		return netip.PrefixFrom(ip, ip.BitLen()), nil
	}
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%q: %w", s, ErrInvalidNetwork)
	}
	if p.Addr().Is4In6() && p.Bits() >= 96 {
		p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
	}
	return p.Masked(), nil
}

var hostLabel = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

func validHost(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	if len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if !hostLabel.MatchString(label) {
			return false
		}
	}
	return true
}

func validateListener(section, listen string, port int) []error {
	var errs []error
	if net.ParseIP(listen) == nil {
		errs = append(errs, fmt.Errorf("%s.listen %q: %w", section, listen, ErrInvalidListen))
	}
	if port < 1 || port > 65535 {
		errs = append(errs, fmt.Errorf("%s.port %d: %w", section, port, ErrInvalidPort))
	}
	return errs
}

// Validate returns every problem of a configuration joined in one error, or nil when it is valid.
func Validate(c Config) error {
	errs := validateListener("http", c.HTTP.Listen, c.HTTP.Port)
	errs = append(errs, validateListener("socks", c.SOCKS.Listen, c.SOCKS.Port)...)
	if c.SOCKS.AdvertiseAddress != "" && !validHost(c.SOCKS.AdvertiseAddress) {
		errs = append(errs, fmt.Errorf("socks.advertise_address %q: %w", c.SOCKS.AdvertiseAddress, ErrInvalidAdvertise))
	}
	if c.HTTP.Enabled && c.SOCKS.Enabled && c.HTTP.Port == c.SOCKS.Port && c.HTTP.Listen == c.SOCKS.Listen {
		errs = append(errs, fmt.Errorf("%w: %s", ErrSameAddress, c.HTTP.Addr()))
	}
	seen := map[string]bool{}
	for i, u := range c.Users {
		switch {
		case u.Name == "":
			errs = append(errs, fmt.Errorf("users[%d].name: empty", i))
		case len(u.Name) > maxCredentialLen:
			errs = append(errs, fmt.Errorf("users[%d].name: longer than %d bytes", i, maxCredentialLen))
		case seen[u.Name]:
			errs = append(errs, fmt.Errorf("users[%d].name: duplicate %q", i, u.Name))
		}
		seen[u.Name] = true
		if u.PasswordHash == "" {
			errs = append(errs, fmt.Errorf("users[%d].password_hash: empty", i))
		}
	}
	return errors.Join(append(errs, validateNetworks(c.Networks)...)...)
}

func validateNetworks(networks []Network) []error {
	var errs []error
	seen := map[string]bool{}
	for i, n := range networks {
		switch {
		case n.Name == "":
			errs = append(errs, fmt.Errorf("networks[%d].name: %w", i, ErrNameEmpty))
		case seen[n.Name]:
			errs = append(errs, fmt.Errorf("networks[%d].name %q: %w", i, n.Name, ErrNameTaken))
		}
		seen[n.Name] = true
		if len(n.Addresses) == 0 {
			errs = append(errs, fmt.Errorf("networks[%d].addresses: %w", i, ErrNoAddresses))
		}
		for j, a := range n.Addresses {
			if _, err := ParseNetwork(a); err != nil {
				errs = append(errs, fmt.Errorf("networks[%d].addresses[%d] %w", i, j, err))
			}
		}
	}
	return errs
}
