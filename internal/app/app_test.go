package app

import (
	"strings"
	"testing"
)

func TestHelpAndVersion(t *testing.T) {
	var out, errOut strings.Builder
	if code := Run([]string{"--help"}, "test", &out, &errOut); code != 0 {
		t.Fatalf("help exit code = %d", code)
	}
	if !strings.Contains(out.String(), "ichuansu serve") {
		t.Fatalf("help output is incomplete: %s", out.String())
	}

	out.Reset()
	if code := Run([]string{"--version"}, "1.2.3", &out, &errOut); code != 0 {
		t.Fatalf("version exit code = %d", code)
	}
	if strings.TrimSpace(out.String()) != "ichuansu 1.2.3" {
		t.Fatalf("unexpected version output: %q", out.String())
	}
}

func TestUnknownCommandReturnsUsageExitCode(t *testing.T) {
	var out, errOut strings.Builder
	if code := Run([]string{"unknown"}, "dev", &out, &errOut); code != 2 {
		t.Fatalf("unknown command exit code = %d, want 2", code)
	}
}

func TestTransferCommandsAreExplicitlyNotImplementedYet(t *testing.T) {
	var out, errOut strings.Builder
	if code := Run([]string{"serve"}, "dev", &out, &errOut); code != 1 {
		t.Fatalf("serve exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "尚未实现") {
		t.Fatalf("expected staged implementation notice, got: %s", errOut.String())
	}
}
