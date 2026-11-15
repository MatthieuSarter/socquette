// Package relay copies data between the two ends of a proxied TCP connection.
package relay

import (
	"io"
	"net"
	"sync"
)

type closeWriter interface{ CloseWrite() error }

// Pipe copies a to b and b to a until both directions end, half-closing each side when its source is done, then
// closes both connections.
func Pipe(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	cp := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if cw, ok := dst.(closeWriter); ok {
			_ = cw.CloseWrite()
		} else {
			_ = dst.Close()
		}
	}
	go cp(a, b)
	go cp(b, a)
	wg.Wait()
	_ = a.Close()
	_ = b.Close()
}
