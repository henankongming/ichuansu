package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSummarizesAndDoesNotExposeSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "i.json")
	content := `{
	  "inbounds": [{"listen":"127.0.0.1","port":10808,"protocol":"socks","settings":{"auth":"password","accounts":[{"user":"user-secret","pass":"pass-secret"}]}}],
	  "outbounds": [{"protocol":"vless","settings":{"vnext":[{"address":"node-secret.example","users":[{"id":"uuid-secret"}]}]}}],
	  "routing":{"rules":[{"domain":["private.example"],"outboundTag":"proxy"}]}
	}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	summary, socks, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Inbounds) != 1 || summary.Inbounds[0].Protocol != "socks" || summary.Inbounds[0].Port != "10808" {
		t.Fatalf("summary: %#v", summary)
	}
	if len(summary.OutboundProtocols) != 1 || summary.OutboundProtocols[0] != "vless" || summary.RoutingRuleCount != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if len(socks) != 1 || socks[0].Address != "127.0.0.1:10808" || socks[0].Username != "user-secret" || socks[0].Password != "pass-secret" {
		t.Fatalf("unexpected SOCKS details: %#v", socks)
	}
	for _, secret := range []string{"user-secret", "pass-secret", "node-secret", "uuid-secret", "private.example"} {
		if strings.Contains(strings.Join(summary.OutboundProtocols, " "), secret) {
			t.Fatalf("secret leaked in summary: %q", secret)
		}
	}
}

func TestUniqueSOCKSRefusesAmbiguousAndMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "i.json")
	if err := os.WriteFile(path, []byte(`{"inbounds":[{"protocol":"socks","listen":"127.0.0.1","port":1080},{"protocol":"socks","listen":"127.0.0.1","port":1081}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := UniqueSOCKSInbound(path); err != ErrAmbiguousSOCKS {
		t.Fatalf("got %v, want ambiguous", err)
	}
	if err := os.WriteFile(path, []byte(`{"inbounds":[{"protocol":"http","listen":"127.0.0.1","port":8080}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := UniqueSOCKSInbound(path); err != ErrNoSOCKSInbound {
		t.Fatalf("got %v, want missing", err)
	}
}

func TestMalformedConfigDoesNotEchoSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "i.json")
	if err := os.WriteFile(path, []byte(`{"password":"do-not-leak",`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := Load(path)
	if err == nil || strings.Contains(err.Error(), "do-not-leak") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestDefaultsAreNotGuessed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "i.json")
	if err := os.WriteFile(path, []byte(`{"inbounds":[{"protocol":"socks"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, socks, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(socks) != 0 {
		t.Fatalf("should not infer Xray default listen/port: %#v", socks)
	}
}
