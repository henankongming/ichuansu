// Package socks5 implements the small SOCKS5 CONNECT subset needed to route
// ichuansu HTTP traffic through an existing local Xray SOCKS inbound.
package socks5

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

var ErrAuthentication = errors.New("Xray SOCKS authentication failed or is unsupported")

type Dialer struct {
	ProxyAddress string
	Username     string
	Password     string
	Timeout      time.Duration
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("SOCKS5 dialer supports TCP only")
	}
	if d.ProxyAddress == "" {
		return nil, fmt.Errorf("Xray SOCKS endpoint is not configured")
	}
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	proxyDialer := net.Dialer{Timeout: timeout}
	conn, err := proxyDialer.DialContext(ctx, "tcp", d.ProxyAddress)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to local Xray SOCKS inbound: %w", err)
	}
	failed := true
	defer func() {
		if failed {
			_ = conn.Close()
		}
	}()

	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()

	methods := []byte{0}
	if d.Username != "" || d.Password != "" {
		if len(d.Username) > 255 || len(d.Password) > 255 || d.Username == "" || d.Password == "" {
			return nil, ErrAuthentication
		}
		methods = []byte{0, 2}
	}
	greeting := []byte{5, byte(len(methods))}
	greeting = append(greeting, methods...)
	if err := writeFull(conn, greeting); err != nil {
		return nil, fmt.Errorf("Xray SOCKS handshake failed")
	}
	var selection [2]byte
	if _, err := io.ReadFull(conn, selection[:]); err != nil || selection[0] != 5 || selection[1] == 0xff {
		return nil, ErrAuthentication
	}
	switch selection[1] {
	case 0:
		// no authentication
	case 2:
		if d.Username == "" || d.Password == "" {
			return nil, ErrAuthentication
		}
		auth := []byte{1, byte(len(d.Username))}
		auth = append(auth, []byte(d.Username)...)
		auth = append(auth, byte(len(d.Password)))
		auth = append(auth, []byte(d.Password)...)
		if err := writeFull(conn, auth); err != nil {
			return nil, ErrAuthentication
		}
		var authReply [2]byte
		if _, err := io.ReadFull(conn, authReply[:]); err != nil || authReply[0] != 1 || authReply[1] != 0 {
			return nil, ErrAuthentication
		}
	default:
		return nil, ErrAuthentication
	}

	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid peer endpoint")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid peer port")
	}
	request := []byte{5, 1, 0}
	if ip := net.ParseIP(host); ip != nil {
		if ipv4 := ip.To4(); ipv4 != nil {
			request = append(request, 1)
			request = append(request, ipv4...)
		} else {
			request = append(request, 4)
			request = append(request, ip.To16()...)
		}
	} else {
		if len(host) == 0 || len(host) > 255 || strings.ContainsAny(host, " /\\") {
			return nil, fmt.Errorf("invalid peer hostname")
		}
		request = append(request, 3, byte(len(host)))
		request = append(request, []byte(host)...)
	}
	var portBytes [2]byte
	binary.BigEndian.PutUint16(portBytes[:], uint16(port))
	request = append(request, portBytes[:]...)
	if err := writeFull(conn, request); err != nil {
		return nil, fmt.Errorf("Xray SOCKS connect request failed")
	}
	var response [4]byte
	if _, err := io.ReadFull(conn, response[:]); err != nil || response[0] != 5 || response[1] != 0 || response[2] != 0 {
		return nil, fmt.Errorf("Xray route could not reach the configured peer endpoint")
	}
	addressLength := 0
	switch response[3] {
	case 1:
		addressLength = 4
	case 4:
		addressLength = 16
	case 3:
		var length [1]byte
		if _, err := io.ReadFull(conn, length[:]); err != nil {
			return nil, fmt.Errorf("invalid SOCKS5 response")
		}
		addressLength = int(length[0])
	default:
		return nil, fmt.Errorf("invalid SOCKS5 response address type")
	}
	if _, err := io.CopyN(io.Discard, conn, int64(addressLength+2)); err != nil {
		return nil, fmt.Errorf("invalid SOCKS5 response")
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	failed = false
	return &idleConn{Conn: conn, timeout: 2 * time.Minute}, nil
}

func writeFull(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

// idleConn bounds inactivity during long file streams without imposing a total
// transfer duration; large files remain supported as long as bytes keep moving.
type idleConn struct {
	net.Conn
	timeout time.Duration
}

func (c *idleConn) Read(buffer []byte) (int, error) {
	_ = c.Conn.SetReadDeadline(time.Now().Add(c.timeout))
	return c.Conn.Read(buffer)
}

func (c *idleConn) Write(buffer []byte) (int, error) {
	_ = c.Conn.SetWriteDeadline(time.Now().Add(c.timeout))
	return c.Conn.Write(buffer)
}
