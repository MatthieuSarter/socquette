// Package socks5 implements a SOCKS5 server (RFC 1928) with username/password authentication (RFC 1929).
package socks5

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
)

// Protocol constants of RFC 1928 and RFC 1929.
const (
	version     byte = 0x05
	authVersion byte = 0x01

	MethodNoAuth   byte = 0x00
	MethodUserPass byte = 0x02
	MethodNone     byte = 0xFF

	CmdConnect      byte = 0x01
	CmdBind         byte = 0x02
	CmdUDPAssociate byte = 0x03

	RepSucceeded           byte = 0x00
	RepGeneralFailure      byte = 0x01
	RepNotAllowed          byte = 0x02
	RepNetworkUnreachable  byte = 0x03
	RepHostUnreachable     byte = 0x04
	RepConnectionRefused   byte = 0x05
	RepCommandNotSupported byte = 0x07
	RepAddressNotSupported byte = 0x08

	atypIPv4   byte = 0x01
	atypDomain byte = 0x03
	atypIPv6   byte = 0x04
)

// Protocol errors.
var (
	ErrVersion     = errors.New("socks5: unsupported protocol version")
	ErrAddressType = errors.New("socks5: unsupported address type")
)

// Addr is a SOCKS address: exactly one of IP and Host is set.
type Addr struct {
	IP   net.IP
	Host string
	Port uint16
}

// String returns the address as host:port.
func (a Addr) String() string {
	host := a.Host
	if a.IP != nil {
		host = a.IP.String()
	}
	return net.JoinHostPort(host, strconv.Itoa(int(a.Port)))
}

// ReadGreeting reads the client's method selection message and returns the offered methods.
func ReadGreeting(r io.Reader) ([]byte, error) {
	var head [2]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return nil, fmt.Errorf("socks5 greeting: %w", err)
	}
	if head[0] != version {
		return nil, ErrVersion
	}
	methods := make([]byte, head[1])
	if _, err := io.ReadFull(r, methods); err != nil {
		return nil, fmt.Errorf("socks5 greeting: %w", err)
	}
	return methods, nil
}

// ChooseMethod picks the authentication method: username/password when users exist and the client offers it, else
// no authentication when the client may go without credentials (anonymous proxy, trusted network) and offers it, else
// MethodNone.
func ChooseMethod(offered []byte, noAuth, haveUsers bool) byte {
	has := func(m byte) bool {
		for _, o := range offered {
			if o == m {
				return true
			}
		}
		return false
	}
	switch {
	case haveUsers && has(MethodUserPass):
		return MethodUserPass
	case noAuth && has(MethodNoAuth):
		return MethodNoAuth
	default:
		return MethodNone
	}
}

func readString(r io.Reader) (string, error) {
	var n [1]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return "", err
	}
	b := make([]byte, n[0])
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return string(b), nil
}

// ReadUserPass reads an RFC 1929 username/password request.
func ReadUserPass(r io.Reader) (name, password string, err error) {
	var ver [1]byte
	if _, err := io.ReadFull(r, ver[:]); err != nil {
		return "", "", fmt.Errorf("socks5 auth: %w", err)
	}
	if ver[0] != authVersion {
		return "", "", ErrVersion
	}
	if name, err = readString(r); err != nil {
		return "", "", fmt.Errorf("socks5 auth: %w", err)
	}
	if password, err = readString(r); err != nil {
		return "", "", fmt.Errorf("socks5 auth: %w", err)
	}
	return name, password, nil
}

// WriteUserPassStatus writes the RFC 1929 status reply.
func WriteUserPassStatus(w io.Writer, ok bool) error {
	status := byte(1)
	if ok {
		status = 0
	}
	_, err := w.Write([]byte{authVersion, status})
	return err
}

// Request is a client command and its destination.
type Request struct {
	Cmd byte
	Dst Addr
}

func readAddr(r io.Reader, atyp byte) (Addr, error) {
	var a Addr
	switch atyp {
	case atypIPv4, atypIPv6:
		size := net.IPv4len
		if atyp == atypIPv6 {
			size = net.IPv6len
		}
		ip := make(net.IP, size)
		if _, err := io.ReadFull(r, ip); err != nil {
			return a, err
		}
		a.IP = ip
	case atypDomain:
		host, err := readString(r)
		if err != nil {
			return a, err
		}
		a.Host = host
	default:
		return a, ErrAddressType
	}
	var port [2]byte
	if _, err := io.ReadFull(r, port[:]); err != nil {
		return a, err
	}
	a.Port = binary.BigEndian.Uint16(port[:])
	return a, nil
}

// ReadRequest reads a client request (CONNECT, BIND or UDP ASSOCIATE).
func ReadRequest(r io.Reader) (Request, error) {
	var head [4]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return Request{}, fmt.Errorf("socks5 request: %w", err)
	}
	if head[0] != version {
		return Request{}, ErrVersion
	}
	dst, err := readAddr(r, head[3])
	if err != nil {
		return Request{}, fmt.Errorf("socks5 request: %w", err)
	}
	return Request{Cmd: head[1], Dst: dst}, nil
}

func appendAddr(dst []byte, a Addr) []byte {
	switch {
	case a.IP.To4() != nil:
		dst = append(append(dst, atypIPv4), a.IP.To4()...)
	case a.IP != nil:
		dst = append(append(dst, atypIPv6), a.IP.To16()...)
	case a.Host != "":
		// Host names come from parsed requests, so they are at most 255 bytes long.
		dst = append(append(dst, atypDomain, byte(len(a.Host))), a.Host...) //nolint:gosec
	default:
		dst = append(dst, atypIPv4, 0, 0, 0, 0)
	}
	return binary.BigEndian.AppendUint16(dst, a.Port)
}

// WriteReply writes a server reply; a zero bnd is sent as 0.0.0.0:0.
func WriteReply(w io.Writer, rep byte, bnd Addr) error {
	_, err := w.Write(appendAddr([]byte{version, rep, 0}, bnd))
	return err
}

// ParseUDP splits a SOCKS UDP request datagram into its fragment number, destination and payload.
func ParseUDP(b []byte) (frag byte, dst Addr, payload []byte, err error) {
	if len(b) < 4 {
		return 0, Addr{}, nil, io.ErrUnexpectedEOF
	}
	r := &sliceReader{b: b[4:]}
	dst, err = readAddr(r, b[3])
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return 0, Addr{}, nil, err
	}
	return b[2], dst, r.b, nil
}

// AppendUDPHeader appends the SOCKS UDP header (no fragmentation) for a datagram coming from the given address.
func AppendUDPHeader(dst []byte, from Addr) []byte {
	return appendAddr(append(dst, 0, 0, 0), from)
}

// sliceReader is a bytes.Reader that exposes the unread remainder.
type sliceReader struct{ b []byte }

func (s *sliceReader) Read(p []byte) (int, error) {
	if len(s.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, s.b)
	s.b = s.b[n:]
	return n, nil
}
