package config

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseDefaultsForMissingKeys(t *testing.T) {
	c, err := Parse([]byte("[http]\nenabled = true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !c.HTTP.Enabled || c.HTTP.Port != 8080 || c.HTTP.Listen != "0.0.0.0" {
		t.Errorf("http = %+v", c.HTTP)
	}
	if c.SOCKS.Enabled || c.SOCKS.Port != 1080 || c.SOCKS.Listen != "0.0.0.0" {
		t.Errorf("socks = %+v", c.SOCKS)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse([]byte("[http\n")); err == nil {
		t.Error("expected a TOML error")
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	if _, err := Parse([]byte("[http]\nport = 0\n")); err == nil {
		t.Error("expected a validation error")
	}
}

func TestRoundTrip(t *testing.T) {
	in := Defaults()
	in.SOCKS.Enabled = true
	in.SOCKS.AdvertiseAddress = "home.example.org"
	in.Users = []User{
		{Name: "alice", PasswordHash: "h1", HTTP: true, SOCKS: true},
		{Name: "bob", PasswordHash: "h2", SOCKS: true},
	}
	data, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip:\n in=%+v\nout=%+v", in, out)
	}
}

func TestAddr(t *testing.T) {
	c := Defaults()
	c.SOCKS.Listen = "::"
	if got := c.HTTP.Addr(); got != "0.0.0.0:8080" {
		t.Errorf("http addr = %q", got)
	}
	if got := c.SOCKS.Addr(); got != "[::]:1080" {
		t.Errorf("socks addr = %q", got)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*Config)
		ok     bool
	}{
		{"defaults", func(*Config) { /* left as Defaults() */ }, true},
		{"port 0", func(c *Config) { c.HTTP.Port = 0 }, false},
		{"port 65536", func(c *Config) { c.SOCKS.Port = 65536 }, false},
		{"listen interface name", func(c *Config) { c.HTTP.Listen = "eth0" }, false},
		{"listen ipv6", func(c *Config) { c.SOCKS.Listen = "::1" }, true},
		{"advertise host name", func(c *Config) { c.SOCKS.AdvertiseAddress = "my.dyndns.org" }, true},
		{"advertise ip", func(c *Config) { c.SOCKS.AdvertiseAddress = "203.0.113.7" }, true},
		{"advertise invalid", func(c *Config) { c.SOCKS.AdvertiseAddress = "bad host!" }, false},
		{"same addr both enabled", func(c *Config) {
			c.HTTP.Enabled, c.SOCKS.Enabled, c.HTTP.Port = true, true, 1080
		}, false},
		{"same addr one disabled", func(c *Config) { c.SOCKS.Enabled, c.HTTP.Port = true, 1080 }, true},
		{"duplicate user", func(c *Config) {
			c.Users = []User{{Name: "a", PasswordHash: "h"}, {Name: "a", PasswordHash: "h"}}
		}, false},
		{"empty name", func(c *Config) { c.Users = []User{{PasswordHash: "h"}} }, false},
		{"name too long", func(c *Config) {
			c.Users = []User{{Name: strings.Repeat("a", 256), PasswordHash: "h"}}
		}, false},
		{"empty hash", func(c *Config) { c.Users = []User{{Name: "a"}} }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Defaults()
			tt.modify(&c)
			err := Validate(c)
			if (err == nil) != tt.ok {
				t.Errorf("Validate() = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestValidateJoinsAllErrors(t *testing.T) {
	c := Defaults()
	c.HTTP.Port = 0
	c.Users = []User{{Name: "", PasswordHash: "h"}}
	err := Validate(c)
	if err == nil {
		t.Fatal("expected errors")
	}
	msg := err.Error()
	if !strings.Contains(msg, "http.port") || !strings.Contains(msg, "name") {
		t.Errorf("missing an error in %q", msg)
	}
}

func TestValidateSentinels(t *testing.T) {
	tests := []struct {
		modify func(*Config)
		want   error
	}{
		{func(c *Config) { c.HTTP.Port = 0 }, ErrInvalidPort},
		{func(c *Config) { c.SOCKS.Listen = "x" }, ErrInvalidListen},
		{func(c *Config) { c.SOCKS.AdvertiseAddress = "bad host!" }, ErrInvalidAdvertise},
		{func(c *Config) { c.HTTP.Enabled, c.SOCKS.Enabled, c.HTTP.Port = true, true, 1080 }, ErrSameAddress},
		{func(c *Config) { c.Networks = []Network{{Name: "x", Addresses: []string{"10.0.0.0/33"}}} },
			ErrInvalidNetwork},
		{func(c *Config) { c.Networks = []Network{{Name: "lan"}} }, ErrNoAddresses},
		{func(c *Config) { c.Networks = []Network{{Addresses: []string{"10.0.0.1"}}} }, ErrNameEmpty},
		{func(c *Config) {
			c.Networks = []Network{{Name: "a", Addresses: []string{"::1"}}, {Name: "a", Addresses: []string{"::1"}}}
		}, ErrNameTaken},
	}
	for _, tt := range tests {
		c := Defaults()
		tt.modify(&c)
		if err := Validate(c); !errors.Is(err, tt.want) {
			t.Errorf("Validate() = %v, want %v", err, tt.want)
		}
	}
}

func TestParseNetwork(t *testing.T) {
	tests := []struct{ in, want string }{
		{"192.168.1.0/24", "192.168.1.0/24"},
		{" 192.168.1.7/24 ", "192.168.1.0/24"},
		{"10.0.0.1", "10.0.0.1/32"},
		{"2001:db8::/32", "2001:db8::/32"},
		{"2001:db8::1", "2001:db8::1/128"},
		{"::ffff:10.1.2.3", "10.1.2.3/32"},
		{"::ffff:10.0.0.0/104", "10.0.0.0/8"},
		{"::/0", "::/0"},
	}
	for _, tt := range tests {
		got, err := ParseNetwork(tt.in)
		if err != nil || got.String() != tt.want {
			t.Errorf("ParseNetwork(%q) = %v, %v, want %s", tt.in, got, err, tt.want)
		}
	}
	for _, bad := range []string{"", "lan", "10.0.0.256", "10.0.0.0/33", "fe80::1%eth0", "10.0.0.0/"} {
		if _, err := ParseNetwork(bad); !errors.Is(err, ErrInvalidNetwork) {
			t.Errorf("ParseNetwork(%q) = %v, want ErrInvalidNetwork", bad, err)
		}
	}
}

func TestParseNetworks(t *testing.T) {
	c, err := Parse([]byte(`
[[networks]]
name = "lan"
addresses = ["192.168.1.0/24", "fd00::/8"]
socks = true
`))
	want := []Network{{Name: "lan", Addresses: []string{"192.168.1.0/24", "fd00::/8"}, SOCKS: true}}
	if err != nil || !reflect.DeepEqual(c.Networks, want) {
		t.Errorf("networks = %+v, %v", c.Networks, err)
	}
}
