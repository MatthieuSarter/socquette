package socks5

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MatthieuSarter/socquette/internal/auth"
	"github.com/MatthieuSarter/socquette/internal/config"
)

const (
	loopbackAnyPort = "127.0.0.1:0"
	userAlice       = "alice"
	passSecret      = "secret"
	replyFmt        = "reply = %#x"
)

// testHash is the bcrypt hash of passSecret, computed once for the whole test binary.
var testHash, _ = auth.HashPassword(passSecret)

type testServer struct {
	tcp string
	udp *net.UDPAddr
	log *lockedWriter
}

func startServer(t *testing.T, anonymous bool, advertise string) testServer {
	t.Helper()
	c := config.Defaults()
	c.SOCKS.Anonymous = anonymous
	c.Users = []config.User{
		{Name: userAlice, PasswordHash: testHash, SOCKS: true},
		{Name: "bob", PasswordHash: testHash, HTTP: true},
	}
	a := auth.New(c)
	ln, err := net.Listen("tcp", loopbackAnyPort)
	if err != nil {
		t.Fatal(err)
	}
	pc, err := net.ListenPacket("udp", loopbackAnyPort)
	if err != nil {
		t.Fatal(err)
	}
	logs := &lockedWriter{}
	s := &Server{
		Auth:      func() *auth.Authenticator { return a },
		Advertise: func() string { return advertise },
		Log:       slog.New(slog.NewTextHandler(logs, nil)),
	}
	go func() { _ = s.Serve(ln, pc) }()
	t.Cleanup(func() { _ = ln.Close(); _ = pc.Close() })
	return testServer{tcp: ln.Addr().String(), udp: pc.LocalAddr().(*net.UDPAddr), log: logs}
}

// handshake connects and authenticates; user "" means no authentication. It returns the conn and the auth status.
func handshake(t *testing.T, addr, user, pw string) (net.Conn, bool) {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	method := MethodNoAuth
	if user != "" {
		method = MethodUserPass
	}
	if _, err := c.Write([]byte{5, 1, method}); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(c, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != method {
		return c, false
	}
	if user == "" {
		return c, true
	}
	msg := append(append([]byte{1, byte(len(user))}, user...), byte(len(pw)))
	if _, err := c.Write(append(msg, pw...)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(c, reply); err != nil {
		t.Fatal(err)
	}
	return c, reply[1] == 0
}

// request sends a command and returns the reply code and bound address.
func request(t *testing.T, c net.Conn, cmd byte, dst Addr) (byte, Addr) {
	t.Helper()
	if _, err := c.Write(appendAddr([]byte{5, cmd, 0}, dst)); err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(c, head); err != nil {
		t.Fatal(err)
	}
	bnd, err := readAddr(c, head[3])
	if err != nil {
		t.Fatal(err)
	}
	return head[1], bnd
}

func tcpEcho(t *testing.T) Addr {
	t.Helper()
	ln, err := net.Listen("tcp", loopbackAnyPort)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(c, c); _ = c.Close() }()
		}
	}()
	a := ln.Addr().(*net.TCPAddr)
	return Addr{IP: a.IP, Port: uint16(a.Port)}
}

func udpEcho(t *testing.T) Addr {
	t.Helper()
	pc, err := net.ListenPacket("udp", loopbackAnyPort)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(buf[:n], from)
		}
	}()
	a := pc.LocalAddr().(*net.UDPAddr)
	return Addr{IP: a.IP, Port: uint16(a.Port)}
}

func TestConnectAuth(t *testing.T) {
	s := startServer(t, false, "")
	c, ok := handshake(t, s.tcp, userAlice, passSecret)
	if !ok {
		t.Fatal("authentication failed")
	}
	if rep, _ := request(t, c, CmdConnect, tcpEcho(t)); rep != RepSucceeded {
		t.Fatalf(replyFmt, rep)
	}
	_, _ = c.Write([]byte("hello"))
	buf := make([]byte, 5)
	if _, err := io.ReadFull(c, buf); err != nil || string(buf) != "hello" {
		t.Fatalf("echo = %q, %v", buf, err)
	}
}

func TestConnectRefused(t *testing.T) {
	s := startServer(t, false, "")
	c, _ := handshake(t, s.tcp, userAlice, passSecret)
	dead := tcpEcho(t)
	ln, _ := net.Listen("tcp", loopbackAnyPort)
	dead.Port = uint16(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()
	if rep, _ := request(t, c, CmdConnect, dead); rep != RepConnectionRefused && rep != RepHostUnreachable {
		t.Errorf(replyFmt, rep)
	}
}

func TestConnectWrongPassword(t *testing.T) {
	s := startServer(t, false, "")
	c, ok := handshake(t, s.tcp, userAlice, "nope")
	if ok {
		t.Fatal("wrong password accepted")
	}
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Error("connection left open")
	}
}

func TestConnectNoRight(t *testing.T) {
	s := startServer(t, false, "")
	if _, ok := handshake(t, s.tcp, "bob", passSecret); ok {
		t.Error("user without the socks right accepted")
	}
}

func TestNoAuthRefusedWhenNotAnonymous(t *testing.T) {
	s := startServer(t, false, "")
	if _, ok := handshake(t, s.tcp, "", ""); ok {
		t.Error("anonymous client accepted")
	}
}

func TestConnectAnonymous(t *testing.T) {
	s := startServer(t, true, "")
	c, ok := handshake(t, s.tcp, "", "")
	if !ok {
		t.Fatal("anonymous client refused")
	}
	if rep, _ := request(t, c, CmdConnect, tcpEcho(t)); rep != RepSucceeded {
		t.Fatalf(replyFmt, rep)
	}
}

func TestBindNotSupported(t *testing.T) {
	s := startServer(t, false, "")
	c, _ := handshake(t, s.tcp, userAlice, passSecret)
	if rep, _ := request(t, c, CmdBind, Addr{IP: net.IPv4zero}); rep != RepCommandNotSupported {
		t.Errorf(replyFmt, rep)
	}
}

// udpExchange sends one datagram through the relay from pc and returns the reply datagram (or an error on timeout).
func udpExchange(pc net.PacketConn, relay net.Addr, dst Addr, payload string) (Addr, string, error) {
	if _, err := pc.WriteTo(append(AppendUDPHeader(nil, dst), payload...), relay); err != nil {
		return Addr{}, "", err
	}
	_ = pc.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 2048)
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		return Addr{}, "", err
	}
	_, from, data, err := ParseUDP(buf[:n])
	return from, string(data), err
}

func associate(t *testing.T, s testServer) (net.Conn, *net.UDPAddr) {
	t.Helper()
	c, _ := handshake(t, s.tcp, userAlice, passSecret)
	rep, bnd := request(t, c, CmdUDPAssociate, Addr{IP: net.IPv4zero})
	if rep != RepSucceeded {
		t.Fatalf(replyFmt, rep)
	}
	if int(bnd.Port) != s.udp.Port {
		t.Fatalf("bnd port = %d, want %d", bnd.Port, s.udp.Port)
	}
	return c, s.udp
}

func TestUDPEcho(t *testing.T) {
	s := startServer(t, false, "")
	_, relay := associate(t, s)
	echo := udpEcho(t)
	pc, _ := net.ListenPacket("udp", loopbackAnyPort)
	defer func() { _ = pc.Close() }()
	from, data, err := udpExchange(pc, relay, echo, "ping")
	if err != nil || data != "ping" || from.String() != echo.String() {
		t.Fatalf("from=%s data=%q err=%v", from, data, err)
	}
}

func TestUDPLearnsPort(t *testing.T) {
	s := startServer(t, false, "")
	_, relay := associate(t, s) // declared 0.0.0.0:0
	echo := udpEcho(t)
	first, _ := net.ListenPacket("udp", loopbackAnyPort)
	defer func() { _ = first.Close() }()
	if _, _, err := udpExchange(first, relay, echo, "a"); err != nil {
		t.Fatal(err)
	}
	other, _ := net.ListenPacket("udp", loopbackAnyPort)
	defer func() { _ = other.Close() }()
	if _, _, err := udpExchange(other, relay, echo, "b"); err == nil {
		t.Error("datagram from a second port of the same client accepted after the port was learned")
	}
}

func TestUDPOtherIPDropped(t *testing.T) {
	s := startServer(t, false, "")
	_, relay := associate(t, s)
	pc, err := net.ListenPacket("udp", "127.0.0.2:0")
	if err != nil {
		t.Skip("127.0.0.2 not available:", err)
	}
	defer func() { _ = pc.Close() }()
	if _, _, err := udpExchange(pc, relay, udpEcho(t), "x"); err == nil {
		t.Error("datagram from another IP relayed")
	}
}

func TestUDPAssociationEndsWithTCP(t *testing.T) {
	s := startServer(t, false, "")
	c, relay := associate(t, s)
	_ = c.Close()
	time.Sleep(100 * time.Millisecond)
	pc, _ := net.ListenPacket("udp", loopbackAnyPort)
	defer func() { _ = pc.Close() }()
	if _, _, err := udpExchange(pc, relay, udpEcho(t), "x"); err == nil {
		t.Error("datagram relayed after the control connection closed")
	}
}

func TestUDPFragmentDropped(t *testing.T) {
	s := startServer(t, false, "")
	_, relay := associate(t, s)
	pc, _ := net.ListenPacket("udp", loopbackAnyPort)
	defer func() { _ = pc.Close() }()
	pkt := AppendUDPHeader(nil, udpEcho(t))
	pkt[2] = 1
	_, _ = pc.WriteTo(append(pkt, 'x'), relay)
	_ = pc.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, _, err := pc.ReadFrom(make([]byte, 64)); err == nil {
		t.Error("fragmented datagram relayed")
	}
}

func TestAdvertiseAddress(t *testing.T) {
	s := startServer(t, false, "203.0.113.7")
	c, _ := handshake(t, s.tcp, userAlice, passSecret)
	_, bnd := request(t, c, CmdUDPAssociate, Addr{IP: net.IPv4zero})
	if bnd.IP.String() != "203.0.113.7" {
		t.Errorf("bnd = %s", bnd)
	}
}

func TestAdvertiseUnresolvable(t *testing.T) {
	s := startServer(t, false, "does-not-exist.invalid")
	c, _ := handshake(t, s.tcp, userAlice, passSecret)
	_, bnd := request(t, c, CmdUDPAssociate, Addr{IP: net.IPv4zero})
	if !bnd.IP.Equal(net.IPv4zero) {
		t.Errorf("bnd = %s, want 0.0.0.0", bnd)
	}
}

func TestLogsConnections(t *testing.T) {
	s := startServer(t, false, "")
	c, _ := handshake(t, s.tcp, userAlice, passSecret)
	echo := tcpEcho(t)
	request(t, c, CmdConnect, echo)
	_ = c.Close()
	handshake(t, s.tcp, userAlice, "nope")
	time.Sleep(100 * time.Millisecond)
	logs := s.log.String()
	if !strings.Contains(logs, "user=alice") || !strings.Contains(logs, "dst="+echo.String()) ||
		!strings.Contains(logs, "auth failed") {
		t.Errorf("logs:\n%s", logs)
	}
}

func TestBndPortEncoding(t *testing.T) {
	// Guards the big-endian port encoding used in replies.
	b := appendAddr(nil, Addr{IP: net.IPv4(1, 2, 3, 4), Port: 0x1234})
	if binary.BigEndian.Uint16(b[len(b)-2:]) != 0x1234 {
		t.Errorf("port bytes = %v", b[len(b)-2:])
	}
}

// lockedWriter is a log sink that tests can read while the server writes to it.
type lockedWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedWriter) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func TestUnknownAddressType(t *testing.T) {
	s := startServer(t, false, "")
	c, _ := handshake(t, s.tcp, userAlice, passSecret)
	_, _ = c.Write([]byte{5, 1, 0, 9})
	reply := make([]byte, 2)
	if _, err := io.ReadFull(c, reply); err != nil || reply[1] != RepAddressNotSupported {
		t.Errorf("reply = %v, %v", reply, err)
	}
}

func TestConnectUnresolvable(t *testing.T) {
	s := startServer(t, false, "")
	c, _ := handshake(t, s.tcp, userAlice, passSecret)
	if rep, _ := request(t, c, CmdConnect, Addr{Host: "does-not-exist.invalid", Port: 80}); rep != RepHostUnreachable {
		t.Errorf(replyFmt, rep)
	}
}

func TestCustomDial(t *testing.T) {
	c := config.Defaults()
	c.SOCKS.Anonymous = true
	a := auth.New(c)
	ln, _ := net.Listen("tcp", loopbackAnyPort)
	t.Cleanup(func() { _ = ln.Close() })
	dialed := make(chan string, 1)
	s := &Server{
		Auth: func() *auth.Authenticator { return a }, Advertise: func() string { return "" },
		Log: slog.New(slog.DiscardHandler),
		Dial: func(_ context.Context, _, addr string) (net.Conn, error) {
			dialed <- addr
			return nil, errors.New("refused by test")
		},
	}
	go func() { _ = s.Serve(ln, nil) }()
	conn, _ := handshake(t, ln.Addr().String(), "", "")
	if rep, _ := request(t, conn, CmdConnect, Addr{Host: "example.com", Port: 443}); rep != RepHostUnreachable {
		t.Errorf(replyFmt, rep)
	}
	if got := <-dialed; got != "example.com:443" {
		t.Errorf("dialed %q", got)
	}
	// Without a UDP socket, UDP ASSOCIATE is refused.
	conn2, _ := handshake(t, ln.Addr().String(), "", "")
	if rep, _ := request(t, conn2, CmdUDPAssociate, Addr{IP: net.IPv4zero}); rep != RepCommandNotSupported {
		t.Errorf("udp reply = %#x", rep)
	}
}

// TestUDPAssociationsOfOneIPStayApart: two clients on one IP (one association per flow, like tun2socks) whose first
// datagrams arrive in the other order must not swap associations, or closing one would kill the other's flow.
func TestUDPAssociationsOfOneIPStayApart(t *testing.T) {
	s := startServer(t, false, "")
	echo := udpEcho(t)
	pcA, _ := net.ListenPacket("udp", loopbackAnyPort)
	pcB, _ := net.ListenPacket("udp", loopbackAnyPort)
	defer func() { _ = pcA.Close(); _ = pcB.Close() }()
	declare := func(pc net.PacketConn) net.Conn {
		c, _ := handshake(t, s.tcp, userAlice, passSecret)
		a := pc.LocalAddr().(*net.UDPAddr)
		if rep, _ := request(t, c, CmdUDPAssociate, Addr{IP: a.IP, Port: uint16(a.Port)}); rep != RepSucceeded {
			t.Fatalf(replyFmt, rep)
		}
		return c
	}
	ctrlA := declare(pcA)
	declare(pcB)
	if _, _, err := udpExchange(pcB, s.udp, echo, "b1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := udpExchange(pcA, s.udp, echo, "a1"); err != nil {
		t.Fatal(err)
	}
	_ = ctrlA.Close()
	time.Sleep(100 * time.Millisecond)
	if _, data, err := udpExchange(pcB, s.udp, echo, "b2"); err != nil || data != "b2" {
		t.Errorf("B's flow died with A's association: %q, %v", data, err)
	}
}
