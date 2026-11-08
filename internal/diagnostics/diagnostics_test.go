package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectConfigSummarizesWithoutExposingSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "i.json")
	contents := "{\n" +
		"  \"inbounds\": [{\"listen\":\"127.0.0.1\",\"port\":10808,\"protocol\":\"socks\",\"settings\":{\"accounts\":[{\"user\":\"secret-user\",\"pass\":\"secret-password\"}]}}],\n" +
		"  \"outbounds\": [{\"protocol\":\"vless\",\"settings\":{\"vnext\":[{\"address\":\"private.example.invalid\",\"users\":[{\"id\":\"uuid-secret\"}]}]}}],\n" +
		"  \"routing\": {\"rules\": [{\"type\":\"field\",\"domain\":[\"full:secret.example.invalid\"],\"outboundTag\":\"proxy\"}]}\n" +
		"}"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	summary, problem := inspectConfig(path)
	if problem != "" {
		t.Fatalf("inspectConfig returned problem: %s", problem)
	}
	if len(summary.Inbounds) != 1 || summary.Inbounds[0].Protocol != "socks" || summary.Inbounds[0].Listen != "127.0.0.1" || summary.Inbounds[0].Port != "10808" {
		t.Fatalf("unexpected inbound summary: %#v", summary.Inbounds)
	}
	if len(summary.OutboundProtocols) != 1 || summary.OutboundProtocols[0] != "vless" {
		t.Fatalf("unexpected outbound summary: %#v", summary.OutboundProtocols)
	}
	if summary.RoutingRuleCount != 1 {
		t.Fatalf("expected 1 routing rule, got %d", summary.RoutingRuleCount)
	}

	var output strings.Builder
	Print(&output, Report{
		Platform:          "linux/amd64",
		ProgramDir:        dir,
		XrayExecutablePath: filepath.Join(dir, "xray"),
		ConfigPath:        path,
		ConfigFound:       true,
		ConfigValid:       true,
		Inbounds:          summary.Inbounds,
		OutboundProtocols: summary.OutboundProtocols,
		RoutingRuleCount:  summary.RoutingRuleCount,
	})
	for _, secret := range []string{"secret-user", "secret-password", "uuid-secret", "private.example.invalid", "secret.example.invalid"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("diagnostic output leaked %q", secret)
		}
	}
}

func TestInspectConfigRejectsMalformedJSONWithoutEchoingContents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "i.json")
	if err := os.WriteFile(path, []byte("{\"password\":\"never-print-this\","), 0o600); err != nil {
		t.Fatal(err)
	}
	_, problem := inspectConfig(path)
	if problem == "" || strings.Contains(problem, "never-print-this") {
		t.Fatalf("unexpected malformed-config message: %q", problem)
	}
}

func TestInspectConfigRejectsNonRegularFile(t *testing.T) {
	dir := t.TempDir()
	_, problem := inspectConfig(dir)
	if problem == "" {
		t.Fatal("expected non-regular path to be rejected")
	}
}

func TestSafeListenAndPort(t *testing.T) {
	if got := safeListen("remote.example.com"); got != "" {
		t.Fatalf("remote hostname should not be printed, got %q", got)
	}
	if got := safePort(float64(70000)); got != "" {
		t.Fatalf("invalid port should be omitted, got %q", got)
	}
}
