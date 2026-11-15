package socks5

import (
	"bytes"
	"errors"
	"net"
	"testing"
)

const (
	errVersionFmt = "err = %v, want ErrVersion"
	exampleHost   = "example.com"
)

func TestReadGreeting(t *testing.T) {
	m, err := ReadGreeting(bytes.NewReader([]byte{5, 2, 0, 2}))
	if err != nil || !bytes.Equal(m, []byte{0, 2}) {
		t.Fatalf("methods=%v err=%v", m, err)
	}
	if _, err := ReadGreeting(bytes.NewReader([]byte{4, 1, 0})); !errors.Is(err, ErrVersion) {
		t.Errorf(errVersionFmt, err)
	}
	if _, err := ReadGreeting(bytes.NewReader([]byte{5, 2, 0})); err == nil {
		t.Error("truncated greeting accepted")
	}
}

func TestChooseMethod(t *testing.T) {
	tests := []struct {
		offered          []byte
		anonymous, users bool
		want             byte
	}{
		{[]byte{0, 2}, true, true, MethodUserPass},
		{[]byte{0}, true, true, MethodNoAuth},
		{[]byte{0}, false, true, MethodNone},
		{[]byte{2}, true, false, MethodNone},
		{[]byte{0, 2}, true, false, MethodNoAuth},
		{[]byte{2}, false, true, MethodUserPass},
	}
	for _, tt := range tests {
		if got := ChooseMethod(tt.offered, tt.anonymous, tt.users); got != tt.want {
			t.Errorf("ChooseMethod(%v, %v, %v) = %#x, want %#x", tt.offered, tt.anonymous, tt.users, got, tt.want)
		}
	}
}

func TestReadUserPass(t *testing.T) {
	in := append(append([]byte{1, 5}, "alice"...), append([]byte{3}, "pwd"...)...)
	name, pw, err := ReadUserPass(bytes.NewReader(in))
	if err != nil || name != "alice" || pw != "pwd" {
		t.Fatalf("name=%q pw=%q err=%v", name, pw, err)
	}
	if _, _, err := ReadUserPass(bytes.NewReader([]byte{5, 1, 'a', 0})); !errors.Is(err, ErrVersion) {
		t.Errorf(errVersionFmt, err)
	}
	if _, _, err := ReadUserPass(bytes.NewReader([]byte{1, 5, 'a'})); err == nil {
		t.Error("truncated user/pass accepted")
	}
}

func TestWriteUserPassStatus(t *testing.T) {
	var b bytes.Buffer
	_ = WriteUserPassStatus(&b, true)
	_ = WriteUserPassStatus(&b, false)
	if !bytes.Equal(b.Bytes(), []byte{1, 0, 1, 1}) {
		t.Errorf("got %v", b.Bytes())
	}
}

func TestReadRequest(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"ipv4", []byte{5, 1, 0, 1, 127, 0, 0, 1, 0x1F, 0x90}, "127.0.0.1:8080"},
		{"domain", append(append([]byte{5, 1, 0, 3, 11}, exampleHost...), 0, 80), "example.com:80"},
		{"ipv6", append(append([]byte{5, 3, 0, 4}, net.ParseIP("::1")...), 0, 53), "[::1]:53"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := ReadRequest(bytes.NewReader(tt.in))
			if err != nil || r.Dst.String() != tt.want || r.Cmd != tt.in[1] {
				t.Fatalf("req=%+v (%s) err=%v", r, r.Dst, err)
			}
		})
	}
	if _, err := ReadRequest(bytes.NewReader([]byte{5, 1, 0, 9, 0, 0})); !errors.Is(err, ErrAddressType) {
		t.Errorf("err = %v, want ErrAddressType", err)
	}
	if _, err := ReadRequest(bytes.NewReader([]byte{4, 1, 0, 1})); !errors.Is(err, ErrVersion) {
		t.Errorf(errVersionFmt, err)
	}
	if _, err := ReadRequest(bytes.NewReader([]byte{5, 1, 0, 1, 127})); err == nil {
		t.Error("truncated request accepted")
	}
}

func TestWriteReply(t *testing.T) {
	var b bytes.Buffer
	_ = WriteReply(&b, RepSucceeded, Addr{IP: net.ParseIP("203.0.113.7"), Port: 1080})
	if !bytes.Equal(b.Bytes(), []byte{5, 0, 0, 1, 203, 0, 113, 7, 0x04, 0x38}) {
		t.Errorf("ipv4 reply = %v", b.Bytes())
	}
	b.Reset()
	_ = WriteReply(&b, RepCommandNotSupported, Addr{})
	if !bytes.Equal(b.Bytes(), []byte{5, 7, 0, 1, 0, 0, 0, 0, 0, 0}) {
		t.Errorf("zero reply = %v", b.Bytes())
	}
	b.Reset()
	_ = WriteReply(&b, RepSucceeded, Addr{IP: net.ParseIP("::1"), Port: 1})
	if b.Len() != 4+16+2 || b.Bytes()[3] != 4 {
		t.Errorf("ipv6 reply = %v", b.Bytes())
	}
}

func TestUDPRoundTrip(t *testing.T) {
	for _, a := range []Addr{
		{IP: net.ParseIP("192.0.2.1").To4(), Port: 53},
		{IP: net.ParseIP("2001:db8::1"), Port: 443},
		{Host: exampleHost, Port: 80},
	} {
		pkt := append(AppendUDPHeader(nil, a), "payload"...)
		frag, dst, payload, err := ParseUDP(pkt)
		if err != nil || frag != 0 || dst.String() != a.String() || string(payload) != "payload" {
			t.Errorf("%s: frag=%d dst=%s payload=%q err=%v", a, frag, dst, payload, err)
		}
	}
}

func TestParseUDPTruncated(t *testing.T) {
	for _, b := range [][]byte{nil, {0, 0, 0}, {0, 0, 0, 1, 1, 2}, {0, 0, 0, 3, 5, 'a'}, {0, 0, 0, 9, 0, 0}} {
		if _, _, _, err := ParseUDP(b); err == nil {
			t.Errorf("ParseUDP(%v) accepted", b)
		}
	}
}

func FuzzParseUDP(f *testing.F) {
	f.Add(append(AppendUDPHeader(nil, Addr{Host: exampleHost, Port: 80}), 'x'))
	f.Add([]byte{0, 0, 0, 1, 1, 2, 3, 4, 0, 53})
	f.Add([]byte{0, 0, 0, 4})
	f.Fuzz(func(_ *testing.T, b []byte) {
		_, _, _, _ = ParseUDP(b)
	})
}
