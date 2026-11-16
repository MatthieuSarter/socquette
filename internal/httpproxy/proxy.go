// Package httpproxy implements an authenticated HTTP forward proxy: CONNECT tunnels and plain HTTP forwarding.
package httpproxy

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/MatthieuSarter/socquette/internal/auth"
	"github.com/MatthieuSarter/socquette/internal/relay"
)

const dialTimeout = 30 * time.Second

// ParseProxyAuth decodes a "Basic base64(name:password)" Proxy-Authorization header value.
func ParseProxyAuth(header string) (name, password string, ok bool) {
	scheme, encoded, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Basic") {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", "", false
	}
	name, password, ok = strings.Cut(string(decoded), ":")
	if !ok {
		return "", "", false
	}
	return name, password, true
}

var hopByHop = []string{
	"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer",
	"Transfer-Encoding", "Upgrade",
}

// RemoveHopByHop deletes the headers that apply to one connection only, including those listed in Connection.
func RemoveHopByHop(h http.Header) {
	for _, v := range h.Values("Connection") {
		for _, name := range strings.Split(v, ",") {
			h.Del(strings.TrimSpace(name))
		}
	}
	for _, name := range hopByHop {
		h.Del(name)
	}
}

// defaultTransport never uses an upstream proxy, whatever HTTP_PROXY says.
var defaultTransport = &http.Transport{
	Proxy:                 nil,
	DialContext:           (&net.Dialer{Timeout: dialTimeout}).DialContext,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: time.Second,
}

// Handler is the proxy. Auth is called on every request, so the caller can swap users on a configuration reload.
type Handler struct {
	Auth      func() *auth.Authenticator
	Log       *slog.Logger
	Transport http.RoundTripper // nil means a transport with no upstream proxy
}

// authorize returns the user name (a trusted network's name, "-" when anonymous) and whether the request may go on.
// The client address is the TCP peer's: behind a reverse proxy, that is the reverse proxy's address.
func (h *Handler) authorize(r *http.Request) (string, bool) {
	a := h.Auth()
	header := r.Header.Get("Proxy-Authorization")
	if header == "" {
		client, _ := netip.ParseAddrPort(r.RemoteAddr) // an invalid address matches no network
		return a.Unauthenticated(auth.HTTP, client.Addr())
	}
	name, password, ok := ParseProxyAuth(header)
	return name, ok && a.Check(auth.HTTP, name, password)
}

// ServeHTTP proxies one request.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := h.Log.With("proxy", "http", "client", r.RemoteAddr)
	if r.Method != http.MethodConnect && !r.URL.IsAbs() {
		http.Error(w, "this is a proxy, not a web server", http.StatusBadRequest)
		return
	}
	user, ok := h.authorize(r)
	if !ok {
		log.Warn("http auth failed", "user", user)
		w.Header().Set("Proxy-Authenticate", `Basic realm="Socquette"`)
		http.Error(w, "proxy authentication required", http.StatusProxyAuthRequired)
		return
	}
	log = log.With("user", user)
	if r.Method == http.MethodConnect {
		h.tunnel(w, r, log.With("dst", r.Host))
		return
	}
	h.forward(w, r, log.With("dst", r.URL.Host))
}

func (h *Handler) tunnel(w http.ResponseWriter, r *http.Request, log *slog.Logger) {
	ctx, cancel := context.WithTimeout(r.Context(), dialTimeout)
	var d net.Dialer
	out, err := d.DialContext(ctx, "tcp", r.Host)
	cancel()
	if err != nil {
		log.Info("http connect", "result", "failed", "error", err)
		http.Error(w, "cannot reach "+r.Host, http.StatusBadGateway)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		_ = out.Close()
		http.Error(w, "tunneling not supported", http.StatusInternalServerError)
		return
	}
	client, brw, err := hj.Hijack()
	if err != nil {
		_ = out.Close()
		return
	}
	_ = client.SetDeadline(time.Time{})
	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
		_ = client.Close()
		_ = out.Close()
		return
	}
	// Bytes the client sent right after its request (e.g. a TLS ClientHello) may already sit in the server's buffer.
	if n := brw.Reader.Buffered(); n > 0 { //nolint:staticcheck // Explicit: the read side.
		early, _ := brw.Peek(n)
		if _, err := out.Write(early); err != nil {
			_ = client.Close()
			_ = out.Close()
			return
		}
	}
	log.Info("http connect", "result", "ok")
	relay.Pipe(client, out)
}

func (h *Handler) forward(w http.ResponseWriter, r *http.Request, log *slog.Logger) {
	transport := h.Transport
	if transport == nil {
		transport = defaultTransport
	}
	out := r.Clone(r.Context())
	out.RequestURI = ""
	RemoveHopByHop(out.Header)
	resp, err := transport.RoundTrip(out)
	if err != nil {
		log.Info("http request", "method", r.Method, "result", "failed", "error", err)
		http.Error(w, "cannot reach "+r.URL.Host, http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	RemoveHopByHop(resp.Header)
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
	log.Info("http request", "method", r.Method, "result", resp.StatusCode)
}
