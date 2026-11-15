package httpproxy

import (
	"bufio"
	"encoding/base64"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/MatthieuSarter/socquette/internal/auth"
	"github.com/MatthieuSarter/socquette/internal/config"
)

const (
	loopbackAnyPort       = "127.0.0.1:0"
	statusFmt             = "status = %d"
	hdrProxyAuthenticate  = "Proxy-Authenticate"
	hdrProxyAuthorization = "Proxy-Authorization"
)

var testHash, _ = auth.HashPassword("secret")

func basic(user, pw string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pw))
}

func TestParseProxyAuth(t *testing.T) {
	tests := []struct {
		header   string
		user, pw string
		ok       bool
	}{
		{basic("alice", "se:cret"), "alice", "se:cret", true},
		{"basic " + base64.StdEncoding.EncodeToString([]byte("a:b")), "a", "b", true},
		{"Basic !!!", "", "", false},
		{"Basic " + base64.StdEncoding.EncodeToString([]byte("nocolon")), "", "", false},
		{"Bearer abc", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		user, pw, ok := ParseProxyAuth(tt.header)
		if user != tt.user || pw != tt.pw || ok != tt.ok {
			t.Errorf("ParseProxyAuth(%q) = %q, %q, %v", tt.header, user, pw, ok)
		}
	}
}

func TestRemoveHopByHop(t *testing.T) {
	h := http.Header{}
	for _, k := range []string{"Connection", "Proxy-Connection", "Keep-Alive", hdrProxyAuthorization, "Te", "Trailer",
		"Transfer-Encoding", "Upgrade", hdrProxyAuthenticate, "X-Custom", "X-Keep"} {
		h.Set(k, "v")
	}
	h.Set("Connection", "close, X-Custom")
	RemoveHopByHop(h)
	if len(h) != 1 || h.Get("X-Keep") != "v" {
		t.Errorf("left %v", h)
	}
}

func startProxy(t *testing.T, anonymous bool) *url.URL {
	t.Helper()
	c := config.Defaults()
	c.HTTP.Anonymous = anonymous
	c.Users = []config.User{
		{Name: "alice", PasswordHash: testHash, HTTP: true},
		{Name: "bob", PasswordHash: testHash, SOCKS: true},
	}
	a := auth.New(c)
	h := &Handler{Auth: func() *auth.Authenticator { return a }, Log: slog.New(slog.DiscardHandler)}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	return u
}

func origin(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seen-Proxy-Auth", r.Header.Get(hdrProxyAuthorization))
		_, _ = io.WriteString(w, "hello from origin")
	}))
	t.Cleanup(srv.Close)
	return srv
}

// get sends a GET through the proxy with the given Proxy-Authorization header ("" for none).
func get(t *testing.T, proxy *url.URL, target, proxyAuth string) *http.Response {
	t.Helper()
	conn, err := net.Dial("tcp", proxy.Host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	req, _ := http.NewRequest(http.MethodGet, target, nil)
	if proxyAuth != "" {
		req.Header.Set(hdrProxyAuthorization, proxyAuth)
	}
	if err := req.WriteProxy(conn); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestHTTPForward(t *testing.T) {
	p, o := startProxy(t, false), origin(t)
	resp := get(t, p, o.URL+"/x", basic("alice", "secret"))
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "hello from origin" {
		t.Fatalf("status=%d body=%q", resp.StatusCode, body)
	}
	if resp.Header.Get("X-Seen-Proxy-Auth") != "" {
		t.Error("Proxy-Authorization leaked to the origin")
	}
}

func TestHTTPBadGateway(t *testing.T) {
	p := startProxy(t, false)
	ln, _ := net.Listen("tcp", loopbackAnyPort)
	dead := ln.Addr().String()
	_ = ln.Close()
	if resp := get(t, p, "http://"+dead+"/", basic("alice", "secret")); resp.StatusCode != http.StatusBadGateway {
		t.Errorf(statusFmt, resp.StatusCode)
	}
}

func connect(t *testing.T, proxy *url.URL, target, proxyAuth string) (net.Conn, *bufio.Reader, int) {
	t.Helper()
	conn, err := net.Dial("tcp", proxy.Host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	msg := "CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n"
	if proxyAuth != "" {
		msg += "Proxy-Authorization: " + proxyAuth + "\r\n"
	}
	// The early "ping" checks that bytes sent right after the request headers are not lost.
	if _, err := io.WriteString(conn, msg+"\r\nping"); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatal(err)
	}
	return conn, br, resp.StatusCode
}

func tcpEcho(t *testing.T) string {
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
	return ln.Addr().String()
}

func TestHTTPConnect(t *testing.T) {
	p := startProxy(t, false)
	conn, br, status := connect(t, p, tcpEcho(t), basic("alice", "secret"))
	if status != http.StatusOK {
		t.Fatalf(statusFmt, status)
	}
	_, _ = io.WriteString(conn, "pong")
	buf := make([]byte, 8)
	if _, err := io.ReadFull(br, buf); err != nil || string(buf) != "pingpong" {
		t.Fatalf("echo = %q, %v", buf, err)
	}
}

func TestHTTPNoCreds407(t *testing.T) {
	p, o := startProxy(t, false), origin(t)
	resp := get(t, p, o.URL, "")
	if resp.StatusCode != http.StatusProxyAuthRequired ||
		resp.Header.Get(hdrProxyAuthenticate) != `Basic realm="Socquette"` {
		t.Errorf("status=%d header=%q", resp.StatusCode, resp.Header.Get(hdrProxyAuthenticate))
	}
	if _, _, status := connect(t, p, tcpEcho(t), ""); status != http.StatusProxyAuthRequired {
		t.Errorf("CONNECT status = %d", status)
	}
}

func TestHTTPAnonymous(t *testing.T) {
	p, o := startProxy(t, true), origin(t)
	if resp := get(t, p, o.URL, ""); resp.StatusCode != http.StatusOK {
		t.Errorf(statusFmt, resp.StatusCode)
	}
}

func TestHTTPAnonymousWrongCreds407(t *testing.T) {
	p, o := startProxy(t, true), origin(t)
	if resp := get(t, p, o.URL, basic("alice", "nope")); resp.StatusCode != http.StatusProxyAuthRequired {
		t.Errorf(statusFmt, resp.StatusCode)
	}
}

func TestHTTPNoRight407(t *testing.T) {
	p, o := startProxy(t, false), origin(t)
	if resp := get(t, p, o.URL, basic("bob", "secret")); resp.StatusCode != http.StatusProxyAuthRequired {
		t.Errorf(statusFmt, resp.StatusCode)
	}
}

func TestHTTPOriginForm400(t *testing.T) {
	p := startProxy(t, false)
	resp, err := http.Get(p.String() + "/") //nolint:noctx // Test request.
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf(statusFmt, resp.StatusCode)
	}
}

func TestHTTPConnectBadGateway(t *testing.T) {
	p := startProxy(t, false)
	ln, _ := net.Listen("tcp", loopbackAnyPort)
	dead := ln.Addr().String()
	_ = ln.Close()
	if _, _, status := connect(t, p, dead, basic("alice", "secret")); status != http.StatusBadGateway {
		t.Errorf(statusFmt, status)
	}
}
