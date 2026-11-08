//go:build !linux && !windows

package diagnostics

import "errors"

func countXrayProcesses() (int, error) {
	return 0, errors.New("process inspection is unsupported on this platform")
}
