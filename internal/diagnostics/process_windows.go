//go:build windows

package diagnostics

import (
	"encoding/csv"
	"io"
	"os/exec"
	"strings"
)

func countXrayProcesses() (int, error) {
	output, err := exec.Command("tasklist", "/FI", "IMAGENAME eq xray.exe", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return 0, err
	}
	reader := csv.NewReader(strings.NewReader(string(output)))
	reader.FieldsPerRecord = -1
	count := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		if len(record) > 0 && strings.EqualFold(strings.TrimSpace(record[0]), "xray.exe") {
			count++
		}
	}
	return count, nil
}
