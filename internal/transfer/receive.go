package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrConflict = errors.New("target file already exists")
var ErrChecksum = errors.New("SHA-256 verification failed")
var ErrLength = errors.New("transferred file length verification failed")

// SaveAtomic writes into a same-directory temporary file, verifies exact length
// and SHA-256, then installs the result with an atomic no-overwrite hard link.
func SaveAtomic(destinationDir, filename string, source io.Reader, expectedSize int64, expectedSHA256 string, progress func(int64, int64)) (string, error) {
	name, err := SafeFilename(filename)
	if err != nil {
		return "", err
	}
	if expectedSize < 0 || expectedSize == int64(^uint64(0)>>1) || len(expectedSHA256) != 64 {
		return "", fmt.Errorf("传输元数据无效")
	}
	if _, err := hex.DecodeString(expectedSHA256); err != nil {
		return "", fmt.Errorf("SHA-256 元数据无效")
	}
	absoluteDir, err := filepath.Abs(destinationDir)
	if err != nil {
		return "", fmt.Errorf("无法解析接收目录")
	}
	info, err := os.Stat(absoluteDir)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("接收目录不存在或不可访问")
	}
	finalPath := filepath.Join(absoluteDir, name)
	if _, err := os.Lstat(finalPath); err == nil {
		return "", ErrConflict
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("无法检查目标文件")
	}

	temp, err := os.CreateTemp(absoluteDir, ".ichuansu-*.part")
	if err != nil {
		return "", fmt.Errorf("无法在接收目录创建临时文件（请检查权限和磁盘空间）")
	}
	tempPath := temp.Name()
	keepTemp := false
	defer func() {
		_ = temp.Close()
		if !keepTemp {
			_ = os.Remove(tempPath)
		}
	}()
	_ = temp.Chmod(0o600)

	hash := sha256.New()
	limited := io.LimitReader(source, expectedSize+1)
	var written int64
	buffer := make([]byte, 256*1024)
	for {
		read, readErr := limited.Read(buffer)
		if read > 0 {
			if _, err := temp.Write(buffer[:read]); err != nil {
				return "", fmt.Errorf("写入临时文件失败（磁盘空间或权限不足）")
			}
			_, _ = hash.Write(buffer[:read])
			written += int64(read)
			if progress != nil {
				progress(written, expectedSize)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", fmt.Errorf("接收文件流失败：%w", readErr)
		}
	}
	if written != expectedSize {
		return "", fmt.Errorf("%w：期望 %d 字节，实际 %d 字节", ErrLength, expectedSize, written)
	}
	actualHash := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actualHash, expectedSHA256) {
		return "", ErrChecksum
	}
	if err := temp.Sync(); err != nil {
		return "", fmt.Errorf("同步临时文件失败")
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("关闭临时文件失败")
	}

	// os.Link is atomic and fails if finalPath already exists. Both paths are in
	// the same directory, avoiding cross-volume operations and overwrite races.
	if err := os.Link(tempPath, finalPath); err != nil {
		if os.IsExist(err) {
			return "", ErrConflict
		}
		return "", fmt.Errorf("无法安全安装目标文件（文件系统不支持原子无覆盖写入）")
	}
	if err := os.Remove(tempPath); err != nil {
		// The final name is already verified and complete. Keep the final result,
		// but report the temporary cleanup issue to avoid silently hiding it.
		keepTemp = true
		return finalPath, fmt.Errorf("文件已保存，但临时文件清理失败：%s", filepath.Base(tempPath))
	}
	return finalPath, nil
}
