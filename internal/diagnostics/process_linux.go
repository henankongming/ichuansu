//go:build linux

package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
)

func countXrayProcesses() (int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() || !isNumeric(entry.Name()) {
			continue
		}
		link := filepath.Join("/proc", entry.Name(), "exe")
		target, err := os.Readlink(link)
		if err == nil && isXrayExecutable(filepath.Base(target)) {
			count++
			continue
		}
		// Reading /proc/<pid>/exe may be restricted. Fall back to comm,
		// which contains the short process name and no command-line secrets.
		comm, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if err == nil && isXrayProcessName(strings.TrimSpace(string(comm))) {
			count++
		}
	}
	return count, nil
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func isXrayExecutable(value string) bool {
	base := strings.ToLower(value)
	return base == "xray" || base == "xray.exe" || base == "xray-linux-amd64"
}

func isXrayProcessName(value string) bool {
	name := strings.ToLower(value)
	return name == "xray" || name == "xray-linux-amd64"
}
