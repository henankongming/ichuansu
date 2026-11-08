package client

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/henankongming/ichuansu/internal/server"
)

func TestClientPutGetRoundTrip(t *testing.T) {
	remoteDir := t.TempDir()
	token := bytes.Repeat([]byte{0x31}, 32)
	httpServer := httptest.NewServer(server.NewHandler(remoteDir, token))
	defer httpServer.Close()
	transferClient, err := New(httpServer.URL, token, LocalTCPDialer{})
	if err != nil {
		t.Fatal(err)
	}
	defer transferClient.CloseIdleConnections()

	if err := transferClient.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	localFile := filepath.Join(t.TempDir(), "hello 文件.txt")
	payload := []byte("data through the client")
	if err := os.WriteFile(localFile, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := transferClient.IPut(context.Background(), localFile, nil); err != nil {
		t.Fatal(err)
	}
	remotePayload, err := os.ReadFile(filepath.Join(remoteDir, filepath.Base(localFile)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, remotePayload) {
		t.Fatal("uploaded content mismatch")
	}

	downloadDir := t.TempDir()
	downloaded, err := transferClient.IGet(context.Background(), filepath.Join(remoteDir, filepath.Base(localFile)), downloadDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(downloaded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, got) {
		t.Fatal("downloaded content mismatch")
	}
}

func TestClientRejectsUnexpectedBaseURL(t *testing.T) {
	token := bytes.Repeat([]byte{0x31}, 32)
	for _, value := range []string{"https://example.com:443", "http://user:pass@example.com:80", "http://example.com:80/path", "http://example.com"} {
		if _, err := New(value, token, LocalTCPDialer{}); err == nil {
			t.Errorf("expected %q to be rejected", value)
		}
	}
}

func TestClientNeverFallsBackToDirectTCP(t *testing.T) {
	remoteDir := t.TempDir()
	token := bytes.Repeat([]byte{0x31}, 32)
	httpServer := httptest.NewServer(server.NewHandler(remoteDir, token))
	defer httpServer.Close()
	transferClient, err := New(httpServer.URL, token, failDialer{})
	if err != nil {
		t.Fatal(err)
	}
	defer transferClient.CloseIdleConnections()
	// The HTTP server is reachable via ordinary TCP, but the provided Xray
	// dialer refuses all connections. The client must not bypass that dialer.
	if err := transferClient.Health(context.Background()); err == nil {
		t.Fatal("expected Xray route failure; direct fallback must not occur")
	}
}

type failDialer struct{}

func (failDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	return nil, errors.New("simulated Xray tunnel unavailable")
}
