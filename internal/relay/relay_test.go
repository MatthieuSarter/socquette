package relay

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func tcpPair(t *testing.T) (client, server net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	accepted := make(chan net.Conn)
	go func() {
		c, _ := ln.Accept()
		accepted <- c
	}()
	client, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return client, <-accepted
}

func TestPipe(t *testing.T) {
	left, a := tcpPair(t)
	b, right := tcpPair(t)
	done := make(chan struct{})
	go func() { Pipe(a, b); close(done) }()

	if _, err := left.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(right, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("right got %q, %v", buf, err)
	}
	if _, err := right.Write([]byte("pong")); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(left, buf); err != nil || string(buf) != "pong" {
		t.Fatalf("left got %q, %v", buf, err)
	}

	_ = left.Close()
	if _, err := right.Read(buf); !errors.Is(err, io.EOF) {
		t.Errorf("right read after left close: %v, want EOF", err)
	}
	_ = right.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Pipe did not return")
	}
}
