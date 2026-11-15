package socks5

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"syscall"
	"time"

	"github.com/MatthieuSarter/socquette/internal/auth"
	"github.com/MatthieuSarter/socquette/internal/relay"
)

// handshakeTimeout bounds the greeting, authentication and request phase, and the outbound dial.
const handshakeTimeout = 30 * time.Second

// Server serves SOCKS5 clients. Its fields are read on every new connection, so the caller can swap the values they
// return on a configuration reload.
type Server struct {
	Auth      func() *auth.Authenticator // current users
	Advertise func() string              // current advertise_address ("" means 0.0.0.0)
	Log       *slog.Logger
	// Dial opens outbound TCP connections; nil means a net.Dialer.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
}

// Serve accepts SOCKS5 clients on ln and relays UDP ASSOCIATE datagrams on pc (nil disables UDP). It returns when ln
// is closed. The caller owns ln and pc and closes them.
func (s *Server) Serve(ln net.Listener, pc net.PacketConn) error {
	var udp *udpRelay
	if pc != nil {
		udp = newUDPRelay(pc, s.Log)
		go udp.serve()
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.handle(c, udp)
	}
}

func (s *Server) dial(ctx context.Context, addr string) (net.Conn, error) {
	if s.Dial != nil {
		return s.Dial(ctx, "tcp", addr)
	}
	var d net.Dialer
	return d.DialContext(ctx, "tcp", addr)
}

// authenticate runs the method negotiation and, when chosen, RFC 1929; it returns the user name ("-" when
// anonymous) and whether the client may go on.
func (s *Server) authenticate(c net.Conn, log *slog.Logger) (string, bool) {
	methods, err := ReadGreeting(c)
	if err != nil {
		log.Debug("socks greeting failed", "error", err)
		return "", false
	}
	a := s.Auth()
	method := ChooseMethod(methods, a.Anonymous(auth.SOCKS), a.HasUsers(auth.SOCKS))
	if _, err := c.Write([]byte{version, method}); err != nil || method == MethodNone {
		if method == MethodNone {
			log.Warn("socks auth failed", "reason", "no acceptable method")
		}
		return "", false
	}
	if method == MethodNoAuth {
		return "-", true
	}
	name, password, err := ReadUserPass(c)
	if err != nil {
		log.Debug("socks auth read failed", "error", err)
		return "", false
	}
	ok := a.Check(auth.SOCKS, name, password)
	if !ok {
		log.Warn("socks auth failed", "user", name)
	}
	return name, WriteUserPassStatus(c, ok) == nil && ok
}

func (s *Server) handle(c net.Conn, udp *udpRelay) {
	defer func() { _ = c.Close() }()
	log := s.Log.With("proxy", "socks", "client", c.RemoteAddr().String())
	_ = c.SetDeadline(time.Now().Add(handshakeTimeout))
	user, ok := s.authenticate(c, log)
	if !ok {
		return
	}
	log = log.With("user", user)
	req, err := ReadRequest(c)
	if err != nil {
		if errors.Is(err, ErrAddressType) {
			_ = WriteReply(c, RepAddressNotSupported, Addr{})
		}
		log.Debug("socks request failed", "error", err)
		return
	}
	switch {
	case req.Cmd == CmdConnect:
		s.connect(c, req.Dst, log)
	case req.Cmd == CmdUDPAssociate && udp != nil:
		s.associate(c, udp, req.Dst.Port, log)
	default:
		_ = WriteReply(c, RepCommandNotSupported, Addr{})
		log.Info("socks command refused", "command", req.Cmd)
	}
}

func replyCode(err error) byte {
	var dnsErr *net.DNSError
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return RepConnectionRefused
	case errors.As(err, &dnsErr):
		return RepHostUnreachable
	case errors.Is(err, syscall.ENETUNREACH):
		return RepNetworkUnreachable
	default:
		return RepHostUnreachable
	}
}

func (s *Server) connect(c net.Conn, dst Addr, log *slog.Logger) {
	log = log.With("dst", dst.String())
	ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
	out, err := s.dial(ctx, dst.String())
	cancel()
	if err != nil {
		_ = WriteReply(c, replyCode(err), Addr{})
		log.Info("socks connect", "result", "failed", "error", err)
		return
	}
	local := out.LocalAddr().(*net.TCPAddr)
	if err := WriteReply(c, RepSucceeded, Addr{IP: local.IP, Port: uint16(local.Port)}); err != nil { //nolint:gosec
		_ = out.Close()
		return
	}
	_ = c.SetDeadline(time.Time{})
	log.Info("socks connect", "result", "ok")
	relay.Pipe(c, out)
}

// advertisedIP resolves the advertise address; a host name is resolved on every call, since home IPs change.
func (s *Server) advertisedIP(log *slog.Logger) net.IP {
	adv := s.Advertise()
	if adv == "" {
		return net.IPv4zero
	}
	if ip := net.ParseIP(adv); ip != nil {
		return ip
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, adv)
	if err != nil || len(ips) == 0 {
		log.Warn("cannot resolve the advertise address", "address", adv, "error", err)
		return net.IPv4zero
	}
	for _, ip := range ips {
		if ip.IP.To4() != nil {
			return ip.IP
		}
	}
	return ips[0].IP
}

func (s *Server) associate(c net.Conn, udp *udpRelay, declared uint16, log *slog.Logger) {
	clientIP := c.RemoteAddr().(*net.TCPAddr).IP
	a, err := udp.add(clientIP, declared)
	if err != nil {
		_ = WriteReply(c, RepGeneralFailure, Addr{})
		log.Warn("socks udp associate", "result", "failed", "error", err)
		return
	}
	defer udp.remove(a)
	port := uint16(udp.pc.LocalAddr().(*net.UDPAddr).Port) //nolint:gosec // A port always fits in 16 bits.
	if err := WriteReply(c, RepSucceeded, Addr{IP: s.advertisedIP(log), Port: port}); err != nil {
		return
	}
	_ = c.SetDeadline(time.Time{})
	log.Info("socks udp associate", "result", "ok")
	// The association lives as long as the control connection.
	_, _ = io.Copy(io.Discard, c)
}
