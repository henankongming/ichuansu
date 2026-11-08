package transfer

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var ErrUnsafePath = errors.New("unsafe path or filename")
var ErrSourceMissing = errors.New("source path does not exist")
var ErrSourceUnavailable = errors.New("source path is unavailable")

// PreparedSource is a stable snapshot descriptor. Directory inputs are stored
// as a temporary ZIP; ordinary files are re-opened and re-hashed by the receiver.
type PreparedSource struct {
	Name   string
	Path   string
	Size   int64
	SHA256 string
	remove bool
}

func PrepareSource(sourcePath string) (*PreparedSource, error) {
	if strings.TrimSpace(sourcePath) == "" {
		return nil, fmt.Errorf("源路径不能为空")
	}
	absolute, err := filepath.Abs(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("无法解析源路径")
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("源路径不存在：%w", ErrSourceMissing)
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("源路径没有读取权限：%w", ErrSourceUnavailable)
		}
		return nil, fmt.Errorf("源路径不可访问：%w", ErrSourceUnavailable)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("不接受符号链接作为源路径")
	}
	if info.IsDir() {
		name, err := SafeFilename(filepath.Base(filepath.Clean(absolute)) + ".zip")
		if err != nil {
			return nil, err
		}
		zipPath, err := zipDirectory(absolute)
		if err != nil {
			return nil, err
		}
		prepared, err := describe(zipPath, name, true)
		if err != nil {
			_ = os.Remove(zipPath)
			return nil, err
		}
		return prepared, nil
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("源路径必须是普通文件或文件夹")
	}
	name, err := SafeFilename(info.Name())
	if err != nil {
		return nil, err
	}
	return describe(absolute, name, false)
}

func (s *PreparedSource) Open() (*os.File, error) {
	return os.Open(s.Path)
}

func (s *PreparedSource) Cleanup() {
	if s != nil && s.remove {
		_ = os.Remove(s.Path)
	}
}

func describe(path, name string, remove bool) (*PreparedSource, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("无法读取源文件")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("源文件状态无效")
	}
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return nil, fmt.Errorf("计算源文件 SHA-256 失败")
	}
	return &PreparedSource{
		Name: name, Path: path, Size: size,
		SHA256: hex.EncodeToString(hash.Sum(nil)), remove: remove,
	}, nil
}

func zipDirectory(root string) (string, error) {
	root = filepath.Clean(root)
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("待压缩路径不是有效文件夹")
	}
	rootName, err := SafeFilename(filepath.Base(root))
	if err != nil {
		return "", err
	}
	archive, err := os.CreateTemp("", "ichuansu-*.zip")
	if err != nil {
		return "", fmt.Errorf("无法创建临时 ZIP")
	}
	archivePath := archive.Name()
	ok := false
	defer func() {
		_ = archive.Close()
		if !ok {
			_ = os.Remove(archivePath)
		}
	}()
	writer := zip.NewWriter(archive)
	rootHeader := &zip.FileHeader{Name: rootName + "/", Method: zip.Store}
	rootHeader.SetMode(os.ModeDir | 0o700)
	if _, err := writer.CreateHeader(rootHeader); err != nil {
		_ = writer.Close()
		return "", fmt.Errorf("创建 ZIP 顶层目录失败")
	}

	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil // Never follow or archive symlinks.
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if !info.Mode().IsRegular() && !info.IsDir() {
			return nil // Skip sockets, devices and other special files.
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		entryName := rootName + "/" + relative
		if info.IsDir() {
			entryName += "/"
		}
		if err := validateZipName(entryName); err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = entryName
		if info.IsDir() {
			header.Method = zip.Store
		} else {
			header.Method = zip.Deflate
		}
		zipEntry, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(zipEntry, input)
		closeErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		_ = writer.Close()
		return "", fmt.Errorf("压缩文件夹失败：%w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("完成 ZIP 文件失败")
	}
	if err := archive.Sync(); err != nil {
		return "", fmt.Errorf("写入临时 ZIP 失败")
	}
	if err := archive.Close(); err != nil {
		return "", fmt.Errorf("关闭临时 ZIP 失败")
	}
	ok = true
	return archivePath, nil
}

func validateZipName(name string) error {
	if !utf8.ValidString(name) || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, ":") {
		return ErrUnsafePath
	}
	for _, segment := range strings.Split(strings.TrimSuffix(name, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return ErrUnsafePath
		}
	}
	return nil
}

func SafeFilename(name string) (string, error) {
	if name == "" || name == "." || name == ".." || !utf8.ValidString(name) || len(name) > 240 {
		return "", ErrUnsafePath
	}
	if filepath.Base(name) != name || strings.ContainsAny(name, "/\\:") {
		return "", ErrUnsafePath
	}
	for _, r := range name {
		if r < 32 || r == 127 || strings.ContainsRune(`<>"|?*`, r) {
			return "", ErrUnsafePath
		}
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return "", ErrUnsafePath
	}
	upper := strings.ToUpper(strings.TrimRight(name, ". "))
	base := strings.SplitN(upper, ".", 2)[0]
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" ||
		(len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
		return "", ErrUnsafePath
	}
	return name, nil
}
