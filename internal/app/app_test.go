package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpAndVersion(t *testing.T) {
	var out, errOut strings.Builder
	if code := RunInDir([]string{"--help"}, "test", t.TempDir(), &out, &errOut); code != 0 {
		t.Fatalf("help exit code=%d", code)
	}
	if !strings.Contains(out.String(), "ichuansu serve") {
		t.Fatalf("help output incomplete: %s", out.String())
	}
	out.Reset()
	if code := RunInDir([]string{"--version"}, "1.2.3", t.TempDir(), &out, &errOut); code != 0 {
		t.Fatalf("version exit code=%d", code)
	}
	if strings.TrimSpace(out.String()) != "ichuansu 1.2.3" {
		t.Fatalf("unexpected version: %q", out.String())
	}
}

func TestUnknownCommandReturnsTwo(t *testing.T) {
	var out, errOut strings.Builder
	if code := RunInDir([]string{"unknown"}, "dev", t.TempDir(), &out, &errOut); code != 2 {
		t.Fatalf("unknown command exit code=%d", code)
	}
}

func TestServeFailsClosedWithoutPeerMapping(t *testing.T) {
	var out, errOut strings.Builder
	if code := RunInDir([]string{"serve"}, "dev", t.TempDir(), &out, &errOut); code != 3 {
		t.Fatalf("serve exit code=%d output=%s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "不会") && !strings.Contains(errOut.String(), "不能") {
		t.Fatalf("missing fail-closed explanation: %s", errOut.String())
	}
}

func TestIPutChecksSourceBeforeNetworkBlocker(t *testing.T) {
	var out, errOut strings.Builder
	if code := RunInDir([]string{"iput", "not-found"}, "dev", t.TempDir(), &out, &errOut); code != 5 {
		t.Fatalf("iput exit code=%d", code)
	}
}

func TestXrayNodeAddressIsNotTreatedAsPeer(t *testing.T) {
	dir := t.TempDir()
	config := []byte("{\"outbounds\":[{\"protocol\":\"vless\",\"settings\":{\"vnext\":[{\"address\":\"proxy-node.example\",\"port\":443}]}}]}")
	if err := os.WriteFile(filepath.Join(dir, "i.json"), config, 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut strings.Builder
	if code := RunInDir([]string{"iget", "/remote/file"}, "dev", dir, &out, &errOut); code != 4 {
		t.Fatalf("iget exit code=%d output=%s", code, errOut.String())
	}
	local := filepath.Join(dir, "hello world.txt")
	if err := os.WriteFile(local, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := RunInDir([]string{"iput", local}, "dev", dir, &out, &errOut); code != 4 {
		t.Fatalf("iput exit code=%d output=%s", code, errOut.String())
	}
	if strings.Contains(errOut.String(), "proxy-node.example") {
		t.Fatal("diagnostics revealed a proxy node address")
	}
}
