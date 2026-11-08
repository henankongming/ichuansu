package server

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
)

const tokenFilename = ".ichuansu-token"

func LoadToken(programDir string) ([]byte, error) {
	path := filepath.Join(programDir, tokenFilename)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, errors.New(".ichuansu-token is not available")
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != 32 {
		return nil, errors.New(".ichuansu-token must be a regular 32-byte file")
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) != 32 {
		return nil, errors.New("cannot read a valid .ichuansu-token")
	}
	return data, nil
}

// LoadOrCreateToken stores a 256-bit application credential in the program
// directory. The same file must be provisioned securely on both peers.
// The token itself is never printed or included in diagnostics.
func LoadOrCreateToken(programDir string) ([]byte, bool, error) {
	path := filepath.Join(programDir, tokenFilename)
	if data, err := LoadToken(programDir); err == nil {
		return data, false, nil
	}
	if _, err := os.Lstat(path); err == nil || !os.IsNotExist(err) {
		return nil, false, errors.New("cannot read a valid .ichuansu-token")
	}

	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return nil, false, errors.New("cannot generate application token")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			data, loadErr := LoadToken(programDir)
			return data, false, loadErr
		}
		return nil, false, errors.New("cannot create .ichuansu-token")
	}
	_, writeErr := file.Write(token)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return nil, false, errors.New("cannot persist application token")
	}
	return token, true, nil
}
