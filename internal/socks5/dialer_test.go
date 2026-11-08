package socks5

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestDialerNegotiatesSOCKS5AndConnects(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		var greeting [2]byte
		if _, err := io.ReadFull(conn, greeting[:]); err != nil {
			serverErr <- err
			return
		}
		methods := make([]byte, int(greeting[1]))
		if _, err := io.ReadFull(conn, methods); err != nil {
			serverErr <- err
			return
		}
		if _, err := conn.Write([]byte{5, 0}); err != nil {
			serverErr <- err
			return
		}
		var head [4]byte
		if _, err := io.ReadFull(conn, head[:]); err != nil {
			serverErr <- err
			return
		}
		if head[0] != 5 || head[1] != 1 || head[3] != 3 {
			serverErr <- io.ErrUnexpectedEOF
			return
		}
		var n [1]byte
		if _, err := io.ReadFull(conn, n[:]); err != nil {
			serverErr <- err
			return
		}
		host := make([]byte, int(n[0]))
		if _, err := io.ReadFull(conn, host); err != nil {
			serverErr <- err
			return
		}
		var port [2]byte
		if _, err := io.ReadFull(conn, port[:]); err != nil {
			serverErr <- err
			return
		}
		if string(host) != "peer.example" || binary.BigEndian.Uint16(port[:]) != 443 {
			serverErr <- io.ErrUnexpectedEOF
			return
		}
		if _, err := conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0x01, 0xbb}); err != nil {
			serverErr <- err
			return
		}
		buffer := make([]byte, 4)
		if _, err := io.ReadFull(conn, buffer); err != nil {
			serverErr <- err
			return
		}
		_, err = conn.Write(buffer)
		serverErr <- err
	}()

	dialer := Dialer{ProxyAddress: listener.Addr().String(), Timeout: time.Second}
	conn, err := dialer.DialContext(context.Background(), "tcp", "peer.example:443")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 4)
	if _, err := io.ReadFull(conn, buffer); err != nil {
		t.Fatal(err)
	}
	if string(buffer) != "ping" {
		t.Fatalf("echo=%q", buffer)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

func TestDialerRejectsInvalidEndpoint(t *testing.T) {
	dialer := Dialer{ProxyAddress: "127.0.0.1:1", Timeout: 100 * time.Millisecond}
	if _, err := dialer.DialContext(context.Background(), "udp", "peer.example:443"); err == nil {
		t.Fatal("expected non-TCP network rejection")
	}
	if _, err := dialer.DialContext(context.Background(), "tcp", "not-an-endpoint"); err == nil {
		t.Fatal("expected invalid target rejection")
	}
}

func TestDialerSupportsXrayUsernamePasswordAuth(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		var greeting [2]byte
		if _, err := io.ReadFull(conn, greeting[:]); err != nil {
			serverErr <- err
			return
		}
		methods := make([]byte, int(greeting[1]))
		if _, err := io.ReadFull(conn, methods); err != nil {
			serverErr <- err
			return
		}
		if _, err := conn.Write([]byte{5, 2}); err != nil {
			serverErr <- err
			return
		}
		var version [2]byte
		if _, err := io.ReadFull(conn, version[:]); err != nil {
			serverErr <- err
			return
		}
		user := make([]byte, int(version[1]))
		if _, err := io.ReadFull(conn, user); err != nil {
			serverErr <- err
			return
		}
		var passLen [1]byte
		if _, err := io.ReadFull(cono, passLen[:]); err != nil {
			serverErr <- err
			return
		}
		password := make([]byte, int(passLen[0]))
		if _, err := io.ReadFull(conn, password); err != nil {
			serverErr <- err
			return
		}
		if string(user) != "alice" || string(password) != "secret" {
			serverErr <- ErrAuthentication
			return
		}
		if _, err := conn.Write([]byte{1, 0}); err != nil {
			serverErr <- err
			return
		}
		var request [4]byte
		if _, err := io.ReadFull(conn, request[:]); err != nil {
			serverErr <- err
			return
		}
		if request[0] != 5 || request[1] != 1 {
			serverErr <- io.ErrUnexpectedEOF
			return
		}
		if request[3] == 1 {
			_, err = io.CopyN(io.Discard, conn, 6)
		} else {
			err = io.ErrUnexpectedEOF
		}
		if err != nil {
			serverErr <- err
			return
		}
		if _, err := conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 80}); err != nil {
			serverErr <- err
			return
		}
		buf := make([]byte, 1)
		_, err = conn.Read(buf)
		serverErr <- err
	}()
	dialer := Dialer{ProxyAddress: listener.Addr().String(), Username: "alice", Password: "secret", Timeout: time.Second}
	conn, err := dialer.DialContext(context.Background(), "tcp", "127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if err := <-serverErr; err != nil && err != io.EOF {
		t.Fatal(err)
	}
}
