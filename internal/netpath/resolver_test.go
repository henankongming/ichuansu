package netpath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPeerEndpointNeverInferredFromXrayNode(t *testing.T) {
	dir := t.TempDir()
	config := `{"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"xray-node.example","port":443}]}}]}`
	if err := os.WriteFile(filepath.Join(dir, "i.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	endpoint, err := ResolvePeerEndpoint(dir)
	if endpoint != "" || err != ErrPeerEndpointUnknown {
		t.Fatalf("got endpoint=%q err=%v", endpoint, err)
	}
}

func TestListenEndpointFailsClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "i.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	address, err := ResolveListenEndpoint(dir)
	if address != "" || err != ErrListenEndpointUnknown {
		t.Fatalf("got address=%q err=%v", address, err)
	}
}
