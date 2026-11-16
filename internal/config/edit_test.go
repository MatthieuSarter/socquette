package config

import (
	"errors"
	"net"
	"reflect"
	"testing"
)

// lanIP is a machine address that the interfaces list twice (as a network and as an address).
const lanIP = "192.168.1.5"

func fakeHash(pw string) (string, error) { return "hash:" + pw, nil }

func TestAddUser(t *testing.T) {
	c, err := AddUser(Defaults(), "alice", "pw", "pw", true, false, fakeHash)
	if err != nil {
		t.Fatal(err)
	}
	want := User{Name: "alice", PasswordHash: "hash:pw", HTTP: true}
	if len(c.Users) != 1 || c.Users[0] != want {
		t.Errorf("users = %+v", c.Users)
	}
	if _, err := AddUser(c, "alice", "x", "x", true, true, fakeHash); !errors.Is(err, ErrNameTaken) {
		t.Errorf("duplicate: %v", err)
	}
	if _, err := AddUser(c, "bob", "x", "y", true, true, fakeHash); !errors.Is(err, ErrPasswordMismatch) {
		t.Errorf("mismatch: %v", err)
	}
	if _, err := AddUser(c, " ", "x", "x", true, true, fakeHash); !errors.Is(err, ErrNameEmpty) {
		t.Errorf("empty name: %v", err)
	}
	if _, err := AddUser(c, "bob", "", "", true, true, fakeHash); !errors.Is(err, ErrPasswordEmpty) {
		t.Errorf("empty password: %v", err)
	}
	hashErr := errors.New("boom")
	failing := func(string) (string, error) { return "", hashErr }
	if _, err := AddUser(c, "bob", "x", "x", true, true, failing); !errors.Is(err, hashErr) {
		t.Errorf("hash error: %v", err)
	}
}

func TestAddUserDoesNotModifyInput(t *testing.T) {
	in, _ := AddUser(Defaults(), "alice", "pw", "pw", true, true, fakeHash)
	_, _ = AddUser(in, "bob", "pw", "pw", true, true, fakeHash)
	if len(in.Users) != 1 {
		t.Errorf("input modified: %+v", in.Users)
	}
}

func TestSetPassword(t *testing.T) {
	c, _ := AddUser(Defaults(), "alice", "old", "old", true, true, fakeHash)
	c2, err := SetPassword(c, "alice", "new", "new", fakeHash)
	if err != nil || c2.Users[0].PasswordHash != "hash:new" || c.Users[0].PasswordHash != "hash:old" {
		t.Fatalf("users = %+v / %+v, err = %v", c2.Users, c.Users, err)
	}
	if _, err := SetPassword(c, "alice", "a", "b", fakeHash); !errors.Is(err, ErrPasswordMismatch) {
		t.Errorf("mismatch: %v", err)
	}
	if _, err := SetPassword(c, "nobody", "a", "a", fakeHash); err == nil {
		t.Error("unknown user accepted")
	}
}

func TestRemoveUser(t *testing.T) {
	c, _ := AddUser(Defaults(), "alice", "pw", "pw", true, true, fakeHash)
	c, _ = AddUser(c, "bob", "pw", "pw", true, true, fakeHash)
	out := RemoveUser(c, "alice")
	if len(out.Users) != 1 || out.Users[0].Name != "bob" || len(c.Users) != 2 {
		t.Errorf("out = %+v, in = %+v", out.Users, c.Users)
	}
}

func TestListenSuggestions(t *testing.T) {
	addrs := []net.Addr{
		&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
		&net.IPNet{IP: net.ParseIP(lanIP), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
		&net.IPAddr{IP: net.ParseIP(lanIP)},
	}
	want := []string{"0.0.0.0", "::", "127.0.0.1", lanIP, "fe80::1"}
	if got := ListenSuggestions(addrs); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestAddNetwork(t *testing.T) {
	c, err := AddNetwork(Defaults(), " lan ", "192.168.1.0/24, fd00::/8\n10.0.0.1", true, false)
	if err != nil {
		t.Fatal(err)
	}
	want := Network{Name: "lan", Addresses: []string{"192.168.1.0/24", "fd00::/8", "10.0.0.1"}, HTTP: true}
	if len(c.Networks) != 1 || !reflect.DeepEqual(c.Networks[0], want) {
		t.Errorf("networks = %+v", c.Networks)
	}
	tests := []struct {
		name, addrs string
		want        error
	}{
		{"lan", "10.0.0.1", ErrNameTaken},
		{" ", "10.0.0.1", ErrNameEmpty},
		{"vpn", " , ", ErrNoAddresses},
		{"vpn", "10.0.0.1, nope", ErrInvalidNetwork},
	}
	for _, tt := range tests {
		if _, err := AddNetwork(c, tt.name, tt.addrs, true, true); !errors.Is(err, tt.want) {
			t.Errorf("AddNetwork(%q, %q) = %v, want %v", tt.name, tt.addrs, err, tt.want)
		}
	}
	if err := Validate(c); err != nil {
		t.Errorf("added network invalid: %v", err)
	}
}

func TestSetNetworkAddresses(t *testing.T) {
	orig, _ := AddNetwork(Defaults(), "lan", "10.0.0.0/8", true, true)
	c, err := SetNetworkAddresses(orig, "lan", "192.168.0.0/16")
	if err != nil || !reflect.DeepEqual(c.Networks[0].Addresses, []string{"192.168.0.0/16"}) {
		t.Errorf("networks = %+v, %v", c.Networks, err)
	}
	if orig.Networks[0].Addresses[0] != "10.0.0.0/8" {
		t.Error("input modified")
	}
	if _, err := SetNetworkAddresses(orig, "lan", "bad"); !errors.Is(err, ErrInvalidNetwork) {
		t.Errorf("bad address: %v", err)
	}
	if _, err := SetNetworkAddresses(orig, "nobody", "10.0.0.1"); err == nil {
		t.Error("unknown network accepted")
	}
}

func TestRemoveNetwork(t *testing.T) {
	c, _ := AddNetwork(Defaults(), "lan", "10.0.0.0/8", true, true)
	c, _ = AddNetwork(c, "vpn", "10.8.0.0/24", true, true)
	if c = RemoveNetwork(c, "lan"); len(c.Networks) != 1 || c.Networks[0].Name != "vpn" {
		t.Errorf("networks = %+v", c.Networks)
	}
}
