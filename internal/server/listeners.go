package server

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/MatthieuSarter/socquette/internal/socks5"
)

// binding is a bound proxy listener.
type binding interface {
	// Close stops accepting new clients; clients already connected carry on.
	Close() error
	// Shutdown closes everything, connected clients included where possible.
	Shutdown()
}

// slot is one proxy: what it is bound to, and the memory of its last failure.
type slot struct {
	name    string
	bind    func(addr string) (binding, error)
	addr    string
	current binding
	retired []binding // closed bindings whose clients may still be connected, shut down on exit
	tried   string    // wanted address of the last failed attempt
	lastErr string
}

func (s *slot) shutdown() {
	if s.current != nil {
		s.current.Shutdown()
	}
	for _, b := range s.retired {
		b.Shutdown()
	}
	s.current, s.addr, s.retired = nil, "", nil
}

func (s *slot) release() {
	if s.current == nil {
		return
	}
	_ = s.current.Close()
	s.retired = append(s.retired, s.current)
	s.current, s.addr = nil, ""
}

func samePort(a, b string) bool {
	_, pa, errA := net.SplitHostPort(a)
	_, pb, errB := net.SplitHostPort(b)
	return errA == nil && errB == nil && pa == pb
}

// reconcile binds s to want ("" = closed). The new address is bound before the old one is closed. When that fails
// on the same port (Windows refuses 127.0.0.1:P while 0.0.0.0:P is bound), the first attempt for that address
// closes the old listener and retries, restoring the old address if the retry fails too. Later attempts for the same
// address only try the non-disruptive way, so a lasting failure doesn't flap the working listener every poll.
func (r *runner) reconcile(s *slot, want string) {
	if want == s.addr {
		s.tried, s.lastErr = "", ""
		return
	}
	if want == "" {
		s.release()
		r.log.Info("proxy stopped", "proxy", s.name)
		return
	}
	b, err := s.bind(want)
	if err != nil && s.current != nil && want != s.tried && samePort(want, s.addr) {
		old := s.addr
		s.release()
		if b, err = s.bind(want); err != nil {
			if back, err2 := s.bind(old); err2 == nil {
				s.current, s.addr = back, old
			} else {
				err = errors.Join(err, err2)
			}
		}
	}
	if err != nil {
		s.tried = want
		if err.Error() != s.lastErr {
			s.lastErr = err.Error()
			r.log.Error("cannot listen", "proxy", s.name, "address", want, "error", err, "kept", s.addr)
		}
		return
	}
	s.release()
	s.current, s.addr, s.tried, s.lastErr = b, want, "", ""
	r.log.Info("proxy listening", "proxy", s.name, "address", want)
}

type httpBinding struct {
	ln  net.Listener
	srv *http.Server
}

func (b *httpBinding) Close() error { return b.ln.Close() }
func (b *httpBinding) Shutdown()    { _ = b.srv.Close() }

func newHTTPBinder(h http.Handler) func(string) (binding, error) {
	return func(addr string) (binding, error) {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return nil, err
		}
		srv := &http.Server{
			Handler:           h,
			ReadHeaderTimeout: 30 * time.Second,
			IdleTimeout:       2 * time.Minute,
			ErrorLog:          slog.NewLogLogger(slog.DiscardHandler, slog.LevelError),
		}
		go func() { _ = srv.Serve(ln) }()
		return &httpBinding{ln: ln, srv: srv}, nil
	}
}

type socksBinding struct {
	ln net.Listener
	pc net.PacketConn
}

// Close also closes the UDP socket, which ends the UDP relaying of the old address's associations.
// ponytail: UDP flows are cut on a SOCKS rebind; keep the old socket until its associations end if that matters.
func (b *socksBinding) Close() error { return errors.Join(b.ln.Close(), b.pc.Close()) }
func (b *socksBinding) Shutdown()    { _ = b.Close() }

func socksBinder(s *socks5.Server) func(string) (binding, error) {
	return func(addr string) (binding, error) {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return nil, err
		}
		pc, err := net.ListenPacket("udp", addr)
		if err != nil {
			_ = ln.Close()
			return nil, err
		}
		go func() { _ = s.Serve(ln, pc) }()
		return &socksBinding{ln: ln, pc: pc}, nil
	}
}
