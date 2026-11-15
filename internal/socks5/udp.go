package socks5

import (
	"errors"
	"log/slog"
	"net"
	"sync"
)

// maxDatagram is the largest UDP payload.
const maxDatagram = 65535

// association is one UDP ASSOCIATE: the client it belongs to, and the socket its datagrams leave from.
type association struct {
	ip       net.IP
	declared uint16       // UDP port the client declared, 0 if none
	client   *net.UDPAddr // nil until the client's first datagram tells its port
	out      net.PacketConn
}

// udpRelay relays the datagrams of every association through one shared client-facing socket, so a server behind a
// NAT needs a single forwarded UDP port.
type udpRelay struct {
	pc  net.PacketConn
	log *slog.Logger

	mu     sync.Mutex
	assocs []*association
}

func newUDPRelay(pc net.PacketConn, log *slog.Logger) *udpRelay {
	return &udpRelay{pc: pc, log: log.With("proxy", "socks")}
}

// add registers an association for a client IP and the UDP port it declared (0 if none). The port the server really
// sees is learned from the first datagram, since a NAT usually rewrites the declared one.
func (u *udpRelay) add(ip net.IP, declared uint16) (*association, error) {
	out, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return nil, err
	}
	a := &association{ip: ip, declared: declared, out: out}
	u.mu.Lock()
	u.assocs = append(u.assocs, a)
	u.mu.Unlock()
	go u.replies(a)
	return a, nil
}

func (u *udpRelay) remove(a *association) {
	u.mu.Lock()
	for i, x := range u.assocs {
		if x == a {
			u.assocs = append(u.assocs[:i], u.assocs[i+1:]...)
			break
		}
	}
	u.mu.Unlock()
	_ = a.out.Close()
}

// lookup finds the association of a datagram's source: the one that learned this exact address, else a pending one
// (port not learned yet) of the same IP — preferably the one that declared this port, then one that declared none,
// then any — which learns the address now. Nil means the datagram is dropped.
func (u *udpRelay) lookup(from *net.UDPAddr) *association {
	u.mu.Lock()
	defer u.mu.Unlock()
	var best *association
	rank := 0
	for _, a := range u.assocs {
		if !a.ip.Equal(from.IP) {
			continue
		}
		if a.client != nil {
			if a.client.Port == from.Port {
				return a
			}
			continue
		}
		r := 1
		switch int(a.declared) {
		case from.Port:
			r = 3
		case 0:
			r = 2
		}
		if r > rank {
			best, rank = a, r
		}
	}
	if best != nil {
		best.client = from
	}
	return best
}

// serve reads client datagrams until the socket is closed.
func (u *udpRelay) serve() {
	buf := make([]byte, maxDatagram)
	for {
		n, src, err := u.pc.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		from, ok := src.(*net.UDPAddr)
		if !ok {
			continue
		}
		a := u.lookup(from)
		if a == nil {
			continue
		}
		frag, dst, payload, err := ParseUDP(buf[:n])
		if err != nil || frag != 0 {
			continue
		}
		// ponytail: host names are resolved inline, which stalls the relay during a slow DNS lookup; resolve in a
		// goroutine (or cache per association) if that ever shows.
		target, err := net.ResolveUDPAddr("udp", dst.String())
		if err != nil {
			u.log.Debug("socks udp resolve failed", "dst", dst.String(), "error", err)
			continue
		}
		_, _ = a.out.WriteTo(payload, target)
	}
}

// replies sends the datagrams that targets send back to an association's client, until the association is removed.
func (u *udpRelay) replies(a *association) {
	buf := make([]byte, maxDatagram)
	for {
		n, src, err := a.out.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		from, ok := src.(*net.UDPAddr)
		u.mu.Lock()
		client := a.client
		u.mu.Unlock()
		if !ok || client == nil {
			continue
		}
		pkt := AppendUDPHeader(make([]byte, 0, n+22), Addr{IP: from.IP, Port: uint16(from.Port)}) //nolint:gosec
		_, _ = u.pc.WriteTo(append(pkt, buf[:n]...), client)
	}
}
